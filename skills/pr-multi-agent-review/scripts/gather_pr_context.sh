#!/usr/bin/env bash
#
# gather_pr_context.sh — preflight checks + gather all PR context for the review panel.
#
# Usage:
#   gather_pr_context.sh --preflight-only   # prints `export VAR=...` lines; exit 1 on failure
#   gather_pr_context.sh <WORKDIR>          # writes <WORKDIR>/context/* and meta.env
#
# Everything every reviewer reads is produced here, so the panel sees identical inputs.

set -euo pipefail

err() {
    echo "$*" >&2
}

# --- Resolve the base/feature branch (shared by both modes) -------------------
detect_branches() {
    # The orchestrator calls this script twice (once --preflight-only, once for
    # the full gather), so it would otherwise fetch twice. Set PR_REVIEW_SKIP_FETCH=1
    # on the second call to reuse the refs the first one already updated.
    if [ "${PR_REVIEW_SKIP_FETCH:-0}" != "1" ]; then
        git fetch origin --quiet 2>/dev/null || true
    fi

    # `|| true` on each probe: under `set -e` + `pipefail` a failing git call
    # (e.g. no `origin` remote) would otherwise abort the whole script before
    # our own friendly error messages get a chance to run.
    DEFAULT_BRANCH="$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|refs/remotes/origin/||' || true)"
    if [ -z "${DEFAULT_BRANCH}" ] && git show-ref --verify --quiet refs/heads/main; then
        DEFAULT_BRANCH=main
    fi
    if [ -z "${DEFAULT_BRANCH}" ] && git show-ref --verify --quiet refs/heads/master; then
        DEFAULT_BRANCH=master
    fi
    if [ -z "${DEFAULT_BRANCH}" ]; then
        err "❌ Could not determine the default branch (tried origin HEAD, main, master)."
        exit 1
    fi

    CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
    if [ "${CURRENT_BRANCH}" = "${DEFAULT_BRANCH}" ] || [ "${CURRENT_BRANCH}" = "HEAD" ]; then
        err "❌ You appear to be on ${DEFAULT_BRANCH} (or detached HEAD), not a feature branch. Nothing to review."
        exit 1
    fi

    # Prefer the remote base — local branches drift.
    if git show-ref --verify --quiet "refs/remotes/origin/${DEFAULT_BRANCH}"; then
        BASE="origin/${DEFAULT_BRANCH}"
    else
        BASE="${DEFAULT_BRANCH}"
    fi

    if [ -z "$(git log "${BASE}..HEAD" --oneline 2>/dev/null)" ]; then
        err "❌ No commits found between ${BASE} and HEAD. Make sure you are on a feature branch with commits ahead of ${DEFAULT_BRANCH}."
        exit 1
    fi
}

# --- Preflight-only mode ------------------------------------------------------
if [ "${1:-}" = "--preflight-only" ]; then
    detect_branches
    # Use `printf %q` so the values are shell-safe to eval/source even when a branch
    # name contains metacharacters. git permits `'`, `;`, `$()`, etc. in ref names,
    # so naive `export VAR='${branch}'` would let a branch like `x';id;#` execute
    # code in the caller. %q emits a guaranteed-safe quoted token instead.
    printf 'DEFAULT_BRANCH=%q\n' "${DEFAULT_BRANCH}"
    printf 'CURRENT_BRANCH=%q\n' "${CURRENT_BRANCH}"
    printf 'BASE=%q\n' "${BASE}"
    exit 0
fi

# --- Full context-gathering mode ----------------------------------------------
WORKDIR="${1:-}"
if [ -z "${WORKDIR}" ]; then
    err "❌ Usage: gather_pr_context.sh <WORKDIR> | --preflight-only"
    exit 1
fi

detect_branches

CTX="${WORKDIR}/context"
mkdir -p "${CTX}" "${WORKDIR}/reviews"

# Diff, commits, changed files — the ground truth of the change.
git diff "${BASE}...HEAD" >"${CTX}/diff.patch"
git log "${BASE}..HEAD" --format="%h %an %s%n%b%n---" >"${CTX}/commits.txt"
git diff "${BASE}...HEAD" --name-status >"${CTX}/changed-files.txt"

# --- PR description + unresolved threads (best-effort; needs gh + a PR) --------
OWNER="" REPO="" PR_NUMBER="" PR_URL=""

write_no_pr() {
    local reason="$1"
    {
        echo "# PR Description"
        echo
        echo "_${reason}_"
        echo
        echo "This review is based on the local diff and commit messages only."
    } >"${CTX}/pr-description.md"
    {
        echo "# Unresolved GitHub Review Threads"
        echo
        echo "_${reason}_"
    } >"${CTX}/unresolved-threads.md"
}

if ! command -v gh >/dev/null 2>&1; then
    write_no_pr "gh CLI not found — no GitHub PR context available."
elif ! command -v jq >/dev/null 2>&1; then
    # We parse every gh payload with jq. Without it, the first jq call below would
    # abort the whole script under `set -e` *after* writing diff/commits/changed-files
    # but *before* the PR description and threads — a half-populated workspace. Guard
    # here so a missing jq degrades to best-effort, exactly like a missing gh.
    write_no_pr "jq not found — install jq for PR description and review-thread context."
elif ! PR_JSON="$(gh pr view --json number,url,title,body,author,labels,additions,deletions 2>/dev/null)"; then
    write_no_pr "No GitHub PR is associated with branch '${CURRENT_BRANCH}' yet."
else
    PR_NUMBER="$(echo "${PR_JSON}" | jq -r '.number')"
    PR_URL="$(echo "${PR_JSON}" | jq -r '.url')"

    # Owner/repo for the GraphQL call.
    REPO_FULL="$(gh repo view --json nameWithOwner --jq '.nameWithOwner' 2>/dev/null || true)"
    OWNER="${REPO_FULL%%/*}"
    REPO="${REPO_FULL##*/}"

    # PR description.
    {
        echo "# PR Description"
        echo
        echo "**Title:** $(echo "${PR_JSON}" | jq -r '.title')"
        echo "**Author:** $(echo "${PR_JSON}" | jq -r '.author.login')"
        echo "**URL:** ${PR_URL}"
        echo "**Labels:** $(echo "${PR_JSON}" | jq -r 'if (.labels | length) == 0 then "none" else ([.labels[].name] | join(", ")) end')"
        echo "**Changes:** +$(echo "${PR_JSON}" | jq -r '.additions') / -$(echo "${PR_JSON}" | jq -r '.deletions')"
        echo
        echo "## Body"
        echo
        echo "${PR_JSON}" | jq -r '.body // "_(no description provided)_"'
    } >"${CTX}/pr-description.md"

    # Unresolved review threads (humans AND the Copilot bot) via GraphQL.
    # Caps at 100 threads / 30 comments each — far beyond any realistic PR, so not
    # paginated; a PR exceeding that would silently miss the overflow.
    # shellcheck disable=SC2016  # $owner/$repo/$pr are GraphQL variables (bound via -f/-F), not shell vars — must stay literal.
    THREADS_JSON="$(gh api graphql \
        -f owner="${OWNER}" -f repo="${REPO}" -F pr="${PR_NUMBER}" \
        -f query='
        query($owner:String!, $repo:String!, $pr:Int!) {
          repository(owner:$owner, name:$repo) {
            pullRequest(number:$pr) {
              reviewThreads(first:100) {
                nodes {
                  isResolved
                  isOutdated
                  path
                  line
                  comments(first:30) {
                    nodes { author { login } body }
                  }
                }
              }
            }
          }
        }' 2>/dev/null || echo '')"

    {
        echo "# Unresolved GitHub Review Threads"
        echo
        # The `?` and `// []` below make this null-safe: a 200-with-errors response
        # where `pullRequest` (or any ancestor) is null — e.g. a token lacking the
        # `read:pull_request` scope — yields an empty node list instead of jq
        # iterating null, exiting non-zero, and aborting the script before meta.env.
        if [ -z "${THREADS_JSON}" ]; then
            echo "_Could not fetch review threads (GraphQL call failed — check the token's read:pull_request scope)._"
        else
            COUNT="$(echo "${THREADS_JSON}" | jq '[(.data.repository.pullRequest.reviewThreads.nodes? // [])[] | select(.isResolved==false)] | length' 2>/dev/null || echo 0)"
            if [ "${COUNT}" = "0" ]; then
                echo "_No unresolved review threads. 🎉_"
            else
                echo "${COUNT} unresolved thread(s). Each reviewer should judge whether the current diff addresses these."
                echo
                echo "${THREADS_JSON}" | jq -r '
                  (.data.repository.pullRequest.reviewThreads.nodes? // [])[]
                  | select(.isResolved==false)
                  | "## `\(.path // "general"):\(.line // "?")`\((if .isOutdated then " _(outdated)_" else "" end))\n"
                    + ([(.comments.nodes? // [])[] | "- **\(.author.login // "unknown")**: \(.body | gsub("\n"; " "))"] | join("\n"))
                    + "\n"' 2>/dev/null || echo "_(failed to format threads)_"
            fi
        fi
    } >"${CTX}/unresolved-threads.md"
fi

# --- meta.env for the orchestrator --------------------------------------------
# `printf %q` for the same reason as preflight: these values (branch names, and
# OWNER/REPO/URL derived from GitHub data) are sourced by the orchestrator, so they
# must be shell-safe regardless of metacharacters.
{
    printf 'OWNER=%q\n' "${OWNER}"
    printf 'REPO=%q\n' "${REPO}"
    printf 'PR_NUMBER=%q\n' "${PR_NUMBER}"
    printf 'PR_URL=%q\n' "${PR_URL}"
    printf 'BASE=%q\n' "${BASE}"
    printf 'CURRENT_BRANCH=%q\n' "${CURRENT_BRANCH}"
    printf 'DEFAULT_BRANCH=%q\n' "${DEFAULT_BRANCH}"
} >"${CTX}/meta.env"

echo "✅ Context gathered in ${CTX}"
