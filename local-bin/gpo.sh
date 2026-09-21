#!/usr/bin/env bash
#
# Push the current branch to origin. With --copilot, also request a Copilot
# code review on the branch's open pull request. --copilot is consumed here;
# every other argument is passed to git push. The review effort level (Lite or
# Balanced) cannot be set from the CLI or the API; it comes from the
# repository's default under Settings > Copilot > Code review.

set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "$(realpath "${BASH_SOURCE[0]}")")"/util.sh

export GH_PROMPT_DISABLED=1
export GH_PAGER=cat

copilot=0
push_args=()
for arg in "$@"; do
    if [[ "$arg" == "--copilot" ]]; then
        copilot=1
    else
        push_args+=("$arg")
    fi
done

branch=$(git branch --show-current)
if [[ -z "$branch" ]]; then
    err "❌ Not on a branch (detached HEAD?)"
    exit 1
fi

(
    set -x
    git push -u origin "$branch" ${push_args[@]+"${push_args[@]}"}
)

if [[ "$copilot" -eq 0 ]]; then
    exit 0
fi

if ! pr=$(gh pr view --json number,state,url); then
    echo "ℹ️ No pull request for branch ${branch}; not requesting a Copilot review"
    exit 0
fi

number=$(jq -r '.number' <<<"$pr")
state=$(jq -r '.state' <<<"$pr")
url=$(jq -r '.url' <<<"$pr")

if [[ "$state" != "OPEN" ]]; then
    echo "ℹ️ Pull request ${url} is ${state}; not requesting a Copilot review"
    exit 0
fi

gh pr edit "$number" --add-reviewer @copilot
echo "✅ Requested Copilot review on ${url}"
