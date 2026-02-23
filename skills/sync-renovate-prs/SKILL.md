---
name: sync-renovate-prs
description: Sync Renovate dependency update PRs from surajssd/aks-rdma-infiniband fork to Azure/aks-rdma-infiniband upstream by cherry-picking commits into a consolidated PR.
disable-model-invocation: true
allowed-tools: Bash, Read
---

# Sync Renovate PRs

Collect commits from open Renovate PRs in `surajssd/aks-rdma-infiniband` and create or update a single consolidated PR in the upstream repo `Azure/aks-rdma-infiniband`, pushing via the current `origin` remote.

## Step 1: Validate prerequisites and derive remotes

Verify required tools are installed:

```bash
command -v gh >/dev/null 2>&1 || { echo "❌ gh CLI is not installed"; exit 1; }
command -v git >/dev/null 2>&1 || { echo "❌ git is not installed"; exit 1; }
command -v jq >/dev/null 2>&1 || { echo "❌ jq is not installed"; exit 1; }
```

Verify the `upstream` remote exists and points to `Azure/aks-rdma-infiniband`:

```bash
git remote get-url upstream | grep -q "Azure/aks-rdma-infiniband"
```

If this fails, report the error: "❌ upstream remote must point to Azure/aks-rdma-infiniband" and stop.

Derive `ORIGIN_OWNER` and `ORIGIN_REPO` from the `origin` remote URL (handles both SSH and HTTPS formats):

```bash
ORIGIN_REPO=$(git remote get-url origin | sed -E 's#.+github\.com[:/]##' | sed 's/\.git$//')
ORIGIN_OWNER=$(echo "$ORIGIN_REPO" | cut -d'/' -f1)
```

If `origin` is not set or cannot be parsed, report the error: "❌ origin remote is not configured or could not be parsed" and stop.

Determine which git remote points to `surajssd/aks-rdma-infiniband` (the Renovate source). Set `RENOVATE_REMOTE` as follows:

1. If `origin` points to `surajssd/aks-rdma-infiniband` → `RENOVATE_REMOTE=origin`
2. Else scan all remotes (`git remote -v`) for one whose URL contains `surajssd/aks-rdma-infiniband` → use that remote name as `RENOVATE_REMOTE`
3. Else add a temporary remote → `git remote add renovate-source https://github.com/surajssd/aks-rdma-infiniband.git` → `RENOVATE_REMOTE=renovate-source`

If a `renovate-source` remote was added, ensure it is removed during cleanup (Step 11).

## Step 2: Fetch latest from remotes

```bash
git fetch origin
git fetch upstream
```

If `$RENOVATE_REMOTE` is not `origin` and not `upstream`, also fetch it:

```bash
git fetch $RENOVATE_REMOTE
```

## Step 3: List open Renovate PRs in fork

```bash
gh pr list --repo surajssd/aks-rdma-infiniband \
  --state open --author "app/renovate" \
  --json number,title,headRefName
```

Store the result. If the list is empty, report "✅ No open Renovate PRs to sync" and stop.

Display the list of found PRs with their numbers and titles.

## Step 4: Create a git worktree

Create a temporary directory and set up a git worktree based on `upstream/main`:

```bash
WORKTREE_DIR=$(mktemp -d)
git worktree add "$WORKTREE_DIR" upstream/main
```

**Important**: Set up cleanup so the worktree is removed when done. At the end of the workflow (success or failure), always run:

```bash
git worktree remove --force "$WORKTREE_DIR" 2>/dev/null || true
```

All subsequent git operations (steps 5-7) must be run inside the worktree directory by passing `-C "$WORKTREE_DIR"` to git or by running `cd "$WORKTREE_DIR"` first.

## Step 5: Create branch `renovate/sync-updates`

Inside the worktree:

```bash
git -C "$WORKTREE_DIR" checkout -B renovate/sync-updates
```

This creates the branch (or resets it if it already exists) based on the current `upstream/main` HEAD.

## Step 6: Cherry-pick each Renovate PR's commits

For each Renovate PR found in Step 3:

1. Fetch the PR's branch from the Renovate source remote:

   ```bash
   git -C "$WORKTREE_DIR" fetch $RENOVATE_REMOTE <headRefName>
   ```

2. Get the list of commits in that branch that are not in `upstream/main`:

   ```bash
   git -C "$WORKTREE_DIR" log --format='%H' $RENOVATE_REMOTE/<headRefName> --not upstream/main
   ```

3. Cherry-pick the commits (in chronological order, i.e., reverse the log output):

   ```bash
   git -C "$WORKTREE_DIR" cherry-pick <commit-sha>
   ```

4. Log progress: `⏳ Cherry-picked PR #<number>: <title>`

If any cherry-pick fails:

- Run `git -C "$WORKTREE_DIR" cherry-pick --abort`
- Report the conflict: "❌ Cherry-pick failed for PR #<number>: <title>. Manual conflict resolution needed."
- Provide the worktree path so the user can resolve manually
- Stop (but still clean up the worktree)

## Step 7: Push branch to fork

Force-push the branch to the fork:

```bash
git -C "$WORKTREE_DIR" push -f origin renovate/sync-updates
```

## Step 8: Check for existing PR in upstream

```bash
gh pr list --repo Azure/aks-rdma-infiniband \
  --state open --head "$ORIGIN_OWNER:renovate/sync-updates" \
  --json number,url
```

## Step 9: Create or update PR

Build the PR body. It should contain:

- A summary line: "Consolidated Renovate dependency updates from the fork."
- A list of commit changes per commit. For each commit, include the original commit message only, don't include the PR number or title from the fork. Format as:

```
- <commit message>
```

The PR title should be: `chore(deps): Renovate dependency updates sync`

**If no existing PR** (Step 8 returned empty):

```bash
gh pr create --repo Azure/aks-rdma-infiniband \
  --base main \
  --head "$ORIGIN_OWNER:renovate/sync-updates" \
  --title "chore(deps): Renovate dependency updates sync" \
  --body "<PR body>"
```

**If existing PR** (Step 8 returned a PR number):

```bash
gh pr edit <number> --repo Azure/aks-rdma-infiniband \
  --body "<PR body>"
```

## Step 10: Report summary

Print a summary including:

- ✅ Number of Renovate PRs synced
- Total number of commits cherry-picked
- PR URL (either newly created or existing)
- List of included PRs (number and title)

## Step 11: Cleanup

Remove the worktree (this should happen automatically via the cleanup set up in Step 4, but ensure it runs):

```bash
git worktree remove --force "$WORKTREE_DIR" 2>/dev/null || true
```

If a `renovate-source` remote was added in Step 1, remove it:

```bash
git remote remove renovate-source 2>/dev/null || true
```
