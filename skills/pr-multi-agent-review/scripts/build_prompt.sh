#!/usr/bin/env bash
#
# build_prompt.sh — assemble a SELF-CONTAINED review prompt for one panel run.
#
# Usage:
#   build_prompt.sh [--no-diff] <context-dir> <prompt-template>
#
# Prints to stdout: the instruction body (everything after the PROMPT-START
# sentinel in the template) followed by the PR context embedded inline as fenced
# blocks (changed files, commits, PR description, unresolved threads, and — unless
# --no-diff — the diff).
#
# --no-diff omits the diff block. run_reviewer.sh uses this because it delivers the
# diff itself: it appends the full diff to this prompt and feeds the whole thing to
# every reviewer on stdin via a file redirect (`tool < file`, not a pipe — so a tool
# that exits without draining stdin can't trigger SIGPIPE; no size limit either). So
# the diff must NOT also be baked into the shared prompt here, or it would appear
# twice. A human running build_prompt.sh by hand omits the flag to get the complete
# prompt.
#
# Why embed instead of pointing reviewers at file paths: the panel CLIs disagree
# wildly on filesystem sandboxing — opencode hard-rejects any path outside its
# working directory, others need per-tool --add-dir flags. Embedding the context
# in the prompt sidesteps all of that: every reviewer gets identical inputs with
# zero file-permission friction, and can still read the repo (its cwd) for
# surrounding context.

set -euo pipefail

err() {
    echo "$*" >&2
}

INCLUDE_DIFF=1
if [ "${1:-}" = "--no-diff" ]; then
    INCLUDE_DIFF=0
    shift
fi

CTX="${1:-}"
TEMPLATE="${2:-}"

if [ -z "${CTX}" ] || [ -z "${TEMPLATE}" ]; then
    err "❌ Usage: build_prompt.sh [--no-diff] <context-dir> <prompt-template>"
    exit 1
fi
if [ ! -d "${CTX}" ]; then
    err "❌ Context dir not found: ${CTX}"
    exit 1
fi
if [ ! -f "${TEMPLATE}" ]; then
    err "❌ Prompt template not found: ${TEMPLATE}"
    exit 1
fi

# 1) Instruction body — everything after the human-only note / PROMPT-START sentinel.
#    `sed '1,/re/d'` deletes to EOF if the sentinel is absent, which would emit an
#    instruction-less, context-only prompt. Guard: only strip when the sentinel is
#    present; otherwise emit the whole template (instructions still reach reviewers;
#    the only cost is the harmless human-note preamble) and warn.
if grep -q '<!-- PROMPT-START -->' "${TEMPLATE}"; then
    sed '1,/<!-- PROMPT-START -->/d' "${TEMPLATE}"
else
    err "⚠️  PROMPT-START sentinel not found in ${TEMPLATE}; emitting the full template so the prompt still carries instructions."
    cat "${TEMPLATE}"
fi

# 2) Embedded context. Every block — not just the diff — carries untrusted content
#    (PR bodies and commit messages routinely contain their own ``` code fences),
#    so a plain ``` wrapper can be broken out of, leaking the rest of the block into
#    the prompt structure. We fence with TILDES instead and size each fence adaptively:
#    one longer than the longest tilde run already in the block. CommonMark lets a
#    closing fence carry ≤3 leading spaces and trailing spaces, so we must count
#    `   ~~~~  ` as a tilde run too — not only pure-tilde lines — or indented untrusted
#    content could still close the fence early. A bare tilde fence is valid CommonMark
#    on both ends (unlike the old `~~~ DIFF ~~~`, whose closing line illegally carried
#    a label). Keep this in sync with the copy in run_reviewer.sh.
safe_fence() {
    local file="$1" longest len
    longest="$(awk 'match($0, /^ {0,3}(~+) *$/, a) { if (length(a[1]) > m) m = length(a[1]) } END { print m + 0 }' "${file}" 2>/dev/null)"
    # Fallback for awk builds without 3-arg match() (e.g. mawk): strip ≤3 leading and
    # trailing spaces, then measure pure-tilde lines.
    if [ -z "${longest}" ]; then
        longest="$(sed -E 's/^ {0,3}//; s/ *$//' "${file}" | awk '/^~+$/ { if (length > m) m = length } END { print m + 0 }')"
    fi
    len=$((longest + 1))
    [ "${len}" -lt 4 ] && len=4
    printf '%.0s~' $(seq 1 "${len}")
}

emit_block() {
    local title="$1" file="$2" fence
    echo
    echo "## ${title}"
    echo
    if [ -s "${CTX}/${file}" ]; then
        fence="$(safe_fence "${CTX}/${file}")"
        echo "${fence}"
        cat "${CTX}/${file}"
        echo "${fence}"
    else
        echo "_(empty or not available)_"
    fi
}

echo
echo "---"
echo
echo "# PR CONTEXT (everything you need is below — read repo files only for extra context)"

emit_block "Changed files" "changed-files.txt"
emit_block "Commit messages" "commits.txt"
emit_block "PR description (author intent — a claim about the diff, not ground truth)" "pr-description.md"
emit_block "Unresolved GitHub review threads" "unresolved-threads.md"
if [ "${INCLUDE_DIFF}" -eq 1 ]; then
    emit_block "The diff (ground truth — what actually changed)" "diff.patch"
fi
