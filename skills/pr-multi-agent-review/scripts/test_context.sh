#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -x /opt/homebrew/opt/coreutils/libexec/gnubin/mktemp ]]; then
    TEST_ROOT="$(/opt/homebrew/opt/coreutils/libexec/gnubin/mktemp -d --suffix=.dir)"
else
    TEST_ROOT="$(mktemp -d)"
fi
trap 'rm -rf "$TEST_ROOT"' EXIT
mkdir -p "$TEST_ROOT/bin"
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
export TEST_PR_RESPONSE="$TEST_ROOT/pr.json" TEST_GH_MODE=pr
export PATH="$TEST_ROOT/bin:$PATH"

cat >"$TEST_ROOT/bin/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
case "$1 $2" in
    "pr view")
        [[ "$TEST_GH_MODE" != no-pr ]] || exit 1
        [[ "$TEST_GH_MODE" != list ]] || exit 1
        if [[ "${3:-}" == 999 ]]; then
            jq '.headRefName = "unrelated"' "$TEST_PR_RESPONSE"
        else
            cat "$TEST_PR_RESPONSE"
        fi
        ;;
    "pr list")
        if [[ "$TEST_GH_MODE" == list ]]; then jq '[.]' "$TEST_PR_RESPONSE"; else printf '[]\n'; fi
        ;;
    "repo view") printf '{"nameWithOwner":"example/repo"}\n' ;;
    "api graphql") printf '{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[]}}}}}\n' ;;
    *) exit 1 ;;
esac
EOF
chmod +x "$TEST_ROOT/bin/gh"
git init -q -b main "$TEST_ROOT/repo"
cd "$TEST_ROOT/repo"
git config user.name Test
git config user.email test@example.com
git config commit.gpgsign false
git config core.hooksPath /dev/null
echo base >base.txt
git add .
git commit -qm base
git checkout -qb release
echo release >release.txt
git add .
git commit -qm release
git checkout -qb feature
echo feature >feature.txt
git add .
git commit -qm feature
jq -n --arg head "$(git rev-parse HEAD)" '{number: 1, url: "https://github.com/example/repo/pull/1", title: "Feature", body: "Requested feature", author: {login: "test"}, labels: [], additions: 1, deletions: 0, baseRefName: "release", headRefName: "feature", headRefOid: $head}' >"$TEST_PR_RESPONSE"

gather() {
    PR_REVIEW_SKIP_FETCH=1 "$SCRIPT_DIR/gather_pr_context.sh" "$1" >/dev/null
}

gather "$TEST_ROOT/release"
diff -u <(printf 'A\tfeature.txt\n') "$TEST_ROOT/release/context/changed-files.txt"
# shellcheck disable=SC1091
source "$TEST_ROOT/release/context/meta.env"
[[ "$BASE" == release && "$HEAD_SHA" == "$(git rev-parse HEAD)" && "$MERGE_BASE" == "$(git rev-parse release)" ]]
[[ "$BASE_SHA" == "$MERGE_BASE" && "$REPO_ROOT" == "$(pwd -P)" ]]
PR_REVIEW_SKIP_FETCH=1 "$SCRIPT_DIR/gather_pr_context.sh" --preflight-only >"$TEST_ROOT/preflight.env"
# shellcheck disable=SC1091
source "$TEST_ROOT/preflight.env"
[[ "$BASE" == release ]]
echo '✅ PR base and snapshot metadata'

git config branch.feature.prNumber 999
gather "$TEST_ROOT/breadcrumb"
diff -u "$TEST_ROOT/release/context/diff.patch" "$TEST_ROOT/breadcrumb/context/diff.patch"
git config --unset branch.feature.prNumber
echo '✅ Stale PR breadcrumb falls through'

TEST_GH_MODE=list gather "$TEST_ROOT/list"
diff -u "$TEST_ROOT/release/context/diff.patch" "$TEST_ROOT/list/context/diff.patch"
echo '✅ Branch search recovers a PR when view fails'

TEST_GH_MODE=no-pr gather "$TEST_ROOT/fallback"
diff -u <(printf 'A\tfeature.txt\nA\trelease.txt\n') "$TEST_ROOT/fallback/context/changed-files.txt"
echo '✅ Default branch fallback without a PR'

git update-ref refs/remotes/origin/release "$(git rev-parse release)"
gather "$TEST_ROOT/remote"
# shellcheck disable=SC1091
source "$TEST_ROOT/remote/context/meta.env"
[[ "$BASE" == origin/release ]]
git update-ref -d refs/remotes/origin/release
echo '✅ Remote PR base preferred when available'

jq '.baseRefName = "missing-target"' "$TEST_PR_RESPONSE" >"$TEST_ROOT/missing.json"
if TEST_PR_RESPONSE="$TEST_ROOT/missing.json" gather "$TEST_ROOT/missing" 2>"$TEST_ROOT/missing.err"; then exit 1; fi
rg -q -F 'Base missing-target is unavailable' "$TEST_ROOT/missing.err"
echo '✅ Missing PR base fails instead of reviewing the default branch'

printf 'Rule from CONTRIBUTING.md:7\n   ~~~~  \nKeep this rule.\n' >"$TEST_ROOT/release/context/standards.md"
printf 'Requirement from specs/feature.md:3\nPreserve ordering.\n' >"$TEST_ROOT/release/context/spec.md"
"$SCRIPT_DIR/build_prompt.sh" --no-diff "$TEST_ROOT/release/context" "$SCRIPT_DIR/../references/review-prompt.md" >"$TEST_ROOT/prompt.txt"
rg -q -F 'Rule from CONTRIBUTING.md:7' "$TEST_ROOT/prompt.txt"
rg -q -F 'Requirement from specs/feature.md:3' "$TEST_ROOT/prompt.txt"
rg -q '^~~~~~$' "$TEST_ROOT/prompt.txt"
if rg -q '^diff --git' "$TEST_ROOT/prompt.txt"; then exit 1; fi
"$SCRIPT_DIR/build_prompt.sh" "$TEST_ROOT/release/context" "$SCRIPT_DIR/../references/review-prompt.md" >"$TEST_ROOT/full-prompt.txt"
rg -q -F 'diff --git a/feature.txt b/feature.txt' "$TEST_ROOT/full-prompt.txt"
echo '✅ Shared standards and spec context, safe fences, and diff delivery'

git checkout -qb empty release
echo transient >temporary.txt
git add .
git commit -qm temporary
git rm -q temporary.txt
git commit -qm remove-temporary
jq --arg head "$(git rev-parse HEAD)" '.headRefName = "empty" | .headRefOid = $head' "$TEST_PR_RESPONSE" >"$TEST_ROOT/empty.json"
if TEST_PR_RESPONSE="$TEST_ROOT/empty.json" gather "$TEST_ROOT/empty" 2>"$TEST_ROOT/empty.err"; then exit 1; fi
rg -q -F 'No changes found' "$TEST_ROOT/empty.err"
echo '✅ Empty final diff rejected'
