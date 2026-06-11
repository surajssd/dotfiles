#!/usr/bin/env bash
#
# run_reviewer.sh — run ONE reviewer CLI headlessly and read-only, capturing its review.
#
# Usage:
#   run_reviewer.sh --label <l> --tool <t> --model <m-or-empty> \
#       --prompt-file <f> --output-file <f> [--timeout <secs>]
#
# Writes the review to --output-file and a one-line status to <output-file>.status.
# Always exits 0 (a failed reviewer is recorded, not fatal) so a background fan-out
# of these never aborts the whole panel.
#
# Per-tool invocation is documented in references/reviewer-cli-matrix.md — keep both
# in sync. Read-only is enforced where the CLI supports it; copilot/opencode lack a
# hard read-only switch, so the prompt forbids edits and the orchestrator diffs
# `git status` after the panel runs.

set -uo pipefail # NOTE: no -e; we handle reviewer failures explicitly.

err() {
    echo "$*" >&2
}

LABEL="" TOOL="" MODEL="" PROMPT_FILE="" OUTPUT_FILE="" TIMEOUT=600
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
PROMPT="${IDENTITY}

$(cat "${PROMPT_FILE}")"

# Pick a timeout mechanism (GNU `timeout` / `gtimeout` on mac via coreutils).
TIMEOUT_BIN=""
if command -v timeout >/dev/null 2>&1; then
    TIMEOUT_BIN="timeout"
elif command -v gtimeout >/dev/null 2>&1; then
    TIMEOUT_BIN="gtimeout"
fi

run_with_timeout() {
    if [ -n "${TIMEOUT_BIN}" ]; then
        "${TIMEOUT_BIN}" "${TIMEOUT}" "$@"
    else
        "$@"
    fi
}

# Build the per-tool command as an array, appending the model flag only when set.
# Each branch mirrors a row in references/reviewer-cli-matrix.md.
declare -a CMD
case "${TOOL}" in
claude)
    CMD=(claude -p "${PROMPT}" --permission-mode plan)
    [ -n "${MODEL}" ] && CMD+=(--model "${MODEL}")
    ;;
codex)
    CMD=(codex exec "${PROMPT}" --sandbox read-only --skip-git-repo-check --color never)
    [ -n "${MODEL}" ] && CMD+=(-m "${MODEL}")
    ;;
gemini)
    CMD=(gemini -p "${PROMPT}" --approval-mode plan)
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

echo "⏳ [${LABEL}] running ${TOOL}${MODEL:+ (model: ${MODEL})} ..." >&2
START="$(date +%s)"

# Capture everything the tool prints to stdout into a raw file. Some panel CLIs
# (copilot, agency) interleave tool-call traces like "● Read foo.ts" with their
# final answer on stdout, so we can't use stdout verbatim — we extract the review
# from between the sentinels the prompt asked for. stderr holds diagnostics.
RAW_FILE="${OUTPUT_FILE}.raw"
if run_with_timeout "${CMD[@]}" >"${RAW_FILE}" 2>"${OUTPUT_FILE}.stderr"; then
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

# Decide status. A timeout via coreutils returns 124.
if [ "${RC}" -eq 0 ] && [ -s "${RAW_FILE}" ]; then
    extract_review >"${OUTPUT_FILE}"
    if [ -s "${OUTPUT_FILE}" ]; then
        echo "ok: ${TOOL}${MODEL:+ ${MODEL}} in ${ELAPSED}s" >"${STATUS_FILE}"
        echo "✅ [${LABEL}] done in ${ELAPSED}s" >&2
    else
        # Ran fine but produced no extractable review (e.g. refused, empty answer).
        cp "${RAW_FILE}" "${OUTPUT_FILE}"
        echo "ok-empty: ${TOOL} produced no clear review (raw kept) in ${ELAPSED}s" >"${STATUS_FILE}"
        err "⚠️  [${LABEL}] finished but no sentinel-wrapped review found; kept raw output."
    fi
elif [ "${RC}" -eq 124 ]; then
    echo "errored: timed out after ${TIMEOUT}s" >"${STATUS_FILE}"
    err "❌ [${LABEL}] timed out after ${TIMEOUT}s"
else
    # Surface why it failed by writing a stub the collator can render.
    {
        echo "# Review by ${LABEL} — FAILED"
        echo
        echo "Exit code: ${RC}. Stderr tail:"
        echo '```'
        tail -n 20 "${OUTPUT_FILE}.stderr" 2>/dev/null
        echo '```'
    } >"${OUTPUT_FILE}"
    echo "errored: exit ${RC} after ${ELAPSED}s" >"${STATUS_FILE}"
    err "❌ [${LABEL}] failed (exit ${RC}); see ${OUTPUT_FILE}.stderr"
fi

exit 0
