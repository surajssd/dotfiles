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

# If this failed, usually it means we already have an outdated branch from the PR.
if ! gh pr checkout "${PR_NUMBER}" --repo "$(get_origin_repo)"; then
    pr_branch=$(git branch --show-current)
    if [[ "$pr_branch" == "main" ]]; then
        echo "❌ [ERROR]: gh failed"
        exit 1
    fi

    git checkout main
    git branch -D "${pr_branch}"
    git push origin --delete "${pr_branch}"
    echo "ℹ️ [INFO]: Deleted outdated branch ${pr_branch} and checking out main."
    gh pr checkout "${PR_NUMBER}" --repo "$(get_origin_repo)"
fi

new_branch_name=$(convert_branch_name)
# If this failed, it means that the branch already exists and that is because we
# already created and pushed it and probably it is outdated now.
if ! git checkout -b "${new_branch_name}"; then
    git branch -D "${new_branch_name}"
    git push origin --delete "${new_branch_name}" || true
    echo "ℹ️ [INFO]: Deleted outdated branch ${new_branch_name} and checking out main."
    git checkout -b "${new_branch_name}"
fi

git push -u origin "$(git branch --show-current)"
