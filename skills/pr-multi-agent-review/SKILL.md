---
name: pr-multi-agent-review
description: Orchestrate a panel of local AI coding CLIs (claude, codex, gemini, opencode, copilot, agency copilot) to independently review the Pull Request currently checked out, then collate their findings into one consensus-first markdown report plus a visual HTML dashboard. Use this whenever the user wants a "multi-agent", "panel", "multi-model", or "second opinion" review of the current branch/PR, asks to "review this PR with several agents / models", wants reviews "collated" or "cross-checked" across tools, or wants the review to also cover tests, a manual testing plan, documentation updates, and unresolved GitHub review threads. Nothing is posted to GitHub — the output is local files. Do NOT use for reviewing a pasted snippet, a remote PR that is not checked out, or when the user wants a single reviewer (use pr-review-dashboard for a solo visual review).
allowed-tools: Bash, Read, Write, Skill
---

# PR Multi-Agent Review

Run several AI coding CLIs as an independent review panel over the **currently checked-out PR**, then synthesize their reviews into one report. The value over a single reviewer is *triangulation*: a finding three tools independently flag is almost certainly real; a finding only one raises is either a sharp catch or a false positive — and the report should make that distinction visible so the user knows where to spend attention.

Nothing is posted to GitHub. Every artifact is a local file in a temp directory.

**Requirements.** `git` and at least one reviewer CLI are mandatory. `gh` + `jq` are needed for PR description and unresolved-thread context — without either, the skill degrades gracefully (diff + commits only) and says so. A `timeout`/`gtimeout` binary is used if present; otherwise a built-in bash watchdog enforces the per-reviewer timeout. The scripts avoid GNU-only flags (`readlink -f`, `mktemp --suffix`) so they run on stock macOS as well as Linux.

**Trust boundary (read this).** The panel feeds *untrusted* PR content — body, commit messages, third-party review comments, the diff — into reviewer CLIs, and three of them (`copilot`, `opencode`, `agency`) run with `--allow-all-tools` and no hard read-only sandbox. A malicious PR could attempt prompt-injection against those tools. Run this skill on **PRs you trust**, or isolate the no-sandbox tools (throwaway worktree, network off) when reviewing fork/contributor branches. Step 5 covers the specifics.

**The orchestrator (you) never reviews the code yourself.** Your job is to gather context, dispatch the panel, and collate. If you inject your own opinions as if they were a reviewer's, you destroy the signal of how many *independent* tools agreed. You may reconcile and judge their findings during collation, but the findings must originate from the panel.

---

## Workflow at a glance

1. **Preflight** — confirm a feature branch is checked out; find the base branch.
2. **Gather context** — diff, commits, PR description, changed files, unresolved GitHub threads → write to a temp context dir.
3. **Build the dashboard** — invoke the `pr-review-dashboard` skill for the visual artifact.
4. **Pick the panel** — detect which reviewer CLIs are installed; apply any model overrides the user asked for.
5. **Dispatch** — run every reviewer in parallel, headless and read-only, each writing its own review file.
6. **Collate** — synthesize all reviews (consensus-first) into `FINAL-REPORT.md`.
7. **Deliver** — print the paths and a three-line summary.

Use a TodoList to track these — step 5 fans out into one task per reviewer and it's easy to lose one.

---

## Step 1: Preflight

The whole skill is meaningless off a feature branch, so fail fast. Run the bundled helper, which detects the default branch and verifies you're not on it or in detached HEAD.

Resolve the skill directory portably (BSD `readlink` has no `-f`), then **source** the preflight output rather than `eval`-ing it — the helper emits `printf %q`-quoted assignments, so a branch name containing shell metacharacters (`'`, `;`, `$(...)` — all legal in git refs) can't execute code in your shell:

```bash
SKILL_LINK=~/.claude/skills/pr-multi-agent-review/SKILL.md
SKILL_TARGET="$(readlink "$SKILL_LINK" 2>/dev/null || echo "$SKILL_LINK")"
SKILL_DIR="$(cd "$(dirname "$SKILL_TARGET")" && pwd -P)"

PREFLIGHT="$(mktemp)"
if "$SKILL_DIR/scripts/gather_pr_context.sh" --preflight-only > "$PREFLIGHT"; then
  source "$PREFLIGHT"   # sets DEFAULT_BRANCH, CURRENT_BRANCH, BASE (safely)
  rm -f "$PREFLIGHT"
else
  rm -f "$PREFLIGHT"     # helper already printed why; stop here
fi
```

If the helper exits non-zero it has already printed the reason (on `$DEFAULT_BRANCH`, detached HEAD, no commits ahead of base). Relay that to the user and stop — do not try to review nothing.

## Step 2: Gather PR context

Create a temp workspace and populate it. One helper does all of it so every reviewer reads identical inputs. `PR_REVIEW_SKIP_FETCH=1` reuses the refs the preflight already fetched, avoiding a redundant second `git fetch`:

```bash
WORKDIR="$(mktemp -d)/pr-review" && mkdir -p "$WORKDIR"
PR_REVIEW_SKIP_FETCH=1 "$SKILL_DIR/scripts/gather_pr_context.sh" "$WORKDIR"
ls -1 "$WORKDIR/context"
```

After it runs, `$WORKDIR/context/` contains:

- `diff.patch` — the full `git diff $BASE...HEAD` (three-dot: what GitHub shows)
- `commits.txt` — `git log $BASE..HEAD` with bodies
- `changed-files.txt` — name-status list
- `pr-description.md` — title + body + labels from `gh pr view`, or a note that no PR exists yet
- `unresolved-threads.md` — every **unresolved** review thread (human reviewers *and* the GitHub Copilot bot), with file/line, author, and the comment body
- `meta.env` — `OWNER`, `REPO`, `PR_NUMBER`, `BASE`, `CURRENT_BRANCH`, `PR_URL` for your own use

The PR description is author intent, not ground truth — the diff is ground truth. The unresolved threads matter because a good panel review should tell the user whether issues *already raised on GitHub* are addressed by the latest code, not re-litigate them blind. Both get fed to every reviewer and folded into the final report.

If `gh` is missing or the branch has no PR, the helper still produces the diff/commit/file context and writes a clear "no GitHub PR found" note into the description and threads files — the review proceeds on the diff alone.

## Step 3: Build the visual dashboard

Invoke the `pr-review-dashboard` skill to produce the interactive HTML. It understands the same checked-out PR and gives the user a visual way to load the change into their head while the panel runs:

```
Use the Skill tool: pr-review-dashboard
```

When it finishes, note the HTML path it produced — you'll link it from the final report and mention it in delivery. Kick this off (or let it run) alongside step 5 if you can; it's independent of the panel.

## Step 4: Pick the review panel

Only run tools that are actually installed — invoking a missing binary wastes a turn and clutters the report. Detect what's available:

```bash
"$SKILL_DIR/scripts/detect_reviewers.sh"
```

It prints one line per candidate: `available <label> <tool>` or `missing <tool>`. The candidate roster is `claude`, `codex`, `gemini`, `opencode`, `copilot`, and `agency` (agency copilot). Build your panel from the `available` lines and tell the user which tools were skipped and why ("`gemini` not in PATH — skipping").

### Models: pre-selected by default

By default, pass **no model flag** — each tool uses whatever model it's configured with. This respects the user's existing setup and is what they want unless they say otherwise.

Override only when the user explicitly names models. The interesting case is running **the same tool more than once with different models** — e.g. "run agency copilot with a Claude model and again with a GPT model." Model-capable tools accept `--model`/`-m`; the panel just gets two entries for that tool, each with a distinct label so their reviews don't collide:

| User asks for | Panel entries (label \| tool \| model) |
|---|---|
| default (no models named) | `claude\|claude\|`, `codex\|codex\|`, … one per available tool |
| "agency copilot with sonnet and with gpt-5" | `agency-sonnet\|agency\|claude-sonnet-4.5`, `agency-gpt5\|agency\|gpt-5` |
| "codex on gpt-5-codex" | `codex\|codex\|gpt-5-codex` |

The label is yours to choose — make it readable and unique (it becomes the review filename and the section header). See `references/reviewer-cli-matrix.md` for exactly how each tool is invoked and which support `--model`.

## Step 5: Dispatch the panel

Every reviewer gets the **same** instructions + context so differences in output reflect the models, not the prompt. The wrinkle is *diff delivery*: it's what makes large PRs work, and it differs by tool, so `run_reviewer.sh` owns it. You build a **diff-less** prompt with `build_prompt.sh --no-diff` and hand the diff to `run_reviewer.sh` separately:

```bash
"$SKILL_DIR/scripts/build_prompt.sh" --no-diff \
  "$WORKDIR/context" "$SKILL_DIR/references/review-prompt.md" \
  > "$WORKDIR/review-prompt.nodiff.txt"
```

Why embed the context in the prompt rather than point reviewers at the files? The panel CLIs disagree sharply on filesystem sandboxing — `opencode` hard-rejects any path outside its working directory (so it would read *nothing* from a `/tmp` context dir), others need per-tool `--add-dir` flags. Embedding sidesteps all of it: every reviewer gets byte-identical inputs with zero file-permission friction, and can still read the repo it's running in for surrounding context.

`run_reviewer.sh` takes the diff via `--diff-file` so it can adapt prompt *delivery* per tool — that adaptation is the whole reason large PRs don't break:

- **stdin-capable tools** (`claude`, `codex`, `gemini`) get the diff-less prompt **plus the diff** concatenated onto **stdin**, which has no size limit.
- **argv-only tools** (`opencode`, `copilot`, `agency`) get the prompt as one argv string, which is bounded (Linux caps a single argument at 128 KiB). If instructions+context+diff would exceed a safe cap, the script **omits the embedded diff and instructs the agent to run `git diff "$BASE"...HEAD` itself** — it's running in the repo, so it can. No reviewer ever dies with `E2BIG`/"Argument list too long".

Read `references/review-prompt.md` yourself once so you know what you're asking the panel to produce — it directs each reviewer to cover correctness, security, performance, error handling, concurrency, API/compat, **test quality**, **human + agentic documentation**, to end with a **manual testing plan**, all cited to `file:line`, and to wrap the whole review between `===PR-REVIEW-BEGIN===`/`===PR-REVIEW-END===` sentinels.

Then launch **one background task per panel entry** — they're independent and slow (each is a full agent run), so parallelism is the difference between one minute and ten. For each entry run:

```bash
"$SKILL_DIR/scripts/run_reviewer.sh" \
  --label "<label>" --tool "<tool>" --model "<model-or-empty>" \
  --prompt-file "$WORKDIR/review-prompt.nodiff.txt" \
  --diff-file "$WORKDIR/context/diff.patch" --base "$BASE" \
  --output-file "$WORKDIR/reviews/<label>.md" --timeout 900
```

Launch each as a **background Bash task** (`run_in_background: true`) so they run concurrently and you get notified as each finishes. `run_reviewer.sh` owns every per-tool quirk — headless flag, read-only mode, model flag, stdin-vs-argv delivery, the timeout (with a pure-bash watchdog fallback when neither `timeout` nor `gtimeout` is installed), capturing stdout/stderr, and **extracting the review from between the sentinels** so the TUI progress chatter that `copilot`/`agency` print to stdout doesn't pollute the review. It writes the cleaned review to `<label>.md`, the unfiltered capture to `<label>.md.raw`, and a one-line status to `<label>.md.status`.

### Read-only and the untrusted-input boundary

Read-only is enforced where the tool supports it (codex `--sandbox read-only`, gemini `--approval-mode plan`, claude `--permission-mode plan`). **`copilot`, `opencode`, and `agency` have no hard read-only switch and run with `--allow-all-tools`** — only the prompt asks them not to write. That is a real exposure: the prompt embeds *untrusted* PR content (body, commit messages, third-party review-thread comments, the diff), and a crafted "ignore previous instructions…" payload could drive a fully tool-enabled agent. The post-run `git status` check catches tracked-file writes only — not reads, network calls, or untracked files.

Mitigate, don't pretend it's airtight:

- **Only run this skill on PRs you trust** unless you've isolated the no-sandbox tools. Tell the user this plainly when the PR is from a fork or an unknown contributor.
- For untrusted branches, prefer reviewing inside a throwaway git worktree and/or with networking disabled for the `copilot`/`opencode`/`agency` runs.
- Before dispatching, capture `git status --porcelain` as a baseline; after the panel finishes, diff it again. If a reviewer wrote a stray file, note it; don't blindly revert (you might clobber the user's uncommitted work).

As each task completes, glance at its `.status` and the head of its review file. `ok` = a clean sentinel-wrapped review. `ok-empty` = the tool ran but emitted no sentinels (it likely refused or rambled); the salvaged/raw output is kept so you can judge it. `errored` = it crashed, timed out (a stub `.md` is still written so the member stays visible to collation), or auth failed. A missing reviewer is information, not something to hide or silently retry more than once.

## Step 6: Collate into the final report

Once every task has finished (or errored), read all of `$WORKDIR/reviews/*.md`, plus `unresolved-threads.md` and `pr-description.md`. Write `$WORKDIR/FINAL-REPORT.md` using the structure below.

The organizing principle is **consensus first**: lead with what multiple reviewers independently agree on, because agreement across independent models is the strongest signal in the whole exercise. Preserve disagreement honestly — if two tools call something a bug and one calls it correct, the user needs to see the conflict, not an averaged-away mush. Keep each reviewer's raw output linked in an appendix so nothing is lost.

```markdown
# Multi-Agent PR Review: <PR title or branch>

<one-paragraph orientation: what the PR does, how big, how many reviewers ran, the single most important takeaway>

**PR:** <PR_URL or "no GitHub PR — local branch <CURRENT_BRANCH>">
**Base:** <BASE>  ·  **Panel:** <label list>  ·  **Skipped:** <tools not in PATH>
**Dashboard:** <path to the pr-review-dashboard HTML>

## Consensus findings
<!-- Raised by 2+ reviewers. Highest confidence. Each: severity, file:line, the issue,
     and which panel members flagged it. Order by severity then by agreement count. -->
| Severity | Finding | Location | Flagged by |
|---|---|---|---|

## Single-reviewer findings worth surfacing
<!-- Raised by exactly one reviewer but credible. These are the sharp catches OR the false
     positives — label your read of which, and say why. -->

## Disagreements
<!-- Where reviewers contradict each other. State both positions and your reconciliation,
     or "needs author judgment" if you genuinely can't adjudicate from the diff. -->

## Testing assessment
<!-- Synthesized across the panel: does the PR test the new behavior? What's untested?
     Are the tests meaningful or box-ticking? -->

## Documentation assessment
<!-- Two distinct questions, answered separately:
     - Human docs: README / docs/ / user-facing .md updated where the change warrants it?
     - Agentic docs: CLAUDE.md / AGENTS.md / .github/copilot-instructions.md / GEMINI.md /
       .cursor rules updated if the change affects how agents should work in this repo?
     "Not needed" is a valid answer — say so explicitly rather than omitting. -->

## Manual testing plan
<!-- Concrete, ordered steps a human can run to validate this PR by hand: setup, the actions
     to take, and the expected result for each. Deduped and merged from the panel's plans. -->

## Status of unresolved GitHub threads
<!-- For each unresolved thread from context: does the latest diff appear to address it?
     (addressed / partially / not addressed / can't tell). This is why we fetched them. -->

## Prioritized action list
<!-- The "if you do nothing else, do these" list, severity-ordered. -->

## Appendix: raw reviews
<!-- One link per panel member to its file under reviews/, plus any that errored. -->
```

Severity vocabulary, used consistently: **Critical** (blocks merge / data loss / security hole) · **High** (a reviewer would block on it) · **Medium** (should fix, not blocking) · **Low** (minor) · **Info** (noteworthy, no action). When reviewers use different scales, normalize to this one so the table is comparable.

## Step 7: Deliver

Print the three artifact paths plainly so the user can open them:

- `FINAL-REPORT.md` — the collated review
- the dashboard HTML — visual understanding
- `reviews/` — each panel member's raw output

Then give a three-line spoken summary: one line on what the PR does, one on the most important consensus finding, one on the biggest open question or disagreement. Point the user to the report for the rest. Do **not** post anything to GitHub — this skill only ever reads the PR and writes local files.

---

## Reference files

- `references/review-prompt.md` — the shared prompt template handed to every reviewer. Read it before dispatching so you know what the panel was asked to do.
- `references/reviewer-cli-matrix.md` — exact headless/read-only invocation and model-flag support for each CLI. Consult when a reviewer behaves oddly or when adding a new tool.
