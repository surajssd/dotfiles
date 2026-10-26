#!/usr/bin/env bash
#
# gather_pr_context.sh — preflight checks + gather all PR context for the review panel.
#
# Usage:
#   gather_pr_context.sh --preflight-only   # prints shell-safe VAR=... lines; exit 1 on failure
#   gather_pr_context.sh <WORKDIR>          # writes <WORKDIR>/context/* and meta.env
#
# Everything every reviewer reads is produced here, so the panel sees identical inputs.

set -euo pipefail

err() {
    echo "$*" >&2
}

select_pr() {
    local candidate="$1" head_name head_sha
    printf '%s\n' "$candidate" | jq -e 'type == "object" and (.number | type == "number") and (.baseRefName | type == "string" and length > 0) and (.headRefName | type == "string") and (.headRefOid | type == "string")' >/dev/null 2>&1 || return 1
    head_name="$(printf '%s\n' "$candidate" | jq -r '.headRefName')"
    head_sha="$(printf '%s\n' "$candidate" | jq -r '.headRefOid')"
    [[ "$head_name" == "$CURRENT_BRANCH" ]] || return 1
    if [[ "$head_sha" != "$HEAD_SHA" ]]; then
        git merge-base --is-ancestor "$HEAD_SHA" "$head_sha" 2>/dev/null || return 1
        err "ℹ️ PR head is newer than local HEAD; reviewing local ${HEAD_SHA}."
    fi
    PR_JSON="$candidate"
}

resolve_pr() {
    PR_JSON=""
    command -v gh >/dev/null 2>&1 && command -v jq >/dev/null 2>&1 || return 0
    local fields pr_number candidate candidates
    fields=number,url,title,body,author,labels,additions,deletions,baseRefName,headRefName,headRefOid
    pr_number="$(git config "branch.${CURRENT_BRANCH}.prNumber" 2>/dev/null || true)"
    if [[ -n "$pr_number" ]]; then
        select_pr "$(gh pr view "$pr_number" --json "$fields" 2>/dev/null || true)" || true
    fi
    if [[ -z "$PR_JSON" ]]; then
        select_pr "$(gh pr view --json "$fields" 2>/dev/null || true)" || true
    fi
    if [[ -z "$PR_JSON" ]]; then
        candidates="$(gh pr list --head "$CURRENT_BRANCH" --state all --json "$fields" 2>/dev/null || true)"
        while IFS= read -r candidate; do
            if select_pr "$candidate"; then break; fi
        done < <(printf '%s\n' "$candidates" | jq -c --arg head "$HEAD_SHA" 'sort_by(.headRefOid != $head)[]' 2>/dev/null || true)
    fi
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
    REPO_ROOT="$(git rev-parse --show-toplevel)"
    HEAD_SHA="$(git rev-parse HEAD)"
    CURRENT_BRANCH="$(git rev-parse --abbrev-ref HEAD)"
    if [[ "$CURRENT_BRANCH" == HEAD ]]; then
        err "❌ Detached HEAD does not identify a feature branch."
        exit 1
    fi

    resolve_pr
    BASE_BRANCH="$DEFAULT_BRANCH"
    if [[ -n "$PR_JSON" ]]; then
        BASE_BRANCH="$(printf '%s\n' "$PR_JSON" | jq -r '.baseRefName')"
    fi
    if [[ -z "$BASE_BRANCH" ]]; then
        err "❌ Could not determine the PR base or default branch (tried origin HEAD, main, master)."
        exit 1
    fi
    if [[ "$CURRENT_BRANCH" == "$BASE_BRANCH" ]]; then
        err "❌ You are on the base branch ${BASE_BRANCH}, not a feature branch."
        exit 1
    fi

    # Prefer the remote base — local branches drift.
    if git show-ref --verify --quiet "refs/remotes/origin/${BASE_BRANCH}"; then
        BASE="origin/${BASE_BRANCH}"
    else
        BASE="${BASE_BRANCH}"
        err "ℹ️ Using local base ${BASE}; no matching origin ref is available."
    fi
    if ! BASE_SHA="$(git rev-parse --verify "${BASE}^{commit}" 2>/dev/null)"; then
        err "❌ Base ${BASE} is unavailable; fetch the PR target before reviewing."
        exit 1
    fi
    MERGE_BASE="$(git merge-base "$BASE_SHA" "$HEAD_SHA")"
    if git diff --quiet "$MERGE_BASE" "$HEAD_SHA"; then
        err "❌ No changes found between the merge base of ${BASE} and HEAD."
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
    printf 'BASE_BRANCH=%q\n' "${BASE_BRANCH}"
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
git diff "$MERGE_BASE" "$HEAD_SHA" >"${CTX}/diff.patch"
git log "${BASE_SHA}..${HEAD_SHA}" --format="%h %an %s%n%b%n---" >"${CTX}/commits.txt"
git diff "$MERGE_BASE" "$HEAD_SHA" --name-status >"${CTX}/changed-files.txt"

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
elif [[ -z "$PR_JSON" ]]; then
    write_no_pr "No matching GitHub PR was found for branch '${CURRENT_BRANCH}' and its local history."
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
        # Distinguish three outcomes so a failed/permission-denied fetch is never
        # silently rendered as "no unresolved threads 🎉" (which would read as a clean
        # PR): (1) the call itself failed → empty THREADS_JSON; (2) GraphQL returned a
        # top-level `.errors` array, or `pullRequest` is null (typically a missing
        # `read:pull_request` scope or no visibility) → fetch problem; (3) a real
        # payload → count and render.
        if [ -z "${THREADS_JSON}" ]; then
            echo "_Could not fetch review threads (GraphQL call failed — check the token's read:pull_request scope)._"
        elif [ "$(echo "${THREADS_JSON}" | jq -r 'if (.errors | length // 0) > 0 then "err" elif .data.repository.pullRequest == null then "err" else "ok" end' 2>/dev/null || echo err)" = "err" ]; then
            ERRMSG="$(echo "${THREADS_JSON}" | jq -r '(.errors[0].message // "pullRequest not visible") | gsub("\n"; " ")' 2>/dev/null || echo "unknown error")"
            echo "_Could not fetch review threads: ${ERRMSG}. (Often a missing \`read:pull_request\` token scope.) Treat thread status as UNKNOWN, not resolved._"
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
                    + ([(.comments.nodes? // [])[] | "- **\(.author.login // "unknown")**: \((.body // "") | gsub("\n"; " "))"] | join("\n"))
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
    printf 'BASE_BRANCH=%q\n' "${BASE_BRANCH}"
    printf 'BASE_SHA=%q\n' "${BASE_SHA}"
    printf 'MERGE_BASE=%q\n' "${MERGE_BASE}"
    printf 'HEAD_SHA=%q\n' "${HEAD_SHA}"
    printf 'REPO_ROOT=%q\n' "${REPO_ROOT}"
    printf 'CURRENT_BRANCH=%q\n' "${CURRENT_BRANCH}"
    printf 'DEFAULT_BRANCH=%q\n' "${DEFAULT_BRANCH}"
} >"${CTX}/meta.env"

echo "✅ Context gathered in ${CTX}"
