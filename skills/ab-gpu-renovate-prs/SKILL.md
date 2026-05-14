---
name: ab-gpu-renovate-prs
description: Check open Renovate bot PRs on Azure/AgentBaker for GPU component updates (nvidia-device-plugin, datacenter-gpu-manager, dcgm-exporter, nvidia-dcgm, aks-gpu-cuda). Shows pipeline status and whether review is needed.
allowed-tools: Bash
disable-model-invocation: true
---

# GPU Renovate PRs

Check open Renovate bot PRs in `Azure/AgentBaker` that update GPU-related components and display a summary table with pipeline status and review status.

## Step 1: Configuration

Define the configurable component list, repo, and check name:

```bash
REPO="Azure/AgentBaker"
COMPONENTS=("nvidia-device-plugin" "datacenter-gpu-manager*" "nvidia-dcgm" "dcgm-exporter" "aks-gpu-cuda")
CHECK_NAME="version-consistency"
```

## Step 2: Get authenticated user

```bash
MY_LOGIN=$(gh api user --jq '.login')
```

If this fails, report "❌ Could not determine authenticated GitHub user. Is `gh` configured?" and stop.

## Step 3: List open Renovate PRs

```bash
gh pr list --repo "$REPO" --author "app/renovate" --state open \
  --json number,title,url,reviewRequests,reviews
```

Store the JSON result. If the command fails, report "❌ Failed to list PRs from $REPO" and stop.

## Step 4: Filter for GPU components

For each PR in the list, check if the title matches any of the `COMPONENTS` entries (case-insensitive). A PR matches if its title contains the component name as a substring. If a component entry ends with a `*`, treat it as a prefix match (i.e., the PR title matches if it contains a word that starts with the prefix before the `*`).

If no PRs match, report "✅ No open GPU component PRs found" and stop.

## Step 5: Get check status for each matching PR

For each matching PR, run:

```bash
gh pr checks <NUMBER> --repo "$REPO" --json name,state,workflow
```

Look for a check where the `name` field equals `CHECK_NAME` (`version-consistency`). Map to a status string:

- `✅ pass` if state is `SUCCESS`
- `❌ fail` if state is `FAILURE`
- `⏳ pending` if state is `PENDING`
- `➖ not run` if the check does not exist in the output

## Step 6: Determine review status for each matching PR

Using the `reviewRequests` and `reviews` fields from the PR data in Step 3, determine the review status for each PR:

1. If `MY_LOGIN` appears in `reviewRequests` (check the `login` field of each entry) → `🔴 Review requested`
2. If `MY_LOGIN` does not appear in any entry in `reviews` (check the `author.login` field) → `🟡 Not reviewed`
3. Otherwise find the latest review by `MY_LOGIN` and use its `state`:
   - `APPROVED` → `🟢 Approved`
   - `CHANGES_REQUESTED` → `🔴 Changes requested`
   - `COMMENTED` → `🟡 Commented`

## Step 7: Output table

Render a markdown table with the results. Each PR number should be a clickable link to the PR URL.

```
| PR | Title | version-consistency | Review Status |
|----|-------|---------------------|---------------|
| [#8129](https://github.com/Azure/AgentBaker/pull/8129) | update nvidia-device-plugin (minor) | ➖ not run | 🟢 Approved |
| [#8143](https://github.com/Azure/AgentBaker/pull/8143) | update dependency dcgm-exporter | ✅ pass | 🟡 Not reviewed |
```
