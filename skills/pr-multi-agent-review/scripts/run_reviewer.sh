#!/usr/bin/env bash
#
# run_reviewer.sh — run ONE reviewer CLI headlessly and read-only, capturing its review.
#
# Usage:
#   run_reviewer.sh --label <l> --tool <t> --model <m-or-empty> [--effort <e>] \
#       --prompt-file <f> [--diff-file <f>] [--base <ref>] \
#       --output-file <f> [--timeout <secs>]
#
# --effort sets per-tool reasoning effort (codex model_reasoning_effort, opencode
# --variant). Valid values differ per tool; the orchestrator supplies one the chosen
# tool accepts. claude/agy have no such flag, so an --effort given for them is
# ignored with a note. Empty = tool default.
#
# --prompt-file holds the instructions + PR context WITHOUT the diff. The diff is
# passed separately (--diff-file) and appended here, because the assembled prompt
# (identity + instructions + context + diff) is delivered to EVERY tool the same way:
# on stdin, via a file redirect (`tool < prompt`). stdin has no argv size limit, so
# large PRs are fine, and a file redirect (not a pipe) means a tool that exits without
# draining stdin does NOT make us take SIGPIPE. claude, codex, and opencode were
# verified to read the full prompt from stdin. Two tools are exceptions BY DESIGN, not
# by omission, and read ${PROMPT_BUILT} into their argv instead of relying on this
# redirect: cursor (cursor-agent) has no documented stdin support, and agy (Google
# Antigravity CLI 1.2.x) rejects `-p ""` with the prompt on stdin ("empty prompt").
# See references/reviewer-cli-matrix.md for both.
#
# Writes the review to --output-file and a one-line status to <output-file>.status.
# Always exits 0 (a failed reviewer is recorded, not fatal) so a background fan-out
# of these never aborts the whole panel.
#
# Per-tool invocation is documented in references/reviewer-cli-matrix.md — keep both
# in sync. Read-only is enforced where the CLI supports it; opencode lacks a hard
# read-only switch, so the prompt forbids edits and the orchestrator diffs
# `git status` after the panel runs.

set -uo pipefail # NOTE: no -e; we handle reviewer failures explicitly.

err() {
    echo "$*" >&2
}

LABEL="" TOOL="" MODEL="" EFFORT="" PROMPT_FILE="" DIFF_FILE="" BASE="" OUTPUT_FILE="" TIMEOUT=600
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
    --effort)
        EFFORT="$2"
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
# label rather than the underlying engine — matters when the same tool is run twice
# with different models/efforts under distinct labels. The collator keys on the label.
IDENTITY="You are the panel member labelled \"${LABEL}\"${MODEL:+ (model: ${MODEL})}${EFFORT:+ (effort: ${EFFORT})}. Begin your review's \"# Review by …\" heading with exactly \"${LABEL}\" so your output is attributed correctly when collated."

# agy takes the prompt's "write your review to stdout" literally: without this note it
# ran a shell `echo` of the review inside a tool call, headless mode discarded that
# output, and its final message was a three-line summary claiming the review had been
# printed (status ok-empty). Telling it that its final response text IS the captured
# stdout produced a sentinel-clean review on the same PR (agy 1.2.6, 2026-09-28).
PREAMBLE=""
if [ "${TOOL}" = "agy" ]; then
    PREAMBLE='DELIVERY NOTE: the text of your final response is what the collator captures as "stdout". Do NOT run a shell command such as echo, cat, or printf to print the review; write the whole review, including both sentinel lines, directly as your final response text.'
fi

# safe_fence FILE — longest fence that the file's content cannot close. CommonMark
# lets a closing fence carry ≤3 leading spaces and trailing spaces, so we must treat
# `   ~~~~  ` as a tilde run too, not only pure-tilde lines — otherwise indented
# untrusted content could break out of the block (it reaches fully tool-enabled reviewers).
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
    [ -n "${PREAMBLE}" ] && printf '%s\n\n' "${PREAMBLE}"
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

# Build the per-tool command. The prompt arrives on stdin (via the redirect in
# run_guarded) for every tool except agy and cursor, so those commands take an empty
# prompt slot — `-p`, `run ""`, `-` — and the model flag goes BEFORE any
# positional/stdin marker. Each branch mirrors a row in references/reviewer-cli-matrix.md.
# LOGIN_HINT names the command that repairs an authentication failure for tools where
# that command is known; the status line quotes it so the orchestrator can relay it.
declare -a CMD
LOGIN_HINT=""
case "${TOOL}" in
claude)
    CMD=(claude -p --permission-mode plan)
    [ -n "${MODEL}" ] && CMD+=(--model "${MODEL}")
    [ -n "${EFFORT}" ] && err "ℹ️ [${LABEL}] claude has no reasoning-effort flag in -p mode; ignoring --effort ${EFFORT}"
    ;;
codex)
    # `-m` before the trailing `-` (stdin marker), so the flag is unambiguously a flag.
    CMD=(codex exec --sandbox read-only --skip-git-repo-check --color never)
    [ -n "${MODEL}" ] && CMD+=(-m "${MODEL}")
    # Reasoning effort is a config override, not a flag; must also precede the `-`.
    [ -n "${EFFORT}" ] && CMD+=(-c "model_reasoning_effort=\"${EFFORT}\"")
    CMD+=(-)
    ;;
agy)
    # Google Antigravity CLI (gemini-cli lineage). agy 1.2.x rejects `-p ""` with the prompt
    # on stdin (`error: Error: empty prompt. Usage: agy --print "your prompt here"`), so like
    # cursor it gets the assembled prompt as the -p argv string — same argv-size caveat.
    # It has no hard read-only mode like gemini's `--approval-mode plan`; `--sandbox` is the
    # nearest — terminal-restricted, and it auto-approves tool calls so a headless run won't
    # hang on a permission prompt. Headless mode still auto-DENIES any tool that is not
    # listed under permissions.allow in ~/.gemini/antigravity-cli/settings.json, and it
    # refuses a cwd missing from trustedWorkspaces there; a denial is detected below and
    # reported as an actionable status. `--print-timeout` (default 0 = unbounded) is pinned
    # to our outer guard so agy can end its turn cleanly before the watchdog kills it.
    CMD=(agy -p "$(cat "${PROMPT_BUILT}")" --sandbox --print-timeout "${TIMEOUT}s")
    [ -n "${MODEL}" ] && CMD+=(--model "${MODEL}")
    [ -n "${EFFORT}" ] && err "ℹ️ [${LABEL}] agy has no reasoning-effort flag; ignoring --effort ${EFFORT}"
    ;;
opencode)
    # `opencode run ""` + stdin: opencode reads the prompt from stdin (verified).
    CMD=(opencode run "")
    [ -n "${MODEL}" ] && CMD+=(-m "${MODEL}")
    # opencode calls reasoning effort a model "variant" (provider-specific levels).
    [ -n "${EFFORT}" ] && CMD+=(--variant "${EFFORT}")
    ;;
cursor)
    # Cursor CLI (binary: cursor-agent, also aliased as `agent`). `--plan` is a genuine
    # hard read-only mode (no edits) combined with `-p` for headless output, same tier
    # as claude/codex. `--trust` avoids a hang on the interactive workspace-trust prompt.
    #
    # UNLIKE every other tool here, cursor-agent takes its prompt as an argv string, not
    # stdin (no documented stdin support in `agent --help`). So instead of relying on the
    # stdin redirect run_guarded gives every command, read the assembled prompt into the
    # argv itself. This can hit the OS argv-size limit on very large diffs/plans — a
    # different, less graceful failure than this script's context-overflow detection.
    #
    # `cursor-agent status` can report "Login successful!" while `-p` runs still fail with
    # "Authentication required" (stale stored login, 2026-09-28); `cursor-agent login` fixes it.
    CMD=(cursor-agent -p "$(cat "${PROMPT_BUILT}")" --plan --trust --output-format text)
    LOGIN_HINT="cursor-agent login"
    [ -n "${MODEL}" ] && CMD+=(--model "${MODEL}")
    [ -n "${EFFORT}" ] && err "ℹ️ [${LABEL}] cursor has no reasoning-effort flag; pick a different --model (e.g. cursor-grok-4.5-medium) instead; ignoring --effort ${EFFORT}"
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

# agy and cursor receive the prompt via argv, not the stdin redirect every other
# command gets from run_guarded — say so accurately in the progress line.
DELIVERY="via stdin"
case "${TOOL}" in agy | cursor) DELIVERY="via argv" ;; esac
echo "⏳ [${LABEL}] running ${TOOL}${MODEL:+ (model: ${MODEL})}${EFFORT:+ (effort: ${EFFORT})} ${DELIVERY} ..." >&2
START="$(date +%s)"

# Run the reviewer. run_guarded owns the redirects (to RAW_FILE / ERR_FILE) so the
# captured output is flushed and fully visible once it returns. Some panel CLIs
# interleave tool-call traces with their final answer on stdout, so we extract the
# review from between the sentinels the prompt asked for.
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
# A large PR can exceed a model's context window outright. We surface an actionable
# message instead of a cryptic exit code so the user knows to re-run with a
# larger-context --model.
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

# Did the CLI refuse because its stored login is missing or stale? Matched on stderr
# only (the tool's own error stream), never on review prose. Observed: cursor-agent
# `Error: Authentication required. Please run 'agent login' first, or set CURSOR_API_KEY
# environment variable.` while `cursor-agent status` still claimed to be logged in.
looks_like_auth_failure() {
    has_both_sentinels && return 1
    grep -qiE 'authentication required|not (logged|signed) in|please (log|sign) in|login required|run .{0,20}login' \
        "${ERR_FILE}" 2>/dev/null
}

AUTH_HINT="${TOOL} is not logged in; run '${LOGIN_HINT:-the ${TOOL} login command}' and re-run this label once"
record_auth_failure() {
    write_stub "NOT LOGGED IN"
    echo "errored: ${AUTH_HINT}" >"${STATUS_FILE}"
    err "❌ [${LABEL}] ${AUTH_HINT}"
}

# Did agy's headless mode auto-deny a tool call? It then exits 0 with empty stdout and
# a stderr line such as `jetski: no output produced — a tool required the "read_file"
# permission that headless mode cannot prompt for, so it was auto-denied`. The fix is a
# settings.json allow list, so say so instead of reporting a bare "no output".
looks_like_denied_permission() {
    has_both_sentinels && return 1
    grep -qiE 'auto-denied|cannot prompt for|permission .*denied' "${RAW_FILE}" "${ERR_FILE}" 2>/dev/null
}

DENIED_HINT="${TOOL} auto-denied a tool permission in headless mode; add the permissions.allow list and this repo's path to trustedWorkspaces in ~/.gemini/antigravity-cli/settings.json (see references/reviewer-cli-matrix.md), then re-run this label once"
record_denied_permission() {
    write_stub "TOOL PERMISSION AUTO-DENIED"
    echo "errored: ${DENIED_HINT}" >"${STATUS_FILE}"
    err "❌ [${LABEL}] ${DENIED_HINT}"
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
    if looks_like_denied_permission; then
        record_denied_permission
    elif looks_like_context_overflow; then
        record_overflow
    else
        write_stub "PRODUCED NO OUTPUT"
        echo "ok-empty: ${TOOL} exited 0 with empty output in ${ELAPSED}s" >"${STATUS_FILE}"
        err "⚠️  [${LABEL}] exited 0 but wrote nothing to stdout."
    fi
else
    if looks_like_auth_failure; then
        record_auth_failure
    elif looks_like_denied_permission; then
        record_denied_permission
    elif looks_like_context_overflow; then
        record_overflow
    else
        write_stub "FAILED"
        echo "errored: exit ${RC} after ${ELAPSED}s" >"${STATUS_FILE}"
        err "❌ [${LABEL}] failed (exit ${RC}); see ${ERR_FILE}"
    fi
fi

exit 0
