#!/usr/bin/env bash
#
# build_prompt.sh — assemble a SELF-CONTAINED review prompt for one panel run.
#
# Usage:
#   build_prompt.sh <context-dir> <prompt-template>
#
# Prints to stdout: the instruction body (everything after the PROMPT-START
# sentinel in the template) followed by the PR context embedded inline as fenced
# blocks (diff, commits, changed files, PR description, unresolved threads).
#
# Why embed instead of pointing reviewers at file paths: the panel CLIs disagree
# wildly on filesystem sandboxing — opencode hard-rejects any path outside its
# working directory, others need per-tool --add-dir flags. Embedding the context
# in the prompt sidesteps all of that: every reviewer gets identical inputs with
# zero file-permission friction, and can still read the repo (its cwd) for
# surrounding context. The diff is bounded by the skill's size tiers, so this
# stays well under ARG_MAX.

set -euo pipefail

err() {
    echo "$*" >&2
}

CTX="${1:-}"
TEMPLATE="${2:-}"

if [ -z "${CTX}" ] || [ -z "${TEMPLATE}" ]; then
    err "❌ Usage: build_prompt.sh <context-dir> <prompt-template>"
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

# 1) Instruction body — everything after the human-only note / sentinel.
sed '1,/<!-- PROMPT-START -->/d' "${TEMPLATE}"

# 2) Embedded context. Each block is fenced so the reviewer can tell where the
#    PR data ends and (e.g.) a nested diff's own backticks don't confuse it — we
#    use a long, unlikely fence for the diff specifically.
emit_block() {
    local title="$1" file="$2" fence="${3:-\`\`\`}"
    echo
    echo "## ${title}"
    echo
    if [ -s "${CTX}/${file}" ]; then
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
# The diff last and with a distinctive fence — it's the largest and most likely
# to contain its own ``` sequences.
emit_block "The diff (ground truth — what actually changed)" "diff.patch" '~~~~~~~~ DIFF ~~~~~~~~'
