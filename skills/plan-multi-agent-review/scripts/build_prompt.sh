#!/usr/bin/env bash
#
# build_prompt.sh — assemble a SELF-CONTAINED plan-review prompt for one panel run.
#
# Usage:
#   build_prompt.sh <context-dir> <prompt-template>
#
# Prints to stdout: the instruction body (everything after the PROMPT-START sentinel in
# the template) followed by the plan-review context embedded inline as fenced blocks —
# repo orientation, the list of plan-referenced files that don't exist in the repo, each
# embedded referenced file, and finally the plan itself (last, as the primary artifact
# under review). run_reviewer.sh prepends a per-reviewer identity line and delivers the
# whole thing to every tool on stdin via a file redirect (no argv size limit; a redirect
# rather than a pipe, so a tool that exits without draining stdin can't trigger SIGPIPE).
#
# Why embed instead of pointing reviewers at file paths: the panel CLIs disagree wildly on
# filesystem sandboxing — opencode hard-rejects any path outside its working directory,
# others need per-tool --add-dir flags. Embedding gives every reviewer identical inputs
# with zero file-permission friction, while they can still explore the live repo (their
# cwd) read-only for anything not embedded.

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

# 2) Embedded context. Every block carries untrusted content (a plan body and the repo
#    files it cites routinely contain their own ``` code fences), so a plain ``` wrapper
#    can be broken out of, leaking the rest of the block into the prompt structure. We
#    fence with TILDES instead and size each fence adaptively: one longer than the longest
#    tilde run already in the block. CommonMark lets a closing fence carry ≤3 leading
#    spaces and trailing spaces, so we must count `   ~~~~  ` as a tilde run too — not only
#    pure-tilde lines — or indented untrusted content could still close the fence early.
#    Keep this in sync with the copy in run_reviewer.sh.
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

# Emit one fenced block for an ABSOLUTE file path with a chosen title.
emit_file() {
    local title="$1" file="$2" fence
    echo
    echo "## ${title}"
    echo
    if [ -s "${file}" ]; then
        fence="$(safe_fence "${file}")"
        echo "${fence}"
        cat "${file}"
        echo "${fence}"
    else
        echo "_(empty or not available)_"
    fi
}

echo
echo "---"
echo
echo "# PLAN CONTEXT (everything you need is below — explore the repo read-only for more)"

emit_file "Repository orientation" "${CTX}/repo-orientation.md"

# Files the plan references that do NOT exist in the repo. For a plan this is expected for
# files it proposes to create, and a red flag when it claims to modify something absent —
# the reviewer judges which. Only emit the block when there's something to say.
if [ -s "${CTX}/missing-references.txt" ]; then
    emit_file "Plan-referenced paths NOT found in the repo (new files to create, or broken references — you decide which)" \
        "${CTX}/missing-references.txt"
fi

# Loop over every embedded referenced file, titled with its real repo-relative path so
# reviewers can cite findings at the correct location.
if [ -s "${CTX}/referenced-files.txt" ]; then
    echo
    echo "---"
    echo
    echo "# REFERENCED REPO FILES (cited by the plan; embedded here for grounding)"
    while IFS= read -r rel; do
        [ -n "${rel}" ] || continue
        emit_file "${rel}" "${CTX}/referenced-files/${rel}"
    done <"${CTX}/referenced-files.txt"
fi

# The plan itself goes LAST — it is the primary artifact under review (mirrors the PR
# skill placing the ground-truth diff last). run_reviewer.sh prepends instructions ahead
# of all of this, so "last" here means "closest to where the reviewer starts writing",
# not "farthest from the instructions".
echo
echo "---"
echo
emit_file "THE PLAN UNDER REVIEW (this is what you are reviewing)" "${CTX}/plan.md"
