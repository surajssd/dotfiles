#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "$(realpath "${BASH_SOURCE[0]}")")"/util.sh

interval=30
once=0

usage() {
    cat <<HELP
Usage: $(basename "$0") [--once] [-i seconds] [-h]

Fetch the current PR branch, rebase onto the remote default branch, and
push through gpo.sh with an explicit lease. Run from a clean worktree of
an open PR targeting the default branch, with all commits pushed to the
same branch on origin. GitHub CLI authentication is required.
The complete PR diff must stay identical. Existing GitHub approvals are
checked after pushing, but GitHub can still dismiss them.

Flags:
  --once       Run one cycle and exit (default: keep watching)
  -i seconds   Poll interval (default: ${interval})
  -h, --help   Show this help
HELP
}

die() {
    err "❌ $*"
    exit 1
}

while [[ $# -gt 0 ]]; do
    case "$1" in
    --once) once=1 ;;
    -i)
        [[ $# -ge 2 ]] || die '-i requires seconds'
        interval=$2
        shift
        ;;
    -i[0-9]*) interval=${1#-i} ;;
    -h | --help)
        usage
        exit 0
        ;;
    *)
        usage
        die "Unknown argument: $1"
        ;;
    esac
    shift
done
[[ $interval =~ ^[1-9][0-9]*$ ]] || die 'Poll interval must be a positive integer'

branch=$(git branch --show-current)
[[ -n $branch ]] || die 'Run from a PR branch, not a detached HEAD'
git remote get-url origin >/dev/null || die 'gpo.sh requires an origin remote'
for command in gpo.sh gh jq; do
    command -v "$command" >/dev/null || die "$command is required"
done
export GH_PROMPT_DISABLED=1 GH_PAGER=cat

# Prefer upstream over origin.
if git remote get-url upstream &>/dev/null; then
    remote=upstream
else
    remote=origin
fi
default=$(git ls-remote --symref "$remote" HEAD | awk '/^ref:/ {sub("refs/heads/", "", $2); print $2}')
[[ -n $default ]] || die "Could not determine the default branch of ${remote}"
[[ $branch != "$default" ]] || die 'Run from a PR branch, not the default branch'

# Root of the main checkout, even when running from a worktree.
main_repo=$(dirname "$(git rev-parse --path-format=absolute --git-common-dir)")
if [[ -x /opt/homebrew/opt/coreutils/libexec/gnubin/mktemp ]]; then
    scratch=$(/opt/homebrew/opt/coreutils/libexec/gnubin/mktemp -d --suffix=.auto-rebase)
else
    scratch=$(mktemp -d)
fi
trap 'rm -rf "$scratch"' EXIT

echo "ℹ️ Remote: ${remote}"
echo "ℹ️ Default branch: ${default}"
echo "ℹ️ Main repo: ${main_repo}"

sync_notes() {
    notes_expected=$(git ls-remote origin refs/notes/commits | awk '{print $1}')
    if [[ -n $notes_expected ]]; then
        git fetch --quiet origin refs/notes/commits
        notes_expected=$(git rev-parse FETCH_HEAD)
        local local_notes
        local_notes=$(git rev-parse --verify refs/notes/commits 2>/dev/null || true)
        if [[ -z $local_notes ]] || git merge-base --is-ancestor "$local_notes" "$notes_expected"; then
            git update-ref refs/notes/commits "$notes_expected" "$local_notes"
        elif ! git merge-base --is-ancestor "$notes_expected" "$local_notes"; then
            die 'Local and remote commit notes diverge; merge them before retrying'
        fi
    fi
}

pr_diff() {
    git diff --no-relative --no-ext-diff --no-textconv --no-color --no-renames --binary --full-index \
        --ignore-submodules=none --submodule=short \
        --src-prefix=a/ --dst-prefix=b/ "$(git merge-base HEAD "$target")" HEAD
}

verify_approvals() {
    local pushed attempt missing
    pushed=$(git rev-parse HEAD)
    for attempt in 1 2 3; do
        gh pr view "$branch" --json state,headRefOid,latestReviews,url >"$scratch/after.json" ||
            die 'Push succeeded, but GitHub approval verification failed; check the PR'
        if jq -e --arg head "$pushed" '.state == "OPEN" and .headRefOid == $head' "$scratch/after.json" >/dev/null; then
            missing=$(jq -r --slurpfile after "$scratch/after.json" \
                '([.latestReviews[] | select(.state == "APPROVED") | .author.login] -
                  [$after[0].latestReviews[] | select(.state == "APPROVED") | .author.login]) | join(", ")' "$scratch/before.json")
            [[ -z $missing ]] || die "Push succeeded, but approvals are no longer present: $missing"
            if jq -e 'any(.latestReviews[]; .state == "APPROVED")' "$scratch/before.json" >/dev/null; then
                echo '✅ Push succeeded; all previously observed approvals remain present'
            else
                echo '✅ Push succeeded; the PR had no existing approvals to verify'
            fi
            return
        fi
        [[ $attempt == 3 ]] || sleep 2
    done
    die 'Push succeeded, but GitHub did not report an open PR at the pushed commit; check the PR'
}

rebase_once() {
    [[ $(git branch --show-current) == "$branch" ]] || die 'The checked-out branch changed; stopping'
    [[ -z $(git status --porcelain) ]] || die 'Commit or stash worktree changes before rebasing'
    [[ ! -d $(git rev-parse --git-path rebase-merge) && ! -d $(git rev-parse --git-path rebase-apply) ]] ||
        die 'Finish the current rebase before retrying'

    git fetch --quiet "$remote" "+refs/heads/$default:refs/remotes/$remote/$default"
    git fetch --quiet origin "+refs/heads/$branch:refs/remotes/origin/$branch"
    local expected target behind
    expected=$(git rev-parse "refs/remotes/origin/$branch")
    target=$(git rev-parse "refs/remotes/$remote/$default")
    git merge-base --is-ancestor HEAD "$expected" ||
        die "The branch has unpublished commits or diverges from origin/$branch; reconcile it before retrying"
    git merge --ff-only --quiet "$expected"
    [[ $(git rev-parse HEAD) == "$expected" ]] || die 'The local branch changed during fast-forward; stopping'

    # Keep the local default branch in the main checkout up to date.
    if [[ $(git -C "$main_repo" branch --show-current) == "$default" ]]; then
        git -C "$main_repo" merge --ff-only --quiet "$target" ||
            echo "ℹ️ Could not fast-forward ${default} in ${main_repo}"
    else
        echo "ℹ️ ${main_repo} is not on ${default}, skipping local branch update"
    fi

    behind=$(git rev-list --count "HEAD..$target")
    if [[ $behind == 0 ]]; then
        echo 'ℹ️ The branch is not behind'
        return
    fi

    gh pr view "$branch" --json state,headRefOid,baseRefName,latestReviews,url >"$scratch/before.json" ||
        die 'Could not read the PR; check gh authentication and that this branch has an open PR'
    jq -e --arg head "$expected" --arg base "$default" \
        '.state == "OPEN" and .headRefOid == $head and .baseRefName == $base' "$scratch/before.json" >/dev/null ||
        die 'GitHub did not report an open PR matching the fetched head and default base; retry after checking the PR'
    sync_notes
    pr_diff >"$scratch/before.patch"
    echo "ℹ️ $(date) branch is ${behind} commits behind ${remote}/${default}, rebasing"
    if ! GIT_SEQUENCE_EDITOR=true GIT_NOTES_REWRITE_REF=refs/notes/commits GIT_NOTES_REWRITE_MODE=concatenate \
        git -c notes.rewrite.rebase=true rebase -S -i --no-autosquash --no-update-refs --no-autostash "$target"; then
        git rebase --abort || true
        if command -v osascript >/dev/null; then
            osascript -e 'display notification "Rebase hit conflicts; check the terminal." with title "auto-rebase"' || true
        fi
        die 'Rebase failed; nothing was pushed. Check the worktree before retrying'
    fi
    pr_diff >"$scratch/after.patch"
    cmp -s "$scratch/before.patch" "$scratch/after.patch" ||
        die 'PR diff changed; the local rebase is retained, but nothing was pushed. Inspect it before retrying'

    local push_args=(--atomic "--force-with-lease=refs/heads/$branch:$expected")
    if git show-ref --verify --quiet refs/notes/commits; then
        push_args+=("--force-with-lease=refs/notes/commits:$notes_expected" refs/notes/commits)
    fi
    gpo.sh "${push_args[@]}" || die 'Push failed; fetch and inspect the remote before retrying'
    verify_approvals
}

while true; do
    rebase_once
    [[ $once == 0 ]] || break
    echo "ℹ️ $(date). Sleeping for ${interval}s..."
    sleep "$interval"
done
