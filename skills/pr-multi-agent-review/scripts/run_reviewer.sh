#!/usr/bin/env bash
#
# run_reviewer.sh — run ONE reviewer CLI headlessly and read-only, capturing its review.
#
# Usage:
#   run_reviewer.sh --label <l> --tool <t> --model <m-or-empty> \
#       --prompt-file <f> [--diff-file <f>] [--base <ref>] \
#       --output-file <f> [--timeout <secs>]
#
# --prompt-file holds the instructions + PR context WITHOUT the diff. The diff is
# passed separately (--diff-file) and appended here, because the assembled prompt
# (identity + instructions + context + diff) is delivered to EVERY tool the same way:
# on stdin, via a file redirect (`tool < prompt`). stdin has no argv size limit, so
# large PRs are fine, and a file redirect (not a pipe) means a tool that exits without
# draining stdin does NOT make us take SIGPIPE. All six CLIs (claude, codex, gemini,
# opencode, copilot, agency) were verified to read the full prompt from stdin — see
# the dated note in references/reviewer-cli-matrix.md.
#
# copilot and agency additionally get `--context long_context` so a large PR fits
# their window without swapping the user's configured model. If a model still
# overflows (no long tier, or the diff exceeds even the long window), the status
# dispatch detects the overflow and tells the user to re-run with `--model <bigger>`.
#
# Writes the review to --output-file and a one-line status to <output-file>.status.
# Always exits 0 (a failed reviewer is recorded, not fatal) so a background fan-out
# of these never aborts the whole panel.
#
# Per-tool invocation is documented in references/reviewer-cli-matrix.md — keep both
# in sync. Read-only is enforced where the CLI supports it; copilot/opencode/agency
# lack a hard read-only switch, so the prompt forbids edits and the orchestrator
# diffs `git status` after the panel runs.

set -uo pipefail # NOTE: no -e; we handle reviewer failures explicitly.

err() {
    echo "$*" >&2
}

LABEL="" TOOL="" MODEL="" PROMPT_FILE="" DIFF_FILE="" BASE="" OUTPUT_FILE="" TIMEOUT=600
while [ $# -gt 0 ]; do
    case "$1" in
    --label)
        LABEL="$2"
        shift 2
        ;;
    --tool)
        TOOL="$2"
        shift 2
        ;;
    --model)
        MODEL="$2"
        shift 2
        ;;
    --prompt-file)
        PROMPT_FILE="$2"
        shift 2
        ;;
    --diff-file)
        DIFF_FILE="$2"
        shift 2
        ;;
    --base)
        # Still accepted (SKILL.md passes it) but no longer consumed: the diff is now
        # always embedded on stdin, so the old "run git diff <base>...HEAD yourself"
        # pointer that used BASE is gone. Kept for interface stability.
        # shellcheck disable=SC2034
        BASE="$2"
        shift 2
        ;;
    --output-file)
        OUTPUT_FILE="$2"
        shift 2
        ;;
    --timeout)
        TIMEOUT="$2"
        shift 2
        ;;
    *)
        err "Unknown arg: $1"
        exit 1
        ;;
    esac
done

if [ -z "${LABEL}" ] || [ -z "${TOOL}" ] || [ -z "${PROMPT_FILE}" ] || [ -z "${OUTPUT_FILE}" ]; then
    err "❌ Usage: run_reviewer.sh --label <l> --tool <t> --model <m> --prompt-file <f> --output-file <f>"
    exit 1
fi

# Require a NON-EMPTY prompt file: an empty one would send a reviewer no instructions
# at all, which silently produces garbage. `-s` catches the empty case that `-f` misses.
if [ ! -s "${PROMPT_FILE}" ]; then
    err "❌ Prompt file missing or empty: ${PROMPT_FILE}"
    exit 1
fi

STATUS_FILE="${OUTPUT_FILE}.status"
RAW_FILE="${OUTPUT_FILE}.raw"
ERR_FILE="${OUTPUT_FILE}.stderr"
mkdir -p "$(dirname "${OUTPUT_FILE}")"

# Prepend the reviewer's assigned panel label so its review self-identifies by that
# label rather than the underlying engine (matters most for `agency copilot`, which
# otherwise reports as "GitHub Copilot CLI"). The collator keys on the label.
IDENTITY="You are the panel member labelled \"${LABEL}\"${MODEL:+ (model: ${MODEL})}. Begin your review's \"# Review by …\" heading with exactly \"${LABEL}\" so your output is attributed correctly when collated."

# safe_fence FILE — longest fence that the file's content cannot close. CommonMark
# lets a closing fence carry ≤3 leading spaces and trailing spaces, so we must treat
# `   ~~~~  ` as a tilde run too, not only pure-tilde lines — otherwise indented
# untrusted content could break out of the block (it reaches --allow-all-tools tools).
safe_fence() {
    local file="$1" longest len
    longest="$(awk 'match($0, /^ {0,3}(~+) *$/, a) { if (length(a[1]) > m) m = length(a[1]) } END { print m + 0 }' "${file}" 2>/dev/null)"
    # Fallback for awk builds without the 3-arg match() (e.g. mawk): strip leading/
    # trailing spaces, then measure pure-tilde lines.
    if [ -z "${longest}" ]; then
        longest="$(sed -E 's/^ {0,3}//; s/ *$//' "${file}" | awk '/^~+$/ { if (length > m) m = length } END { print m + 0 }')"
    fi
    len=$((longest + 1))
    [ "${len}" -lt 4 ] && len=4
    printf '%.0s~' $(seq 1 "${len}")
}

# Assemble identity + instructions + non-diff context + (optionally) the diff, into
# a file. Every tool reads this file on stdin via a redirect (no SIGPIPE), so there
# is no argv size limit and the full diff is always embedded.
PROMPT_BUILT="$(mktemp)"
trap 'rm -f "${PROMPT_BUILT}"' EXIT

{
    printf '%s\n\n' "${IDENTITY}"
    cat "${PROMPT_FILE}"
} >"${PROMPT_BUILT}"

append_full_diff() {
    local fence
    {
        printf '\n## The diff (ground truth — what actually changed)\n\n'
        fence="$(safe_fence "${DIFF_FILE}")"
        printf '%s\n' "${fence}"
        cat "${DIFF_FILE}"
        printf '%s\n' "${fence}"
    } >>"${PROMPT_BUILT}"
}

# stdin has no size limit, so always embed the real diff when one was provided.
if [ -n "${DIFF_FILE}" ] && [ -s "${DIFF_FILE}" ]; then
    append_full_diff
fi

# Pick a timeout mechanism. Prefer GNU `timeout`/`gtimeout`; if neither exists
# (stock macOS without coreutils — a documented target), fall back to a pure-bash
# watchdog so a wedged reviewer can never hang the background fan-out forever.
TIMEOUT_BIN=""
if command -v timeout >/dev/null 2>&1; then
    TIMEOUT_BIN="timeout"
elif command -v gtimeout >/dev/null 2>&1; then
    TIMEOUT_BIN="gtimeout"
fi

# Run "$@" under a ${TIMEOUT}s guard, sending its stdout to ${RAW_FILE} and stderr to
# ${ERR_FILE}. The prompt arrives via a file redirect from ${PROMPT_BUILT} (a redirect,
# not a pipe — a tool that exits early without reading stdin won't make us take SIGPIPE
# and misreport a good review as failed).
# Redirects are applied to the command itself (not inherited through backgrounding),
# so the output files are owned and flushed by the command and are fully visible once
# `wait` returns. Returns 124 on timeout (matching coreutils).
run_guarded() {
    local in="${PROMPT_BUILT}"

    if [ -n "${TIMEOUT_BIN}" ]; then
        "${TIMEOUT_BIN}" "${TIMEOUT}" "$@" <"${in}" >"${RAW_FILE}" 2>"${ERR_FILE}"
        return $?
    fi

    # --- pure-bash watchdog fallback ---
    # Start the reviewer in its OWN process group when `setsid` is available, so the
    # watchdog can signal the whole group (the CLI + any children it forks). Without
    # setsid (e.g. stock macOS) a backgrounded child shares our process group, so a
    # negative-pid kill would target the orchestrator's own group — dangerous — and we
    # fall back to killing the pid plus its direct children by parent pid instead.
    local use_setsid=""
    command -v setsid >/dev/null 2>&1 && use_setsid="setsid"

    ${use_setsid} "$@" <"${in}" >"${RAW_FILE}" 2>"${ERR_FILE}" &
    local cmd_pid=$!

    # Flag file written by the watchdog the instant it fires, so we can tell a
    # timeout-kill apart from the command's own non-zero exit. Keep the mktemp file
    # (don't rm+recreate by name — that reopens the /tmp symlink race mktemp avoids)
    # and test it with `-s` (non-empty), so a 0-byte leftover can't read as "fired".
    local fired
    fired="$(mktemp)"
    : >"${fired}" # ensure empty to start

    # Kill helper: process group when we have setsid (child IS its group leader),
    # else the pid plus its direct children.
    kill_tree() {
        local sig="$1"
        if [ -n "${use_setsid}" ]; then
            kill "${sig}" "-${cmd_pid}" 2>/dev/null
        else
            pkill "${sig}" -P "${cmd_pid}" 2>/dev/null
            kill "${sig}" "${cmd_pid}" 2>/dev/null
        fi
    }

    (
        sleep "${TIMEOUT}"
        printf 'fired' >"${fired}"
        kill_tree -TERM
        sleep 5
        kill_tree -KILL
    ) &
    local watchdog_pid=$!

    local rc=0
    wait "${cmd_pid}" 2>/dev/null || rc=$?

    # Cancel the watchdog only if it hasn't fired (kill -0 confirms it's still alive
    # and mid-sleep), then reap it.
    if kill -0 "${watchdog_pid}" 2>/dev/null; then
        kill -TERM "${watchdog_pid}" 2>/dev/null
    fi
    wait "${watchdog_pid}" 2>/dev/null || true

    if [ -s "${fired}" ]; then
        rm -f "${fired}"
        return 124
    fi
    rm -f "${fired}"
    return "${rc}"
}

# Build the per-tool command. The prompt arrives on stdin for EVERY tool (via the
# redirect in run_guarded), so each command takes an empty prompt slot — `-p ""`,
# `run ""`, etc. — and the model flag goes BEFORE any positional/stdin marker. Each
# branch mirrors a row in references/reviewer-cli-matrix.md.
declare -a CMD
case "${TOOL}" in
claude)
    CMD=(claude -p --permission-mode plan)
    [ -n "${MODEL}" ] && CMD+=(--model "${MODEL}")
    ;;
codex)
    # `-m` before the trailing `-` (stdin marker), so the flag is unambiguously a flag.
    CMD=(codex exec --sandbox read-only --skip-git-repo-check --color never)
    [ -n "${MODEL}" ] && CMD+=(-m "${MODEL}")
    CMD+=(-)
    ;;
gemini)
    # `gemini -p ""` + piped stdin: stdin is appended to the (empty) -p value.
    # --skip-trust avoids the "untrusted directory" refusal that otherwise makes
    # gemini exit immediately when run in a repo it hasn't been told to trust.
    CMD=(gemini -p "" --approval-mode plan --skip-trust)
    [ -n "${MODEL}" ] && CMD+=(-m "${MODEL}")
    ;;
opencode)
    # `opencode run ""` + stdin: opencode reads the prompt from stdin (verified).
    CMD=(opencode run "")
    [ -n "${MODEL}" ] && CMD+=(-m "${MODEL}")
    ;;
copilot)
    # Prompt on stdin (verified copilot reads it). `--context long_context` expands the
    # window so a large PR fits without swapping the configured model; an overflow on a
    # model with no long tier is detected below and reported with a --model suggestion.
    CMD=(copilot -p "" --allow-all-tools --no-color --context long_context)
    [ -n "${MODEL}" ] && CMD+=(--model "${MODEL}")
    ;;
agency)
    # `agency copilot` forwards everything after `--` to the underlying Copilot CLI,
    # including the prompt on stdin and `--context long_context` (verified forwarded).
    CMD=(agency copilot -- -p "" --allow-all-tools --no-color --context long_context)
    [ -n "${MODEL}" ] && CMD+=(--model "${MODEL}")
    ;;
*)
    err "❌ Unknown tool: ${TOOL}"
    # Write a stub .md too, so a typo'd custom panel entry still appears in collation
    # (which globs reviews/*.md) rather than silently vanishing.
    {
        echo "# Review by ${LABEL} — UNKNOWN TOOL"
        echo
        echo "No reviewer CLI is wired for tool '${TOOL}'."
    } >"${OUTPUT_FILE}"
    echo "errored: unknown tool '${TOOL}'" >"${STATUS_FILE}"
    exit 0
    ;;
esac

echo "⏳ [${LABEL}] running ${TOOL}${MODEL:+ (model: ${MODEL})} via stdin ..." >&2
START="$(date +%s)"

# Run the reviewer. run_guarded owns the redirects (to RAW_FILE / ERR_FILE) so the
# captured output is flushed and fully visible once it returns. Some panel CLIs
# (copilot, agency) interleave tool-call traces with their final answer on stdout, so
# we extract the review from between the sentinels the prompt asked for.
if run_guarded "${CMD[@]}"; then
    RC=0
else
    RC=$?
fi
END="$(date +%s)"
ELAPSED=$((END - START))

# Sentinel handling. We match ONLY standalone sentinel lines (`^===…===$`), because
# this skill's own prompt/template text quotes the sentinels inline — a substring
# match truncates any review that mentions them (it did exactly that to two reviews
# in the skill's self-review). A run counts as a clean review only when BOTH a
# standalone BEGIN and a standalone END are present.
BEGIN_RE='^===PR-REVIEW-BEGIN===[[:space:]]*$'
END_RE='^===PR-REVIEW-END===[[:space:]]*$'

has_both_sentinels() {
    grep -qE "${BEGIN_RE}" "${RAW_FILE}" 2>/dev/null &&
        grep -qE "${END_RE}" "${RAW_FILE}" 2>/dev/null
}

extract_review() {
    if has_both_sentinels; then
        # Print between the first standalone BEGIN and the next standalone END,
        # dropping the sentinel lines themselves.
        awk -v b="${BEGIN_RE}" -v e="${END_RE}" '
            $0 ~ b { inside=1; next }
            $0 ~ e { if (inside) exit }
            inside { print }
        ' "${RAW_FILE}"
    else
        # No clean sentinel pair — best effort: drop TUI box/status glyph lines.
        sed -E '/^[[:space:]]*(●|│|└|├|✓|✗|�)/d' "${RAW_FILE}"
    fi
}

# Stub review file so the collator (which globs reviews/*.md) always has an entry.
write_stub() {
    local reason="$1"
    {
        echo "# Review by ${LABEL} — ${reason}"
        echo
        echo "Tool: ${TOOL}${MODEL:+ (model: ${MODEL})}. Exit code: ${RC}. Elapsed: ${ELAPSED}s."
        echo
        echo "Stderr tail:"
        echo '```'
        tail -n 20 "${ERR_FILE}" 2>/dev/null
        echo '```'
    } >"${OUTPUT_FILE}"
}

# Did the reviewer fail because the prompt overflowed the model's context window?
# This is the issue #2 case: copilot/agency on a model whose window (even the
# long_context tier) can't hold a large PR. We surface an actionable message instead
# of a cryptic exit code so the user knows to re-run with a larger-context --model.
#
# Intrinsic gate FIRST: a sentinel-clean review is NEVER reclassified as overflow.
# Beyond that, the two greps are deliberately asymmetric to avoid false-positives on a
# review that merely *discusses* context limits (a review of THIS skill does exactly
# that):
#   - `context_length_exceeded` is an underscored API error code that never appears in
#     ordinary review prose, so it's safe to match on stdout OR stderr.
#   - the natural-language phrases ("maximum context length", "request too large", …)
#     DO appear in prose, so they are matched on stderr ONLY — the model's own error
#     stream — never on stdout.
looks_like_context_overflow() {
    has_both_sentinels && return 1
    grep -qiE 'context_length_exceeded' "${RAW_FILE}" "${ERR_FILE}" 2>/dev/null && return 0
    grep -qiE 'context (window|length)|maximum context length|too many tokens|exceeds.*context|maximum.*tokens|request too large|input is too long|prompt is too long' \
        "${ERR_FILE}" 2>/dev/null
}

# Record an overflow result: stub .md (so collation still sees the member) plus an
# actionable status telling the user to retry this label on a larger-context model.
OVERFLOW_HINT="prompt exceeded the model's context window; re-run this label with --model <larger-context model>"
record_overflow() {
    write_stub "CONTEXT OVERFLOW"
    echo "errored: ${OVERFLOW_HINT}" >"${STATUS_FILE}"
    err "❌ [${LABEL}] ${OVERFLOW_HINT}"
}

# Decide status. Distinguish the common exit codes rather than collapsing all
# failures into "FAILED": 124 = timeout, 126 = found-but-not-executable, 127 =
# command not found (e.g. CLI vanished from PATH mid-run).
if [ "${RC}" -eq 124 ]; then
    write_stub "TIMED OUT after ${TIMEOUT}s"
    echo "errored: timed out after ${TIMEOUT}s" >"${STATUS_FILE}"
    err "❌ [${LABEL}] timed out after ${TIMEOUT}s"
elif [ "${RC}" -eq 127 ] || [ "${RC}" -eq 126 ]; then
    write_stub "COULD NOT EXECUTE ${TOOL} (rc ${RC})"
    echo "errored: could not execute ${TOOL} (rc ${RC})" >"${STATUS_FILE}"
    err "❌ [${LABEL}] could not execute ${TOOL} (rc ${RC})"
elif [ "${RC}" -eq 0 ] && [ -s "${RAW_FILE}" ]; then
    extract_review >"${OUTPUT_FILE}"
    if [ -s "${OUTPUT_FILE}" ] && has_both_sentinels; then
        echo "ok: ${TOOL}${MODEL:+ ${MODEL}} in ${ELAPSED}s" >"${STATUS_FILE}"
        echo "✅ [${LABEL}] done in ${ELAPSED}s" >&2
    elif [ -s "${OUTPUT_FILE}" ]; then
        # Output but no clean sentinel pair — glyph-stripped salvage. Salvage WINS over
        # an overflow guess: we have recoverable content, so keep it rather than clobber
        # it with a stub. Flag ok-empty so the collator knows this may be tool-call
        # noise, not a sentinel-clean review.
        echo "ok-empty: ${TOOL} emitted no clean sentinel pair; salvaged raw output in ${ELAPSED}s" >"${STATUS_FILE}"
        err "⚠️  [${LABEL}] finished but no standalone sentinel pair found; salvaged raw output."
    elif looks_like_context_overflow; then
        # No extractable output AND stderr shows a context-overflow error.
        record_overflow
    else
        cp "${RAW_FILE}" "${OUTPUT_FILE}"
        echo "ok-empty: ${TOOL} produced no extractable review in ${ELAPSED}s" >"${STATUS_FILE}"
        err "⚠️  [${LABEL}] finished but produced no usable review."
    fi
elif [ "${RC}" -eq 0 ]; then
    if looks_like_context_overflow; then
        record_overflow
    else
        write_stub "PRODUCED NO OUTPUT"
        echo "ok-empty: ${TOOL} exited 0 with empty output in ${ELAPSED}s" >"${STATUS_FILE}"
        err "⚠️  [${LABEL}] exited 0 but wrote nothing to stdout."
    fi
else
    if looks_like_context_overflow; then
        record_overflow
    else
        write_stub "FAILED"
        echo "errored: exit ${RC} after ${ELAPSED}s" >"${STATUS_FILE}"
        err "❌ [${LABEL}] failed (exit ${RC}); see ${ERR_FILE}"
    fi
fi

exit 0
