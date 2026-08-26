#!/usr/bin/env bash
set -eu

interval=30

usage() {
    cat <<EOF
Usage: $(basename "$0") [-i seconds] [-h]

Watch the current branch and rebase it onto the remote default branch
whenever it falls behind, then run 'gpo.sh -f'. Run it from the worktree
of the PR branch.

Flags:
  -i seconds   Poll interval (default: ${interval})
  -h           Show this help
EOF
}

while getopts "i:h" opt; do
    case "$opt" in
    i) interval="$OPTARG" ;;
    h)
        usage
        exit 0
        ;;
    *)
        usage
        exit 1
        ;;
    esac
done

notify() {
    osascript -e "display notification \"$1\" with title \"auto-rebase\""
}

# Prefer upstream over origin.
if git remote get-url upstream &>/dev/null; then
    remote="upstream"
elif git remote get-url origin &>/dev/null; then
    remote="origin"
else
    echo "❌ No upstream or origin remote found"
    exit 1
fi

default=$(git ls-remote --symref "$remote" HEAD | awk '/^ref:/ {sub("refs/heads/", "", $2); print $2}')
if [ -z "$default" ]; then
    echo "❌ Could not determine the default branch of ${remote}"
    exit 1
fi

# Root of the main checkout, even when running from a worktree.
main_repo=$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")

echo "ℹ️ Remote: ${remote}"
echo "ℹ️ Default branch: ${default}"
echo "ℹ️ Main repo: ${main_repo}"

while true; do
    git fetch "$remote" "$default" --quiet

    # Keep the local default branch in the main checkout up to date.
    if [ "$(git -C "$main_repo" branch --show-current)" = "$default" ]; then
        git -C "$main_repo" merge --ff-only --quiet "${remote}/${default}" ||
            echo "⚠️ Could not fast-forward ${default} in ${main_repo}"
    else
        echo "⚠️ ${main_repo} is not on ${default}, skipping local branch update"
    fi

    behind=$(git rev-list --count "HEAD..${remote}/${default}")
    if [ "$behind" -gt 0 ]; then
        echo "ℹ️ $(date) branch is ${behind} commits behind ${remote}/${default}, rebasing"
        if GIT_SEQUENCE_EDITOR=true git rebase -S -i "${remote}/${default}"; then
            gpo.sh -f
        else
            echo "❌ Rebase failed, aborting"
            git rebase --abort || true
            notify "Rebase onto ${remote}/${default} hit conflicts in $(basename "$PWD")"
            exit 1
        fi
    else
        echo "ℹ️ The branch is not behind"
    fi
    echo "ℹ️ $(date). Sleeping for ${interval}s..."
    sleep "$interval"
done
