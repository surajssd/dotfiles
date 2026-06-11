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
# passed separately (--diff-file) because delivery differs per tool:
#
#   - stdin-capable tools (claude, codex, gemini) get the FULL prompt (instructions
#     + context + diff) on stdin via a file redirect (`tool < prompt`). stdin has no
#     argv size limit, so large PRs are fine, and a file redirect (not a pipe) means
#     a tool that exits without draining stdin does NOT make us take SIGPIPE.
#   - argv-only tools (opencode, copilot, agency) get the prompt as a single argv
#     string. argv is bounded (Linux: MAX_ARG_STRLEN = 128 KiB PER ARGUMENT). If the
#     ASSEMBLED prompt (identity + instructions + context + diff — not just the diff)
#     would exceed a safe cap, we OMIT the diff and instruct the agent — which runs in
#     the repo working tree — to obtain it via `git diff <base>...HEAD`. If even the
#     diff-less prompt is over the cap, we hard-truncate as a last resort. No reviewer
#     ever dies with "Argument list too long".
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

# Delivery mode by the tool's verified stdin support (see reviewer-cli-matrix.md).
case "${TOOL}" in
claude | codex | gemini) DELIVERY="stdin" ;;
*) DELIVERY="argv" ;;
esac

# Stay under Linux's 128 KiB MAX_ARG_STRLEN per-argument limit (macOS ARG_MAX is
# ~1 MiB total, so Linux is binding). 96 KiB leaves headroom for the other argv
# elements the CLI adds. Only argv-delivery tools are bound by this.
ARGV_CAP=$((96 * 1024))

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
# a file. We always materialise the prompt to a file: stdin tools read it via redirect
# (no SIGPIPE), argv tools have it read into a variable below.
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

append_diff_pointer() {
    # base_ref must be a real ref for the suggested command to run, and it's printf %q
    # so a branch name with shell metacharacters can't inject when a tool-enabled agent
    # runs the suggestion. Fall back to a literal default ref, not prose.
    local base_ref
    base_ref="$(printf '%q' "${BASE:-origin/HEAD}")"
    {
        printf '\n## The diff (ground truth)\n\n'
        printf 'The diff is too large to embed for this tool. You are running inside the\n'
        printf 'repository working tree, so obtain it yourself with:\n\n'
        printf '    git diff %s...HEAD\n\n' "${base_ref}"
        printf 'Read individual files at HEAD for surrounding context as needed.\n'
    } >>"${PROMPT_BUILT}"
}

if [ -n "${DIFF_FILE}" ] && [ -s "${DIFF_FILE}" ]; then
    if [ "${DELIVERY}" = "stdin" ]; then
        # stdin: no size limit, always embed the real diff.
        append_full_diff
    else
        # argv: decide on the ASSEMBLED prompt size, not just the diff. The non-diff
        # context (PR body, threads, commit bodies) shares the same 128 KiB argv slot.
        nondiff_bytes="$(wc -c <"${PROMPT_BUILT}")"
        diff_bytes="$(wc -c <"${DIFF_FILE}")"
        if [ "$((nondiff_bytes + diff_bytes))" -le "${ARGV_CAP}" ]; then
            append_full_diff
        else
            # Diff won't fit. If even the non-diff prompt is over budget, truncate it
            # FIRST (keeping the head, where instructions + sentinel rules live) and
            # reserve room, so the "fetch the diff yourself" pointer we append next is
            # never itself cut off — the agent always gets the guidance to read git.
            if [ "${nondiff_bytes}" -gt "$((ARGV_CAP - 600))" ]; then
                err "⚠️  [${LABEL}] non-diff context exceeds the argv budget; truncating it."
                truncated="$(head -c "$((ARGV_CAP - 600))" "${PROMPT_BUILT}")"
                printf '%s\n\n[context truncated to fit the argument-size limit for this tool]\n' "${truncated}" >"${PROMPT_BUILT}"
            fi
            append_diff_pointer
        fi
    fi
fi

# Read the assembled prompt for argv tools. After the logic above it is guaranteed to
# fit, but clamp defensively in case the instruction body alone is pathologically large.
if [ "${DELIVERY}" = "argv" ]; then
    if [ "$(wc -c <"${PROMPT_BUILT}")" -gt "${ARGV_CAP}" ]; then
        err "⚠️  [${LABEL}] assembled prompt still exceeds ${ARGV_CAP}B; hard-truncating."
        PROMPT="$(head -c "$((ARGV_CAP - 200))" "${PROMPT_BUILT}")"
        PROMPT="${PROMPT}"$'\n\n[prompt truncated to fit the argument-size limit for this tool; read files in the repo for anything missing]'
    else
        PROMPT="$(cat "${PROMPT_BUILT}")"
    fi
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
# ${ERR_FILE}. stdin tools get the prompt via a file redirect from ${PROMPT_BUILT}
# (a redirect, not a pipe — a tool that exits early without reading stdin won't make
# us take SIGPIPE and misreport a good review as failed). argv tools read /dev/null.
# Redirects are applied to the command itself (not inherited through backgrounding),
# so the output files are owned and flushed by the command and are fully visible once
# `wait` returns. Returns 124 on timeout (matching coreutils).
run_guarded() {
    local in=/dev/null
    [ "${DELIVERY}" = "stdin" ] && in="${PROMPT_BUILT}"

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

# Build the per-tool command. stdin tools take NO prompt in argv (it arrives via the
# redirect); argv tools embed ${PROMPT}. Model flag goes BEFORE any positional/stdin
# marker. Each branch mirrors a row in references/reviewer-cli-matrix.md.
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
    CMD=(opencode run "${PROMPT}")
    [ -n "${MODEL}" ] && CMD+=(-m "${MODEL}")
    ;;
copilot)
    CMD=(copilot -p "${PROMPT}" --allow-all-tools --no-color)
    [ -n "${MODEL}" ] && CMD+=(--model "${MODEL}")
    ;;
agency)
    # `agency copilot` forwards everything after `--` to the underlying Copilot CLI.
    CMD=(agency copilot -- -p "${PROMPT}" --allow-all-tools --no-color)
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

echo "⏳ [${LABEL}] running ${TOOL}${MODEL:+ (model: ${MODEL})} via ${DELIVERY} ..." >&2
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
        # Output but no clean sentinel pair — glyph-stripped salvage. Flag ok-empty so
        # the collator knows this may be tool-call noise, not a sentinel-clean review.
        echo "ok-empty: ${TOOL} emitted no clean sentinel pair; salvaged raw output in ${ELAPSED}s" >"${STATUS_FILE}"
        err "⚠️  [${LABEL}] finished but no standalone sentinel pair found; salvaged raw output."
    else
        cp "${RAW_FILE}" "${OUTPUT_FILE}"
        echo "ok-empty: ${TOOL} produced no extractable review in ${ELAPSED}s" >"${STATUS_FILE}"
        err "⚠️  [${LABEL}] finished but produced no usable review."
    fi
elif [ "${RC}" -eq 0 ]; then
    write_stub "PRODUCED NO OUTPUT"
    echo "ok-empty: ${TOOL} exited 0 with empty output in ${ELAPSED}s" >"${STATUS_FILE}"
    err "⚠️  [${LABEL}] exited 0 but wrote nothing to stdout."
else
    write_stub "FAILED"
    echo "errored: exit ${RC} after ${ELAPSED}s" >"${STATUS_FILE}"
    err "❌ [${LABEL}] failed (exit ${RC}); see ${ERR_FILE}"
fi

exit 0
