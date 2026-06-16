---
name: plan-multi-agent-review
description: Orchestrate a panel of local AI coding CLIs (claude, codex, gemini, opencode, copilot, agency copilot) to independently review a PLAN FILE against the code repository it targets, then collate their findings into one consensus-first markdown report. Use this whenever the user wants a "multi-agent", "panel", "multi-model", or "second opinion" review of a plan / plan file / design doc / implementation plan, asks to "review this plan with several agents / models", or wants plan reviews "collated" or "cross-checked" across tools. The two inputs are a plan file (explicit path, required) and the repo; the sole output is a review of the plan. Nothing is posted anywhere — output is local files in a temp dir. Do NOT use to review a Pull Request or a git diff (use pr-multi-agent-review), a pasted snippet, or when the user wants a single reviewer.
allowed-tools: Bash, Read, Write
---

# Plan Multi-Agent Review

Run several AI coding CLIs as an independent review panel over a **plan file** — judged against the
**code repository it targets** — then synthesize their reviews into one report. The value over a
single reviewer is *triangulation*: a flaw three tools independently flag is almost certainly real;
a concern only one raises is either a sharp catch or a false positive — and the report should make
that distinction visible so the user knows where to spend attention.

The two inputs are **a plan file** (an explicit path — required) and **the repo** the plan is
about. The **sole artifact produced is a review of the plan.** Nothing is posted anywhere; every
artifact is a local file in a temp directory.

**Requirements.** `git` and at least one reviewer CLI are mandatory. A `timeout`/`gtimeout` binary
is used if present; otherwise a built-in bash watchdog enforces the per-reviewer timeout. The
scripts avoid GNU-only flags (`readlink -f`, `mktemp --suffix`) so they run on stock macOS as well
as Linux.

**Trust boundary (read this).** Two distinct exposures, because the panel feeds the *plan* and
repo files into the reviewer CLIs and lets them explore a live working tree:

- **Prompt-injection → tool execution.** Three tools (`copilot`, `opencode`, `agency`) run with
  `--allow-all-tools` and no hard read-only sandbox. Because this skill points every reviewer at a
  **live repo as its working directory** so it can verify the plan, a malicious file in that repo
  (or a crafted instruction inside the plan) could attempt to drive those tools. Run this on repos
  and plans you trust, or isolate those tools (throwaway worktree, network off).
- **Data egress.** Every reviewer streams the plan and the repo content it reads to its model
  provider (Anthropic, OpenAI, GitHub, Google, …). This skill embeds the plan-referenced files
  **and** invites live exploration, so **more repo content can leave the machine than a single diff
  would** — don't review a plan whose repo holds secrets you can't share with the reviewers'
  backends.

Step 6 covers the specifics.

**The orchestrator (you) never reviews the plan yourself.** Your job is to gather context, dispatch
the panel, and collate. If you inject your own opinions as if they were a reviewer's, you destroy
the signal of how many *independent* tools agreed. You may reconcile and judge their findings during
collation, but the findings must originate from the panel.

---

## Workflow at a glance

1. **Resolve & validate** — locate the skill scripts; require an explicit plan-file path.
2. **Gather context** — copy the plan, resolve the repo, extract the files the plan references →
   temp context dir; `source` its `meta.env`.
3. **Safety baseline** — snapshot `git status` before the panel touches the repo.
4. **Pick the panel** — detect which reviewer CLIs are installed; apply any model overrides.
5. **Dispatch** — build the prompt once, then run every reviewer in parallel, read-only, with the
   repo as cwd, each writing its own review file.
6. **Collate** — synthesize all reviews (consensus-first) into `FINAL-REPORT.md`.
7. **Deliver & safety diff** — re-check `git status`; print the paths and a three-line summary.

Use a TodoList to track these — step 5 fans out into one task per reviewer and it's easy to lose one.

---

## Step 1: Resolve scripts and validate input

This skill reviews a specific plan file, so a path is **required** — there is no "current plan" to
auto-detect. If the user didn't give one, ask for it before doing anything else.

Resolve the skill directory portably (BSD `readlink` has no `-f`):

```bash
SKILL_LINK=~/.claude/skills/plan-multi-agent-review/SKILL.md
SKILL_TARGET="$(readlink "$SKILL_LINK" 2>/dev/null || echo "$SKILL_LINK")"
SKILL_DIR="$(cd "$(dirname "$SKILL_TARGET")" && pwd -P)"

PLAN_PATH="<the path the user gave>"   # e.g. ~/.claude/plans/foo.md or ./design.md
```

The gather helper (Step 2) hard-fails with a clear `❌` if `PLAN_PATH` is empty or unreadable, so
you don't need to re-validate here — just make sure you actually pass what the user intended.

## Step 2: Gather plan context

Create a temp workspace and populate it. One helper does all of it so every reviewer reads identical
inputs. Run it from inside the repo the plan targets (so it resolves the right `REPO_ROOT`), then
**`source` its `meta.env`** — that's how you get `REPO_ROOT` and `CURRENT_BRANCH` for the dispatch
cwd and the report header:

```bash
WORKDIR="$(mktemp -d)/plan-review" && mkdir -p "$WORKDIR"
"$SKILL_DIR/scripts/gather_plan_context.sh" "$PLAN_PATH" "$WORKDIR"
source "$WORKDIR/context/meta.env"   # sets PLAN_PATH, PLAN_NAME, REPO_ROOT, CURRENT_BRANCH
ls -1 "$WORKDIR/context"
```

After it runs, `$WORKDIR/context/` contains:

- `plan.md` — a copy of the plan under review (the primary artifact)
- `referenced-files/` — the repo files the plan cites, mirrored at their **real repo paths** so
  reviewers can cite findings at the correct `file:line`
- `referenced-files.txt` — the embedded relpaths, one per line
- `missing-references.txt` — path-shaped references that **don't exist** in the repo. For a plan
  this is expected for files it proposes to *create*, and a red flag when it claims to *modify*
  something absent — the panel judges which
- `repo-orientation.md` — repo root, branch, `git status`, recent commits, a bounded file list
- `meta.env` — `printf %q`-quoted `PLAN_PATH`, `PLAN_NAME`, `REPO_ROOT`, `CURRENT_BRANCH`

The grounding heuristic only ever marks a reference "missing" when it is **path-shaped** (contains
`/` or ends in a source extension) and absent from the repo — so bare code tokens like
`safe_fence()` or `set -euo pipefail` are never mis-flagged. A reference cited by basename resolves
to its real nested path via `git ls-files`. This is the skill's core grounding signal; trust it but
sanity-check `missing-references.txt` when collating.

## Step 3: Capture a safety baseline

Reviewers explore a **live repo** and three of them run with no hard sandbox (Step 6), so snapshot
the working tree before dispatch. After the panel finishes you diff against this to spot any stray
writes:

```bash
git -C "$REPO_ROOT" status --porcelain > "$WORKDIR/git-status.before"
```

## Step 4: Pick the review panel

Only run tools that are actually installed — invoking a missing binary wastes a turn and clutters
the report. Detect what's available:

```bash
"$SKILL_DIR/scripts/detect_reviewers.sh"
```

It prints one line per candidate: `available <label> <tool>` or `missing <tool>`. The candidate
roster is `claude`, `codex`, `gemini`, `opencode`, `copilot`, and `agency` (agency copilot). Build
your panel from the `available` lines and tell the user which tools were skipped and why ("`gemini`
not in PATH — skipping").

### Models and reasoning effort: pre-selected by default

By default, pass **no model flag and no effort flag** — each tool uses whatever model and reasoning
level it's configured with. This respects the user's existing setup and is what they want unless
they say otherwise.

Override only when the user explicitly names models or a reasoning effort. The interesting case is
running **the same tool more than once with different models** — e.g. "run agency copilot with a
Claude model and again with a GPT model." Model-capable tools accept `--model`/`-m`; the panel just
gets two entries for that tool, each with a distinct label so their reviews don't collide. Each
panel entry is a `label | tool | model | effort` tuple (model and effort default to empty):

| User asks for | Panel entries (label \| tool \| model \| effort) |
|---|---|
| default (no models named) | `claude\|claude\|\|`, `codex\|codex\|\|`, … one per available tool |
| "agency copilot with sonnet and with gpt-5" | `agency-sonnet\|agency\|claude-sonnet-4.5\|`, `agency-gpt5\|agency\|gpt-5\|` |
| "codex on gpt-5-codex" | `codex\|codex\|gpt-5-codex\|` |
| "Copilot with extra-high reasoning" | `copilot\|copilot\|\|xhigh` |

**Reasoning effort** is per-tool and the valid values differ — pick one the chosen tool accepts:

| Tool | Effort mechanism | Valid values |
|---|---|---|
| `copilot`, `agency` | `--effort` | `none, low, medium, high, xhigh, max` ("extra high" = `xhigh`) |
| `codex` | `model_reasoning_effort` config | `minimal, low, medium, high` |
| `opencode` | `--variant` | provider-specific, e.g. `minimal, low, high, max` |
| `claude`, `gemini` | *(none)* | an effort request is ignored with a note, the reviewer still runs |

The label is yours to choose — make it readable and unique (it becomes the review filename and the
section header). See `references/reviewer-cli-matrix.md` for exactly how each tool is invoked and
which support `--model` / `--effort`.

## Step 5: Dispatch the panel

Every reviewer gets the **same** instructions + context so differences in output reflect the
models, not the prompt. Build the prompt once — it carries the repo orientation, the missing-refs
list, every embedded referenced file, and the plan itself (last):

```bash
"$SKILL_DIR/scripts/build_prompt.sh" \
  "$WORKDIR/context" "$SKILL_DIR/references/review-prompt.md" \
  > "$WORKDIR/review-prompt.txt"
```

Why embed the context in the prompt rather than only point reviewers at files? The panel CLIs
disagree sharply on filesystem sandboxing — `opencode` hard-rejects any path outside its working
directory, others need per-tool `--add-dir` flags. Embedding guarantees every reviewer gets
byte-identical baseline inputs with zero file-permission friction; running them with the repo as
cwd then lets them explore *beyond* the embedded files for anything else they need to verify.

Read `references/review-prompt.md` yourself once so you know what you're asking the panel to
produce — it directs each reviewer to cover soundness, grounding (against the real repo),
completeness, sequencing/feasibility, reuse/over-engineering, risk, verification adequacy, and
clarity, to commit to a **verdict** (ship-as-is / revise / reject), all cited to a plan section or
`file:line`, and to wrap the whole review between `===PLAN-REVIEW-BEGIN===`/`===PLAN-REVIEW-END===`
sentinels.

Then launch **one background task per panel entry** — they're independent and slow (each is a full
agent run), so parallelism is the difference between one minute and ten. Run each **with the repo as
its working directory** so reviewers can explore the live code; `run_reviewer.sh` takes an empty
`--diff-file` because there is no diff in a plan review:

```bash
( cd "$REPO_ROOT" && "$SKILL_DIR/scripts/run_reviewer.sh" \
    --label "<label>" --tool "<tool>" --model "<model-or-empty>" \
    --effort "<effort-or-empty>" \
    --prompt-file "$WORKDIR/review-prompt.txt" \
    --diff-file "" \
    --output-file "$WORKDIR/reviews/<label>.md" --timeout 900 )
```

Launch each as a **background Bash task** (`run_in_background: true`) so they run concurrently and
you get notified as each finishes. `run_reviewer.sh` owns every per-tool quirk — headless flag,
read-only mode, model flag, stdin delivery, the timeout (with a pure-bash watchdog fallback when
neither `timeout` nor `gtimeout` is installed), capturing stdout/stderr, and **extracting the review
from between the sentinels** so TUI progress chatter doesn't pollute the review. It writes the
cleaned review to `<label>.md`, the unfiltered capture to `<label>.md.raw`, and a one-line status to
`<label>.md.status`.

### Read-only and the untrusted-input boundary

Read-only is enforced where the tool supports it (codex `--sandbox read-only`, gemini
`--approval-mode plan`, claude `--permission-mode plan`). **`copilot`, `opencode`, and `agency` have
no hard read-only switch and run with `--allow-all-tools`** — only the prompt asks them not to
write. With reviewers now pointed at a live repo as cwd, that is a real exposure: a crafted "ignore
previous instructions…" payload in a repo file or in the plan could drive a fully tool-enabled
agent. The post-run `git status` check (Step 7) catches tracked-file writes only — not reads,
network calls, or untracked files.

Mitigate, don't pretend it's airtight:

- **Only run this skill on repos and plans you trust** unless you've isolated the no-sandbox tools.
  Say so plainly when either is from an unknown source.
- For untrusted input, prefer reviewing inside a throwaway git worktree and/or with networking
  disabled for the `copilot`/`opencode`/`agency` runs.

As each task completes, glance at its `.status` and the head of its review file. `ok` = a clean
sentinel-wrapped review. `ok-empty` = the tool ran but emitted no sentinels (it likely refused or
rambled, **or a sentinel site is out of sync** — see below); the salvaged/raw output is kept so you
can judge it. `errored` = it crashed, timed out (a stub `.md` is still written so the member stays
visible to collation), auth failed, or hit **context overflow** (status will say "re-run this label
with `--model <larger-context model>`" — relay that rather than treating it as a generic crash). A
missing reviewer is information, not something to hide or silently retry more than once.

> **Sentinel contract.** The `PLAN-REVIEW-BEGIN`/`PLAN-REVIEW-END` sentinel is defined in **four
> places that must agree**: `scripts/run_reviewer.sh`, `references/review-prompt.md`, this file, and
> `references/reviewer-cli-matrix.md`. If you ever see *every* reviewer come back `ok-empty`, a
> sentinel site has drifted out of sync — check those four before blaming the models.

## Step 6: Collate into the final report

Once every task has finished (or errored), read all of `$WORKDIR/reviews/*.md`, plus
`missing-references.txt` and `plan.md`. Write `$WORKDIR/FINAL-REPORT.md` using the structure below.

The organizing principle is **consensus first**: lead with what multiple reviewers independently
agree on, because agreement across independent models is the strongest signal in the whole exercise.
Preserve disagreement honestly — if two tools call a step broken and one calls it fine, the user
needs to see the conflict, not an averaged-away mush. Keep each reviewer's raw output linked in an
appendix so nothing is lost.

```markdown
# Multi-Agent Plan Review: <plan name>

<one-paragraph orientation: what the plan proposes, how big, how many reviewers ran, the single
most important takeaway>

**Plan:** <PLAN_PATH>   ·   **Repo/branch:** <REPO_ROOT> @ <CURRENT_BRANCH>
**Panel:** <label list>   ·   **Skipped:** <tools not in PATH>

## Overall verdict
<!-- The consensus verdict (ship-as-is / revise / reject) and the one thing that most needs
     attention before the plan is executed. -->

## Consensus findings
<!-- Raised by 2+ reviewers. Highest confidence. Order by severity then by agreement count. -->
| Severity | Finding | Plan section / file:line | Flagged by |
|---|---|---|---|

## Single-reviewer findings worth surfacing
<!-- Raised by exactly one reviewer but credible. These are the sharp catches OR the false
     positives — label your read of which, and say why. -->

## Disagreements
<!-- Where reviewers contradict each other. State both positions and your reconciliation,
     or "needs author judgment" if you genuinely can't adjudicate from the plan + repo. -->

## Missing or under-specified steps
<!-- Merged + deduped across the panel: steps, files, edge cases, or call sites the plan omits. -->

## Grounding check
<!-- Does the plan match the actual repo? Surface referenced-but-missing files (new-to-create vs
     broken reference) and any wrong assumptions the panel caught about existing code. -->

## Verification adequacy
<!-- Does the plan's own verification actually prove the change works? What stays untested? -->

## Prioritized revisions
<!-- The "if you change nothing else, fix these before executing" list, severity-ordered. -->

## Appendix: raw reviews
<!-- One link per panel member to its file under reviews/, plus any that errored. -->
```

Severity vocabulary, used consistently: **Critical** (the plan will fail or cause damage as
written) · **High** (a reviewer would block execution on it) · **Medium** (should fix, not blocking)
· **Low** (minor) · **Info** (noteworthy, no action). When reviewers use different scales, normalize
to this one so the table is comparable.

## Step 7: Deliver and safety-diff

First, diff the working tree against the baseline to catch any stray writes from the no-sandbox
tools:

```bash
git -C "$REPO_ROOT" status --porcelain > "$WORKDIR/git-status.after"
diff "$WORKDIR/git-status.before" "$WORKDIR/git-status.after" || \
  echo "⚠️  working tree changed during the panel — inspect before trusting it"
```

If a reviewer wrote a stray file, note it for the user; **don't blindly revert** (you might clobber
their uncommitted work).

Then print the artifact paths plainly so the user can open them:

- `FINAL-REPORT.md` — the collated plan review
- `reviews/` — each panel member's raw output

Give a three-line spoken summary: one line on what the plan proposes, one on the consensus verdict +
most important finding, one on the biggest open question or disagreement. Point the user to the
report for the rest. Do **not** write anything into the plan file or the repo — this skill only reads
them and writes to the temp workspace.

---

## Reference files

- `references/review-prompt.md` — the shared prompt template handed to every reviewer. Read it
  before dispatching so you know what the panel was asked to do.
- `references/reviewer-cli-matrix.md` — exact headless/read-only invocation and model-flag support
  for each CLI. Consult when a reviewer behaves oddly or when adding a new tool.
