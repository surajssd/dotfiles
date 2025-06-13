#!/usr/bin/env bash
# When renovate creates a PR on origin, and you wanna push it to upstream
# because upstream does not have the renovate bot enabled. This script can come
# in handy.

# Error saying that the PR number is not provided.
PR_NUMBER=${1}
shift
if [[ -z "${PR_NUMBER}" ]]; then
    echo "❌ [ERROR]: Please provide the PR number as the first argument:"
    echo "Usage: $0 <PR_NUMBER>"
    exit 1
fi

set -euo pipefail

function get_origin_repo() {
    git remote get-url origin | cut -d':' -f2 | cut -d'.' -f1
}

function convert_branch_name() {
    current_branch=$(git branch --show-current)

    # Let's replace the / with - in the branch name and return it.
    if [[ "$current_branch" == *"/"* ]]; then
        echo "${current_branch//\//-}"
    else
        echo "$current_branch"
    fi
}

gh pr checkout "${PR_NUMBER}" --repo "$(get_origin_repo)"
git checkout -b "$(convert_branch_name)" || true
git push -u origin "$(git branch --show-current)"
