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
# passed separately (--diff-file) because of how we deliver the prompt per tool:
#
#   - stdin-capable tools (claude, codex, gemini) get the FULL prompt (instructions
#     + context + diff) piped over stdin. stdin has no size limit, so large PRs are
#     fine — this is the fix for the ARG_MAX/E2BIG failure on big diffs.
#   - argv-only tools (opencode, copilot, agency) get the prompt as a single argv
#     string. argv is bounded (on Linux, MAX_ARG_STRLEN = 128 KiB PER ARGUMENT), so
#     if instructions+context+diff would exceed a safe cap we OMIT the embedded diff
#     and instead instruct the agent — which runs inside the repo working tree — to
#     obtain the diff itself via `git diff <base>...HEAD`. No reviewer ever dies with
#     "Argument list too long".
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

if [ ! -f "${PROMPT_FILE}" ]; then
    err "❌ Prompt file not found: ${PROMPT_FILE}"
    exit 1
fi

STATUS_FILE="${OUTPUT_FILE}.status"
mkdir -p "$(dirname "${OUTPUT_FILE}")"

# Prepend the reviewer's assigned panel label so its review self-identifies by
# that label rather than by the underlying engine. This matters most for
# `agency copilot`, which otherwise reports itself as "GitHub Copilot CLI" and
# blurs with the standalone `copilot` entry. The collator keys on the label, so
# accurate self-attribution keeps the appendix and "flagged by" columns honest.
IDENTITY="You are the panel member labelled \"${LABEL}\"${MODEL:+ (model: ${MODEL})}. Begin your review's \"# Review by …\" heading with exactly \"${LABEL}\" so your output is attributed correctly when collated."

# --- Build the prompt, choosing delivery by the tool's stdin support ----------
# Which tools reliably read a piped prompt from stdin (verified empirically; see
# references/reviewer-cli-matrix.md). The rest are argv-only.
case "${TOOL}" in
claude | codex | gemini) DELIVERY="stdin" ;;
*) DELIVERY="argv" ;;
esac

# argv cap: stay comfortably under Linux's 128 KiB MAX_ARG_STRLEN per-argument
# limit (macOS allows ~1 MiB total via ARG_MAX, so the Linux limit is the binding
# one). 96 KiB leaves headroom for the instruction body + context around the diff.
ARGV_CAP=$((96 * 1024))

build_prompt_text() {
    # Always start with identity + instructions + non-diff context.
    printf '%s\n\n' "${IDENTITY}"
    cat "${PROMPT_FILE}"

    # Append the diff, or — for argv tools when it's too big — a pointer to fetch it.
    if [ -n "${DIFF_FILE}" ] && [ -s "${DIFF_FILE}" ]; then
        local diff_bytes base_ref
        diff_bytes="$(wc -c <"${DIFF_FILE}")"
        base_ref="${BASE:-the default branch}"
        if [ "${DELIVERY}" = "argv" ] && [ "${diff_bytes}" -gt "${ARGV_CAP}" ]; then
            # Omit the (huge) diff from argv; tell the agent to read it from git itself.
            printf '\n## The diff (ground truth)\n\n'
            printf 'The diff is %s bytes — too large to embed for this tool. You are running\n' "${diff_bytes}"
            printf 'inside the repository working tree, so obtain it yourself with:\n\n'
            printf '    git diff %s...HEAD\n\n' "${base_ref}"
            printf 'Read individual files at HEAD for surrounding context as needed.\n'
        else
            printf '\n## The diff (ground truth — what actually changed)\n\n'
            # Adaptive tilde fence so diff content cannot break out (matches build_prompt.sh).
            local longest len fence
            longest="$(awk '/^~+$/ { if (length > m) m = length } END { print m + 0 }' "${DIFF_FILE}")"
            len=$((longest + 1))
            [ "${len}" -lt 4 ] && len=4
            fence="$(printf '%.0s~' $(seq 1 "${len}"))"
            printf '%s\n' "${fence}"
            cat "${DIFF_FILE}"
            printf '%s\n' "${fence}"
        fi
    fi
}

PROMPT="$(build_prompt_text)"

# Pick a timeout mechanism. Prefer GNU `timeout`/`gtimeout`; if neither exists
# (stock macOS without coreutils — a documented target), fall back to a pure-bash
# watchdog so a wedged reviewer can never hang the background fan-out forever.
TIMEOUT_BIN=""
if command -v timeout >/dev/null 2>&1; then
    TIMEOUT_BIN="timeout"
elif command -v gtimeout >/dev/null 2>&1; then
    TIMEOUT_BIN="gtimeout"
fi

# Run "$@", feeding $PROMPT on stdin for stdin-delivery tools (harmless empty stdin
# otherwise). Enforces ${TIMEOUT}s via coreutils when present, else a bash watchdog.
# Returns 124 on timeout to match coreutils' convention.
run_guarded() {
    local stdin_data="$1"
    shift

    if [ -n "${TIMEOUT_BIN}" ]; then
        if [ "${DELIVERY}" = "stdin" ]; then
            printf '%s' "${stdin_data}" | "${TIMEOUT_BIN}" "${TIMEOUT}" "$@"
        else
            "${TIMEOUT_BIN}" "${TIMEOUT}" "$@" </dev/null
        fi
        return $?
    fi

    # --- pure-bash watchdog fallback ---
    if [ "${DELIVERY}" = "stdin" ]; then
        printf '%s' "${stdin_data}" | "$@" &
    else
        "$@" </dev/null &
    fi
    local cmd_pid=$!

    # The watchdog touches ${fired} just before killing, so we can distinguish a
    # timeout-kill (return 124) from the command's own non-zero exit — `wait` alone
    # can't, because a TERM-killed command and a self-terminating one both surface
    # as a non-zero status here.
    local fired
    fired="$(mktemp)"
    rm -f "${fired}"
    (
        sleep "${TIMEOUT}"
        : >"${fired}"
        kill -TERM "${cmd_pid}" 2>/dev/null
        sleep 5
        kill -KILL "${cmd_pid}" 2>/dev/null
    ) 2>/dev/null &
    local watchdog_pid=$!

    local rc=0
    wait "${cmd_pid}" 2>/dev/null || rc=$?

    # Stop the watchdog (it may be mid-sleep) and reap it.
    kill -TERM "${watchdog_pid}" 2>/dev/null
    wait "${watchdog_pid}" 2>/dev/null || true

    if [ -e "${fired}" ]; then
        rm -f "${fired}"
        return 124
    fi
    rm -f "${fired}"
    return "${rc}"
}

# Build the per-tool command as an array. For stdin tools the prompt is fed on
# stdin (claude reads stdin in -p mode; `codex exec -` and `gemini -p ""` consume
# stdin). For argv tools the prompt is the argv string. Each branch mirrors a row
# in references/reviewer-cli-matrix.md.
declare -a CMD
case "${TOOL}" in
claude)
    CMD=(claude -p --permission-mode plan)
    [ -n "${MODEL}" ] && CMD+=(--model "${MODEL}")
    ;;
codex)
    CMD=(codex exec --sandbox read-only --skip-git-repo-check --color never -)
    [ -n "${MODEL}" ] && CMD+=(-m "${MODEL}")
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
    echo "errored: unknown tool '${TOOL}'" >"${STATUS_FILE}"
    exit 0
    ;;
esac

echo "⏳ [${LABEL}] running ${TOOL}${MODEL:+ (model: ${MODEL})} via ${DELIVERY} ..." >&2
START="$(date +%s)"

# Capture everything the tool prints to stdout into a raw file. Some panel CLIs
# (copilot, agency) interleave tool-call traces like "● Read foo.ts" with their
# final answer on stdout, so we can't use stdout verbatim — we extract the review
# from between the sentinels the prompt asked for. stderr holds diagnostics.
RAW_FILE="${OUTPUT_FILE}.raw"
if run_guarded "${PROMPT}" "${CMD[@]}" >"${RAW_FILE}" 2>"${OUTPUT_FILE}.stderr"; then
    RC=0
else
    RC=$?
fi
END="$(date +%s)"
ELAPSED=$((END - START))

# Extract the review from between the sentinels. If they're missing (tool ignored
# the instruction), fall back to the raw stdout with box-drawing/TUI glyphs stripped
# so the collator still gets usable text rather than nothing.
extract_review() {
    if grep -q '===PR-REVIEW-BEGIN===' "${RAW_FILE}" 2>/dev/null; then
        sed -n '/===PR-REVIEW-BEGIN===/,/===PR-REVIEW-END===/p' "${RAW_FILE}" |
            sed '/===PR-REVIEW-BEGIN===/d;/===PR-REVIEW-END===/d'
    else
        # No sentinels — best effort: drop lines whose first non-space character is
        # a TUI box/status glyph (● │ └ ├ ✓ ✗ or the unicode replacement char).
        # Deliberately narrow: we don't strip on punctuation or indentation, so
        # legitimately indented or quoted review lines survive.
        sed -E '/^[[:space:]]*(●|│|└|├|✓|✗|�)/d' "${RAW_FILE}"
    fi
}

# Write a stub review file so the collator (which globs reviews/*.md) always sees
# an entry for this panel member, whatever the outcome.
write_stub() {
    local reason="$1"
    {
        echo "# Review by ${LABEL} — ${reason}"
        echo
        echo "Tool: ${TOOL}${MODEL:+ (model: ${MODEL})}. Exit code: ${RC}. Elapsed: ${ELAPSED}s."
        echo
        echo "Stderr tail:"
        echo '```'
        tail -n 20 "${OUTPUT_FILE}.stderr" 2>/dev/null
        echo '```'
    } >"${OUTPUT_FILE}"
}

# Decide status. A timeout (coreutils or our watchdog) returns 124.
if [ "${RC}" -eq 124 ]; then
    write_stub "TIMED OUT after ${TIMEOUT}s"
    echo "errored: timed out after ${TIMEOUT}s" >"${STATUS_FILE}"
    err "❌ [${LABEL}] timed out after ${TIMEOUT}s"
elif [ "${RC}" -eq 0 ] && [ -s "${RAW_FILE}" ]; then
    extract_review >"${OUTPUT_FILE}"
    if [ -s "${OUTPUT_FILE}" ] && grep -q '===PR-REVIEW-BEGIN===' "${RAW_FILE}" 2>/dev/null; then
        echo "ok: ${TOOL}${MODEL:+ ${MODEL}} in ${ELAPSED}s" >"${STATUS_FILE}"
        echo "✅ [${LABEL}] done in ${ELAPSED}s" >&2
    elif [ -s "${OUTPUT_FILE}" ]; then
        # Produced output but no sentinels — glyph-stripped salvage. Flag it as
        # ok-empty so the collator knows this may be tool-call noise, not a clean review.
        echo "ok-empty: ${TOOL} emitted no sentinels; salvaged raw output in ${ELAPSED}s" >"${STATUS_FILE}"
        err "⚠️  [${LABEL}] finished but no sentinel-wrapped review found; salvaged raw output."
    else
        cp "${RAW_FILE}" "${OUTPUT_FILE}"
        echo "ok-empty: ${TOOL} produced no extractable review in ${ELAPSED}s" >"${STATUS_FILE}"
        err "⚠️  [${LABEL}] finished but produced no usable review."
    fi
elif [ "${RC}" -eq 0 ]; then
    # Clean exit but completely empty stdout — distinct from a crash.
    write_stub "PRODUCED NO OUTPUT"
    echo "ok-empty: ${TOOL} exited 0 with empty output in ${ELAPSED}s" >"${STATUS_FILE}"
    err "⚠️  [${LABEL}] exited 0 but wrote nothing to stdout."
else
    write_stub "FAILED"
    echo "errored: exit ${RC} after ${ELAPSED}s" >"${STATUS_FILE}"
    err "❌ [${LABEL}] failed (exit ${RC}); see ${OUTPUT_FILE}.stderr"
fi

exit 0
