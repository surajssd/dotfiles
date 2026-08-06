---
name: pr-review-dashboard
description: Review, explain, or assess the Pull Request represented by the currently checked-out feature branch in a local git repository. Use for requests such as "review this PR", "explain this PR", "what does this PR do", or "review the current branch". Produce an evidence-backed, interactive single-file HTML dashboard with an explicit review recommendation, prioritized findings, architecture views, annotated diffs, verification results, and unknowns. Do not use for pasted snippets, a remote PR that is not checked out, or unrelated working-tree code.
---

# PR Review Dashboard

Act as a staff-level code reviewer. Produce a decision aid that helps an unfamiliar reviewer understand the change, verify its safety, and know what to do next.

Put correctness, evidence, and actionable findings ahead of visual quantity. Use diagrams to compress relationships that prose cannot; do not use them as decoration. Do not modify the reviewed code, submit a GitHub review, or post comments unless the user explicitly asks.

Read [references/review-method.md](references/review-method.md) completely before evaluating the change. For any non-trivial PR, also read [references/diagram-selection.md](references/diagram-selection.md) completely before choosing visualizations.

## 1. Establish the review boundary

Run from the repository root and capture the local state before reviewing:

```bash
ROOT=$(git rev-parse --show-toplevel) || exit 1
cd "$ROOT"
CURRENT=$(git symbolic-ref --quiet --short HEAD || true)
HEAD_SHA=$(git rev-parse HEAD)
git status --short
```

Stop if `CURRENT` is empty: a detached `HEAD` does not identify a feature branch reliably. Record any working-tree changes. Review only committed `$BASE...HEAD` changes unless the user explicitly includes local changes; never silently attribute unrelated uncommitted work to the PR.

Discover and read applicable repository guidance before judging the code. Check root and changed-path scopes for `AGENTS.md`, `CONTRIBUTING*`, review guides, architecture docs, test instructions, and language-specific conventions. Prefer `rg --files` for discovery.

Fetch remote refs on a best-effort basis:

```bash
git fetch origin --quiet
```

If fetching fails, continue only when a usable local base ref exists. Disclose that remote freshness was not verified.

## 2. Resolve the PR and its actual base before diffing

Resolve author intent and the target branch before gathering the diff. A PR may target `develop`, a release branch, or another non-default branch; the repository default is only a fallback.

Use the following lookup ladder. Treat an error, empty string, `null`, or `[]` as no result and continue. The explicit `wtpr` breadcrumb is the only reliable recovery path for some fork PRs and merged/closed worktree PRs.

```bash
FIELDS=url,number,state,title,body,author,labels,additions,deletions,changedFiles,baseRefName,headRefName,headRefOid,headRepositoryOwner,isCrossRepository,isDraft,reviewDecision,mergeable,statusCheckRollup
PR_JSON=""

has_pr_result() {
  [ -n "$1" ] && [ "$1" != "null" ] && [ "$1" != "[]" ]
}

PR_NUM=$(git config "branch.$CURRENT.prNumber" 2>/dev/null || true)
if [ -n "$PR_NUM" ]; then
  PR_JSON=$(gh pr view "$PR_NUM" --json "$FIELDS" 2>/dev/null || true)
fi

if ! has_pr_result "$PR_JSON"; then
  PR_JSON=$(gh pr view --json "$FIELDS" 2>/dev/null || true)
fi

if ! has_pr_result "$PR_JSON"; then
  PR_JSON=$(gh pr list --head "$CURRENT" --state all --json "$FIELDS" 2>/dev/null || true)
fi
```

Sanity-check every candidate before trusting it:

- Require `headRefName` to match `CURRENT` unless the local branch name was intentionally rewritten.
- Prefer `headRefOid == HEAD_SHA`. Accept a newer remote head only when ancestry and PR context show it is the same work; otherwise disclose the mismatch or reject the candidate.
- Treat a repo-wide `branch.<name>.prNumber` as stale when the branch name was reused or the candidate describes different work.
- When a branch-name search returns several PRs, choose the matching head SHA, then the matching lineage, then the most recent only as a disclosed fallback.

If a PR resolves, use its `baseRefName` as `BASE_BRANCH`. Capture its URL, state, draft status, author, body, checks, review decision, and mergeability. If no PR resolves, determine the default branch:

```bash
BASE_BRANCH=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|refs/remotes/origin/||')
[ -z "$BASE_BRANCH" ] && git show-ref --verify --quiet refs/heads/main && BASE_BRANCH=main
[ -z "$BASE_BRANCH" ] && git show-ref --verify --quiet refs/heads/master && BASE_BRANCH=master
```

Stop if `BASE_BRANCH` is empty. Prefer `origin/$BASE_BRANCH`; fall back to the local branch only when necessary and disclose that choice.

```bash
BASE="origin/$BASE_BRANCH"
git rev-parse --verify "$BASE^{commit}" >/dev/null 2>&1 || BASE="$BASE_BRANCH"
git rev-parse --verify "$BASE^{commit}" >/dev/null 2>&1 || exit 1
MERGE_BASE=$(git merge-base "$BASE" HEAD)
COMMIT_COUNT=$(git rev-list --count "$BASE"..HEAD)
```

Stop when `COMMIT_COUNT` is zero. If `CURRENT` equals `BASE_BRANCH`, stop unless a resolved PR and commit comparison prove this is intentional.

Treat the PR body as the author's stated intent, never as ground truth. Record mismatches between the body, issue, commits, diff, tests, and documentation.

## 3. Gather the complete change surface

Use three-dot syntax for the PR diff and two-dot syntax for branch-only commits:

```bash
git diff "$BASE"...HEAD --stat
git diff "$BASE"...HEAD --shortstat
git diff "$BASE"...HEAD --name-status --find-renames --find-copies
git diff "$BASE"...HEAD --numstat --find-renames
git diff "$BASE"...HEAD --summary
git diff "$BASE"...HEAD --check
git log "$BASE"..HEAD --format='%h %s%n%b'
```

Distinguish final branch behavior from commit history. Use `git diff "$BASE"...HEAD` as the review ground truth; use individual commits only to recover rationale or spot risky intermediate/reverted work.

Load code in proportion to size:

- **Small** — at most 10 files and 500 changed lines: read the full final diff and every changed production/test file.
- **Medium** — at most 30 files and 3,000 changed lines: read the full final diff, then focus dashboard excerpts on the consequential paths.
- **Large** — over either threshold: start with stats and file groups, then inspect every changed production path and sample generated/vendor churn. Load targeted diffs and surrounding files rather than one enormous patch.

For all sizes, inspect relevant unchanged callers, callees, interfaces, schemas, configuration, and tests with `rg`, `git show "$MERGE_BASE:<path>"`, `git diff "$BASE"...HEAD -- <path>`, and the file at `HEAD`. Do not infer a symbol's behavior from its name.

When a GitHub PR is available, inspect existing check results, reviews, and unresolved review threads on a best-effort basis. Use them to avoid duplicate work and identify disputed assumptions, but verify claims independently. Do not post anything.

## 4. Perform an evidence-led review

Follow the mandatory review method. In particular:

1. Reconstruct intent, actual behavior, invariants, non-goals, and rollout/rollback.
2. Trace affected behavior across boundaries and through success and failure paths.
3. Evaluate every relevant engineering axis, not only the axes that produced findings.
4. Map important behaviors and risks to tests; judge assertions and scenarios, not test-line count.
5. Run proportionate targeted tests, linters, type checks, builds, or static analysis already supported by the repository. Do not install dependencies or invoke external/production systems without authorization.
6. Admit a finding only after proving the PR introduces it, identifying a reachable trigger and impact, checking contrary evidence, and proposing a concrete correction or verification.

For every finding, record:

- stable ID, priority, and confidence;
- concise defect statement;
- exact file and line evidence;
- trigger/scenario and user or system impact;
- actionable recommendation;
- test or observation that would verify the correction.

Use exact `path:line` or `path:start-end` locations from the reviewed SHA. Never write approximate locations such as `~260`. Link to a commit-pinned GitHub blob when possible. Use the merge-base SHA for deleted code. Mark external contracts, author claims, and inferences explicitly.

If no actionable findings remain, say **No actionable findings**. Do not manufacture a blocker to make the dashboard look thorough. Keep residual risks and unverified assumptions separate.

## 5. Build the dashboard from the template

Copy the bundled template to a fresh directory, then replace every `FILL:` block:

```bash
SKILL_DIR=/absolute/path/to/pr-review-dashboard
OUT_DIR=$(mktemp -d "${TMPDIR:-/tmp}/pr-review-dashboard.XXXXXX")
DASHBOARD="$OUT_DIR/pr-review-dashboard.html"
cp "$SKILL_DIR/scripts/dashboard_template.html" "$DASHBOARD"
```

Do not leave the template's instructional comments in the delivered file. HTML-escape all repository-controlled or GitHub-controlled text, including titles, branch names, authors, PR bodies, comments, code, paths, and labels. Never paste untrusted text as markup or script. Do not interpolate raw untrusted text into Mermaid syntax; use short sanitized identifiers and labels, then put the exact escaped text in the surrounding caption or evidence.

Fill the tabs in this order:

1. **Summary** — Explain author intent versus actual behavior, the change contract and preserved invariants, scope, local recommendation (`Request changes`, `Comment`, or `Approve`), confidence, top finding, verification status, PR/commit metadata, and whether feedback is still actionable for merged/closed work.
2. **Findings & Risks** — Put actionable findings first, ordered by priority. Include evidence, impact, recommendation, and verification for each. Then show the full risk-coverage matrix with `High`, `Medium`, `Low`, `Not applicable`, or `Unknown`; do not conflate an assessed-clean axis with one not checked.
3. **Architecture** — Add only diagrams selected by reviewer question. Read the diagram-selection reference and cite evidence in every caption. Prefer before/after views when change is the point. Include failure, rollback, state, deployment, data, or trust boundaries when they materially affect safety.
4. **Annotated Diff** — Show all hunks for a small PR or the 3–5 most consequential hunks for a larger PR. Preserve exact hunk headers and line numbers. Tie each excerpt to a finding, invariant, or important design choice, and list every omitted file group with a one-line reason.
5. **Verification** — Record exact local commands and observed results, CI/check state, a behavior-to-test matrix, manual/e2e scenarios, and anything not run. Keep local results distinct from remote CI.
6. **Glossary** — Define only the domain terms, modules, acronyms, and contracts needed to understand the review.
7. **Assumptions & Unknowns** — List unverified external contracts, missing context, base/ref freshness, unread areas, unresolved review threads, and precise questions for the author.

## 6. Choose visualizations deliberately

Do not target a diagram count. A small local fix may need none; a cross-service migration may need several. Prefer the smallest view that changes a review decision or materially reduces cognitive load.

Use the template's HTML/CSS primitives for component maps, activity flows, before/after comparisons, schema views, and heatmaps. Use Mermaid only when its layout materially helps a sequence, state, class, or deployment diagram. Keep Mermaid node IDs alphanumeric, quote labels containing punctuation, avoid fragile pipe edge labels, and preserve readable source text as a fallback.

Every diagram must answer a question in its heading, explain changed/inferred/external elements, and cite its code evidence. Remove redundant diagrams. Never visualize a relationship that was not verified.

## 7. Validate the artifact

Run the bundled validator:

```bash
python3 "$SKILL_DIR/scripts/validate_dashboard.py" "$DASHBOARD"
```

Fix every error. Inspect the dashboard in an available browser and exercise tabs, keyboard navigation, view toggles, theme switching, Mermaid rendering, narrow layout, and print output. If no browser is available, disclose that visual/interactive inspection was not performed rather than claiming it passed.

Keep one HTML file with inline CSS and JavaScript. The exact-pinned Mermaid script is the sole permitted external dependency; remove it when no Mermaid block exists. For offline-only portability, use HTML/CSS or inline SVG instead. Use CSS variables for colors, preserve the template's accessibility behavior, and do not duplicate its JavaScript.

## 8. Deliver

Present the HTML path and summarize:

- what the PR changes;
- the local review recommendation and highest-priority finding count;
- what verification ran and the most important remaining unknown.

Point the user to the dashboard for evidence and details.
