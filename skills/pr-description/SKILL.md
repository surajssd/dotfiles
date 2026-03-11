---
name: pr-description
description: Generate a PR description by summarizing commits and diffs on the current branch. Use when user asks to generate, write, create, draft, or summarize a PR description, or asks "what changed in this PR" or "describe these changes".
allowed-tools: Bash, Read, Glob, Grep
---

# Generate PR Description

Analyze the current branch's commits and diffs against the default branch, optionally use a PR template, and output a well-formatted PR description.

## Step 1: Determine the default branch

Try to find the default branch name:

```bash
git rev-parse --verify main >/dev/null 2>&1
```

If `main` exists, set `DEFAULT_BRANCH=main`.

Otherwise, try `master`:

```bash
git rev-parse --verify master >/dev/null 2>&1
```

If `master` exists, set `DEFAULT_BRANCH=master`.

If neither exists, try to detect via the remote HEAD:

```bash
git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|refs/remotes/origin/||'
```

If this returns a branch name, use that as `DEFAULT_BRANCH`.

If no default branch can be determined, report "❌ Could not determine the default branch (tried main, master, and origin HEAD)" and stop.

## Step 2: Gather commits and diffs

Run the following commands to collect change information:

1. Full commit log with diffs (for deep analysis):

   ```bash
   git log $DEFAULT_BRANCH..HEAD -p --reverse
   ```

2. File change summary (for the overview):

   ```bash
   git diff $DEFAULT_BRANCH..HEAD --stat
   ```

3. One-line commit list (for reference):

   ```bash
   git log $DEFAULT_BRANCH..HEAD --format="%h %s"
   ```

If no commits are found between `$DEFAULT_BRANCH` and HEAD (i.e., the output is empty), report "❌ No commits found between $DEFAULT_BRANCH and HEAD. Make sure you are on a feature branch with commits ahead of $DEFAULT_BRANCH." and stop.

## Step 3: Search for a PR template

Check for a PR template in these locations, in priority order, using the Glob tool:

1. `.github/PULL_REQUEST_TEMPLATE.md`
2. `.github/pull_request_template.md`
3. `PULL_REQUEST_TEMPLATE.md` (repo root)
4. `pull_request_template.md` (repo root)
5. `docs/PULL_REQUEST_TEMPLATE.md`
6. `docs/pull_request_template.md`
7. `.github/PULL_REQUEST_TEMPLATE/` directory (if it exists, list all `.md` files inside and use the first one)

Use glob patterns like `**/{PULL_REQUEST_TEMPLATE,pull_request_template}.md` to search efficiently.

If a template is found, read its contents with the `Read` tool. If multiple templates are found (e.g., in `.github/PULL_REQUEST_TEMPLATE/`), use the first one and note which template was used.

If no template is found, that is fine — proceed with the default format in Step 4.

## Step 4: Compose the PR description

Analyze all commits and diffs gathered in Step 2. The description should cover:

- **WHAT** changed — a clear summary of the modifications
- **WHY** — the motivation or purpose behind the changes (inferred from commit messages and code context)
- **HOW** — for non-trivial changes, briefly explain the approach or implementation strategy
- **Impact/Scope** — which areas of the codebase are affected

**If a PR template was found in Step 3:**

- Fill in each section of the template based on the commit and diff analysis
- For checkbox items (e.g., `- [ ] Tests added`), check them (`- [x]`) ONLY if the diffs clearly confirm the criteria is met
- Do NOT remove or skip any sections from the template — fill them all in, even if a section is "N/A"
- Preserve the template's formatting and structure exactly

**If no PR template was found:**

Use this default format:

```markdown
## Summary

<1-3 sentence high-level summary of the changes>

## Changes

<Group changes logically by theme or component, not per-commit. Use bullet points.>

## Test Plan

<Describe how the changes can be tested, or note if tests were added/modified>
```

**Important:** Group changes logically by theme or component, NOT one bullet per commit. Combine related commits into coherent change descriptions.

## Step 5: Output the result

**Default behavior:** Output the PR description as formatted markdown directly to the user. Do NOT post it to GitHub unless explicitly asked.

**If the user explicitly asks to update/post the PR** (e.g., the user said "update the PR", "post it", "apply it to the PR", or invoked with arguments like "update"):

1. Check for an open PR on the current branch:

   ```bash
   gh pr view --json number,url 2>/dev/null
   ```

2. If no open PR exists, report "❌ No open PR found for the current branch. Push your branch and create a PR first, or use `gh pr create`." and stop.

3. If an open PR exists, update it:

   ```bash
   gh pr edit --body "<generated description>"
   ```

4. Report "✅ PR description updated: <PR URL>" on success.
