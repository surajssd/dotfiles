---
name: pr-review-dashboard
description: Use this skill when the user wants to review, understand, or explain a Pull Request that is currently checked out in the local git repository. Triggers include phrases like "review this PR", "explain this PR", "what does this PR do", "review the current branch", or any request to analyze uncommitted-to-main work on a feature branch. Produces an interactive single-file HTML dashboard with architecture diagrams, annotated diffs, and risk assessment. Do NOT use for reviewing arbitrary code snippets pasted in chat, for PRs on remote repositories not checked out locally, or for general code review of files unrelated to a branch diff.
---

# PR Review Dashboard

Act as a Staff-level code reviewer. Help the user understand an unfamiliar Pull Request that is currently checked out. Produce a single-file, interactive HTML dashboard that visually explains the PR, its context, its risks, and your own uncertainty.

**Visualization is the point.** The reason this dashboard exists rather than a markdown summary is that diagrams compress structural information in a way prose cannot. A good dashboard for a non-trivial PR contains *multiple* diagrams — typically 3–6 — each answering a different question (what touches what, what moved, what the new flow looks like, what the schema diff is). One token architecture diagram and a wall of text is the failure mode to avoid. Section 3 below lists the menu of diagram types — scan it for every PR.

---

## Step 1: Determine the default branch

Fetch first so remote refs are current:

```bash
git fetch origin --quiet
```

Try to detect the default branch, most-authoritative source first:

```bash
DEFAULT_BRANCH=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null | sed 's|refs/remotes/origin/||')
[ -z "$DEFAULT_BRANCH" ] && git show-ref --verify --quiet refs/heads/main && DEFAULT_BRANCH=main
[ -z "$DEFAULT_BRANCH" ] && git show-ref --verify --quiet refs/heads/master && DEFAULT_BRANCH=master
echo "DEFAULT_BRANCH=$DEFAULT_BRANCH"
```

If `DEFAULT_BRANCH` is empty, report `❌ Could not determine the default branch (tried origin HEAD, main, master)` and stop.

Prefer `origin/$DEFAULT_BRANCH` as the base for diffs — local branches can be stale.

## Step 2: Sanity-check the working state

A PR review only makes sense from a feature branch — if the user is on the default branch or in detached-HEAD state, there is nothing to diff against.

```bash
CURRENT=$(git rev-parse --abbrev-ref HEAD)
echo "CURRENT=$CURRENT"
```

If `CURRENT` equals `$DEFAULT_BRANCH` or equals `HEAD`, report `❌ You appear to be on $DEFAULT_BRANCH (or detached HEAD), not a feature branch.` to the user and stop. Do not proceed to Step 3.

## Step 3: Gather PR information

Use **three-dot** syntax (`$BASE...HEAD`) for `git diff` — this compares HEAD against the merge base with the default branch, which is what GitHub shows in a PR. Use **two-dot** syntax (`$BASE..HEAD`) for `git log` — this lists commits reachable from HEAD but not the base.

```bash
BASE="origin/$DEFAULT_BRANCH"

# Overview — always run these
git diff $BASE...HEAD --stat
git diff $BASE...HEAD --name-status
git log  $BASE..HEAD  --format="%h %s%n%b"

# Size check before pulling the full diff
git diff $BASE...HEAD --shortstat
```

If no commits exist between `$BASE` and HEAD, report `❌ No commits found between $BASE and HEAD. Make sure you are on a feature branch with commits ahead of $DEFAULT_BRANCH.` and stop.

### Pull author intent if a GitHub PR exists

The diff tells you *what* changed; the PR description tells you *why* the author thought it should change. That gap is exactly what you need to distinguish "what the diff does" from "what the author intended" in Step 4.

Resolving the branch back to its PR is the step most likely to **silently fail**, because `gh pr view` with **no argument** only finds an *open* PR whose head matches the branch's tracked remote. It returns nothing when the PR is already merged or closed, or when the branch was checked out into a worktree (e.g. via the `wtpr` helper) whose upstream points at a different remote than the PR head. Fork PRs — where the head lives under a different owner — are the hardest case: nothing that searches by branch name finds them, so only the explicit breadcrumb in step 1 recovers a fork PR. In each failing case the dashboard wrongly falls back to "no PR yet." Walk the ladder below, **falling through to the next step whenever the current one yields nothing** — where "nothing" means a failed/errored `gh` call, an empty string, an empty array `[]`, or (for step 1) a breadcrumb that fails the sanity-check described after the snippet:

```bash
# `$CURRENT` is the branch name from Step 2. One field set for every lookup, so
# whichever call resolves the PR returns the same author-intent data (body, author,
# labels) plus the headRefOid used to sanity-check the breadcrumb.
FIELDS=url,number,state,title,body,author,labels,additions,deletions,headRefOid
PR_JSON=""

# 1) Explicit breadcrumb: `wtpr` records the PR number in branch config when it
#    creates a worktree from a PR — the only signal that survives merge/close and
#    works for fork PRs, so try it first (subject to the sanity-check below).
PR_NUM=$(git config "branch.$CURRENT.prNumber" 2>/dev/null)
if [ -n "$PR_NUM" ]; then
    PR_JSON=$(gh pr view "$PR_NUM" --json "$FIELDS" 2>/dev/null)
fi

# 2) gh's own branch resolution — works only for an open, same-repo PR.
if [ -z "$PR_JSON" ]; then
    PR_JSON=$(gh pr view --json "$FIELDS" 2>/dev/null)
fi

# 3) Head-ref search across ALL states — catches the merged/closed *same-repo* PRs
#    that no-arg `gh pr view` misses. `--head` matches only same-repo head refs
#    (it does not accept `owner:branch`), so it does NOT surface fork PRs. Returns
#    a JSON array, possibly with several PRs that reused this branch name.
if [ -z "$PR_JSON" ]; then
    PR_JSON=$(gh pr list --head "$CURRENT" --state all --json "$FIELDS" 2>/dev/null)
fi
```

**Sanity-check the breadcrumb before trusting it.** The `prNumber` breadcrumb lives in the repo-wide `.git/config`, keyed by branch name, and is never cleaned up (`git worktree remove` leaves it behind). A reused branch name (`patch-1`, `main`, a recycled fork head) can therefore point at the *wrong* PR. Before accepting step 1's result, confirm the resolved PR belongs to this branch: its `headRefName` should equal `$CURRENT`, and its `headRefOid` should be your `git rev-parse HEAD` **or a descendant of it** — the author may have pushed new commits after `wtpr` fetched the head once, so an exact match is not required and a strict `headRefOid == HEAD` test would wrongly reject an *active* PR. If the resolved PR clearly belongs to different work, discard it and fall through to step 2. (This stays prose, not code: the newer commit often is not in your local object DB, so an offline ancestry check can't always run — judge from the `headRefName`/`headRefOid` the call returns.)

If step 3 returns an array with several PRs that share the branch name, pick the one whose `headRefOid` matches your HEAD (applying the same descendant tolerance); if none match (rare), take the first — `gh` lists most-recent first. Capture the `url` field for the Executive Summary's "Open on GitHub" link, and surface the PR's `state` in the dashboard metadata — reviewing an already-merged PR changes which feedback is still actionable. Only if *every* step above yields nothing (an empty result, or an empty `[]` from step 3 — genuinely no PR for this branch) construct a compare URL from `git remote get-url origin` + `$DEFAULT_BRANCH` + current branch instead, so the dashboard still gives the reader a one-click path back to GitHub.

Treat the body as the author's stated intent — useful context, but not ground truth. The diff is ground truth; the body is a claim about the diff. Note in the dashboard's Assumptions section if the two appear to diverge.

### Decide diff loading strategy by size

- **Small PR** (≤10 files changed AND ≤500 lines changed): load the full diff with `git log $BASE..HEAD -p --reverse`.
- **Medium PR** (≤30 files AND ≤3000 lines): load the full diff but render only the 3–5 most consequential hunks in the dashboard.
- **Large PR** (>30 files OR >3000 lines): **do not** load the full `-p` output. Work from the stat, name-status, and commit messages. Read individual files or hunks on demand with `git show`, `git diff $BASE...HEAD -- <path>`, or by reading the file at HEAD.

## Step 4: Grounding rules (read before writing the dashboard)

- For every behavioral claim, cite the exact file path and line numbers (e.g. `src/foo.go:142–158`).
- Distinguish three things and never blur them:
  (a) what the diff literally does,
  (b) what you infer about author intent,
  (c) what you're guessing because you lack context.
- Never invent function names, types, imports, or call sites. If a symbol is referenced but not visible, say "referenced but not in diff — would need to read X to confirm" — or actually read X.
- Skip pure formatting, import reordering, and lockfile churn unless they signal something real (new dependency, breaking version bump).
- Focus on what a reviewer would actually push back on, not what's merely noteworthy.

## Step 5: Build the dashboard

**Start from the bundled template, not from scratch.** Copy `scripts/dashboard_template.html` (sibling to this SKILL.md) to a fresh file via `cp "$(dirname THIS_SKILL)/scripts/dashboard_template.html" "$(mktemp --suffix=.html)"`, then fill in the `<!-- FILL: ... -->` blocks. The template already includes:

- Light/dark theme toggle with `localStorage` persistence
- Tab system with lazy mermaid rendering (no more blank-tab-on-first-paint bug)
- Theme re-init on toggle (mermaid diagrams swap palettes correctly)
- Before/After/Diff toggle pattern (`.view-toggle` + `.view-panel`)
- Box-and-arrow primitives for HTML/CSS diagrams (`.diagram`, `.node`, `.arrow`, etc.)
- Diff hunk styling, severity-coded risk table, glossary layout
- Mermaid loaded lazily — delete the `<script src=...mermaid...>` line if you don't use any mermaid diagrams

Do not rewrite the boilerplate. If the template is missing a primitive you need, add it once to the template's `<style>` block rather than inlining it for one run — the template should accumulate improvements.

The tabs in the template already match the section order below. Fill them in that order, since each section assumes the reader has read the previous one:

1. **Executive Summary** — TL;DR of what the PR achieves, why it matters, and the single biggest thing a reviewer should scrutinize. Three to six sentences, no fluff. Include a small metadata row near the title with **a clickable link to the PR on GitHub** (use the `url` resolved by the lookup ladder in Step 3 — explicit `prNumber` breadcrumb, then no-arg `gh pr view`, then `gh pr list --head <branch> --state all`; if every lookup truly returns nothing, fall back to the branch compare URL, e.g. `https://github.com/<owner>/<repo>/compare/<base>...<head>`). When the PR resolved to a merged or closed state, say so in this row — it tells the reviewer whether their feedback is still actionable. The reviewer almost always wants to jump back to the PR to leave a comment; not providing that link forces them to context-switch and hunt for it.

2. **Glossary / Concept Primer** — Define the key modules, types, acronyms, and domain terms appearing in this diff, written for someone seeing this codebase for the first time. Without shared vocabulary the rest of the dashboard is opaque, so put this near the top.

3. **Architecture & Visualizations (the centerpiece)** — This is the highest-leverage tab for a reviewer trying to load an unfamiliar PR into their head, and one box-and-arrow diagram is rarely enough. Treat this tab as a *gallery* of diagrams, not a single map. A good rule of thumb: a substantive PR deserves **3–6 distinct visualizations**, each answering a different question. Lean toward *more* diagrams of *narrower* scope rather than one giant diagram that tries to show everything.

   **Default to HTML/CSS, not mermaid.** The bundled `scripts/dashboard_template.html` provides box-and-arrow primitives (`.diagram`, `.diagram-row`, `.node`, `.node.new`, `.arrow`, `.arrow.dashed`, `.view-toggle`, `.view-panel`) that render module maps, data-flow diagrams, migration flowcharts, before/after toggles, and comparison tables as plain `<div>`s. Reach for these first. HTML/CSS box diagrams have three big wins over mermaid:
   - No parser to anger — no syntax-error bombs.
   - You control layout pixel-by-pixel, so labels never collide and edges never cross weirdly.
   - They look like the rest of the page and respect the theme automatically.

   Reserve **mermaid for the three diagram types where its auto-layout genuinely pays for itself**: `sequenceDiagram` (time-ordered interactions), `stateDiagram-v2` (lifecycle / phase machines), and `classDiagram` (type hierarchies). Hand-coding those in HTML is painful and ugly; mermaid's layout is worth the parser fragility. For everything else — module maps, data flow, migration flowcharts, before/after comparisons — use the HTML/CSS primitives.

   **Pick diagrams based on what the PR actually is.** Don't render every type below for every PR — pick the ones that earn their place. But do scan this list deliberately for each PR and ask "would this diagram help a reviewer here?":

   - **Module / dependency map** *(HTML/CSS)* — flowchart showing which packages/files/components touch which, with new or modified nodes visually distinguished (dashed border, accent color). The "what touches what" view.
   - **Before / After / Diff toggle** *(HTML/CSS)* — for refactors, API changes, or schema migrations, render the same map in three states behind a toggle. The template's `.view-toggle` + `.view-panel` does this with no JS to write.
   - **Sequence diagram** *(mermaid)* — for changes to a request flow, reconcile loop, RPC chain, or any time-ordered interaction. `sequenceDiagram`.
   - **State machine / phase diagram** *(mermaid)* — when the PR changes a state machine, lifecycle, or status enum. `stateDiagram-v2`.
   - **Data-flow / pipeline diagram** *(HTML/CSS)* — for ETL, event handling, scrape pipelines, or any "data goes in here, ends up there" change.
   - **Schema diff side-by-side** *(HTML `<pre>` in a `.row`)* — for CRD / proto / SQL / JSON-schema / type changes. Old on the left, new on the right, changed lines wrapped in `<span class="l-add">` / `<span class="l-del">`.
   - **Migration / startup flowchart** *(HTML/CSS)* — for one-time data migrations, backfills, or controller-startup migration steps. Decision tree with retry/error branches; reviewers always want to know "what happens when this fails midway."
   - **Capabilities / behavior comparison table** *(HTML table)* — 4–8 dimensions (API surface, signature, caching, error behavior, legacy handling, etc.) in two columns labelled Before and After. Often the single most useful artifact for a reviewer.
   - **Risk / change-surface heatmap** *(HTML table)* — for large PRs, a grid of files vs. risk-axes colored by severity, so the reviewer can target their attention.
   - **Commit-shape breakdown** *(HTML table or simple bar)* — when the branch has many commits, group by conventional-commit prefix (`feat`, `fix`, `refactor`, etc.) so the reviewer can tell at a glance whether this is a feature PR with stray refactors or a refactor PR with a feature bolted on. Use `git log --format=%s $BASE..HEAD` and group.
   - **Call hierarchy / class diagram** *(mermaid)* — when the PR introduces or restructures types/interfaces. `classDiagram`.

   **Interactive controls earn their keep.** A toggle that switches a single diagram between Before / After / Diff is worth more than rendering three separate diagrams stacked vertically — it forces the reader's eye to land on the same node and notice what changed. The template's `.view-toggle` pattern handles this.

   **One-liner per diagram.** Every diagram needs a sentence above it (use `<p class="caption">`) explaining what question it answers (e.g. "*This shows the request path through the reconciler; the dashed boxes are new in this PR.*"). A diagram without a framing question is decoration.

   **If the PR is genuinely small** (single function, small bugfix) — one clear diagram, or none, is correct. Say so explicitly rather than padding with low-value visualizations.

   **Mermaid syntax safety** *(only relevant if you actually use mermaid)* — Mermaid 10's parser is fragile; a single bad token replaces the whole diagram with a cartoon bomb. The bundled template's CSS and JS already handle the rendering-on-hidden-tab and theme-swap races, but you still have to write valid mermaid source:
   - Wrap node labels in double quotes when they contain anything beyond `[A-Za-z0-9_ ]` — colons, slashes, parentheses, dots, `<br/>`, `&`, quotes.
   - For edge labels with punctuation, prefer `A -- "label with : or /" --> B` over `A -->|label| B`. Pipe labels starting with `:` or containing `/` are a known failure mode.
   - Keep node IDs to `[A-Za-z0-9_]` only.
   - In `classDef`, no space after the colon in style values: `stroke-dasharray:5 5`, not `stroke-dasharray: 5 5`.
   - Mentally re-parse each diagram before emitting it. A broken mermaid block is a worse signal than no diagram.

4. **Annotated Diff Viewer** — Render hunks styled like a real diff (red for removed, green for added, neutral for context). Each hunk gets an inline annotation or hover-tooltip explaining *why* the change was made in plain English — not just restating what the code does. How many hunks:
   - **Small PRs**: render all hunks.
   - **Medium/Large PRs**: render the 3–5 most consequential hunks, and include an explicit note listing what was omitted (file paths + one-line summary each) so the reader knows the scope of the sample.

5. **Risk Assessment** — Consider at least the axes below. Add others if the PR's domain calls for them (e.g. migration safety, i18n, accessibility, data privacy, supply-chain risk for dependency bumps). Mark any axis you considered but found not relevant as "Not applicable" rather than silently dropping it — the reader should be able to tell the difference between "checked and clean" and "didn't think about it."
   - Concurrency / race conditions
   - Error handling and failure modes
   - API contract / backward compatibility
   - Performance on hot paths
   - Security (input handling, auth, secrets, injection surfaces)
   - Observability (logging, metrics, tracing gaps)
   - Test coverage of the new behavior

   Render findings in a table, color-coded by severity:
   - Red — High/Critical, a reviewer would block on this
   - Yellow — Medium, worth raising
   - Blue — Info, noteworthy but not blocking

6. **Assumptions & Unknowns** — What you assumed, what you couldn't verify, and what you'd need from the user or the PR author to be more confident. All uncertainty goes here — do not smuggle it into other sections, because hedging mixed into the Risk or Diff sections makes it impossible for the reader to tell confident findings from speculative ones.

## Step 6: Technical constraints for the HTML

These are mostly already satisfied by the bundled template — listed here so you know what you're allowed to change and what you shouldn't.

- ONE valid HTML file. All HTML, CSS, and vanilla JavaScript inline. (Template enforces this.)
- Theme switching, tab switching, mermaid lazy-render, and the view-toggle pattern are all in the template's `<script>` block — do not duplicate or rewrite them.
- All colors come from CSS custom properties (`--bg`, `--fg`, `--panel`, `--accent`, etc.) defined under `:root, [data-theme="dark"]` and `[data-theme="light"]`. If you need a new color, add it as a variable in both blocks — don't hardcode hex values inline.
- Mermaid.js is loaded from a pinned CDN URL in the template. If you don't use any mermaid diagrams, delete the `<script src="https://cdn.jsdelivr.net/npm/mermaid@10/...">` line so the file is fully self-contained.
- No external images, no canvas, no frameworks, no other CDN dependencies.
- The output has to survive being emailed, dropped into a Slack thread, or opened weeks later on a different machine.

## Step 7: Deliver

Write the HTML file to disk and present it. Then in chat, give a short three-line summary:

- One sentence: what the PR does.
- One sentence: the most important risk you found.
- One sentence: your top open question.

Point the user to the dashboard for the rest.
