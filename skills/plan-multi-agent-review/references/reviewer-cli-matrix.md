# Reviewer CLI Matrix

How each candidate reviewer is invoked headlessly, read-only, with an optional model override.
`run_reviewer.sh` encodes all of this — this doc is the human-readable source of truth behind
that script. Update both together if a CLI's flags change.

The plan context is **embedded in the prompt** (see `build_prompt.sh`), not read from files, so
reviewers don't need filesystem access to a temp dir — this matters because `opencode` hard-
rejects reads outside its working directory. Each reviewer is still run with the **target repo as
its working directory** so it can explore the actual source read-only to verify the plan's claims.
The prompt asks every tool to wrap its review in `===PLAN-REVIEW-BEGIN===`/`===PLAN-REVIEW-END===`
sentinels; `run_reviewer.sh` extracts between them, which is how the TUI progress chatter that
`copilot` and `agency` print to stdout gets stripped out of the saved review.

## Prompt delivery: stdin for every tool

The whole prompt — instructions + repo orientation + every embedded referenced file + the plan
itself — is delivered to **every** tool on **stdin** via a file redirect (`tool < file`, not a
pipe, so a tool that exits without draining stdin doesn't trigger SIGPIPE). stdin has no argv size
limit, so a large plan or large referenced files are fine; there is no truncation for any tool.
(Five of the six CLIs were verified to read the full prompt from stdin — a 450 KiB tail-token probe
round-tripped intact — see the dated note in the PR sibling skill that first established this. The
sixth, `agy`, is wired by analogy to its gemini-cli lineage and is **not yet live-verified** —
smoke-test it on a real run.)

Unlike a PR review, there is **no diff**: the plan is the primary artifact and is embedded as the
final context block. `run_reviewer.sh` is shared with the PR skill and still accepts a `--diff-file`
argument, but this skill always passes it **empty** (`--diff-file ""`), so the diff-append path is
inert and no diff block is produced.

| Tool | Delivery |
|---|---|
| `claude` | full prompt on **stdin** |
| `codex` | full prompt on **stdin** (trailing `-`) |
| `agy` | full prompt on **stdin** (`-p ""`) |
| `opencode` | full prompt on **stdin** (`run ""`) |
| `copilot` | full prompt on **stdin** (`-p ""`) |
| `agency` | full prompt on **stdin** (forwarded `-p ""`) |

### Context window (copilot / agency)

`copilot` and `agency` are run with `--context long_context`, which selects the larger
context-window tier for tiered-pricing models **without changing the user's configured model**
(verified the flag works on copilot and that `agency copilot --` forwards it). This is what lets a
large plan plus many embedded referenced files fit. If a model still overflows — it has no
long-context tier, or the embedded context exceeds even the long window — `run_reviewer.sh` detects
the overflow error and writes a status telling the user to re-run that label with `--model
<larger-context model>` rather than silently swapping the model. (The ≤10-file / ≤256 KB embed cap
in `gather_plan_context.sh` keeps this rare; the rest is left to live repo exploration.)

| Tool | Headless invocation | Read-only flag | Model flag | Effort flag | Notes |
|---|---|---|---|---|---|
| `claude` | `claude -p` (prompt on stdin) | `--permission-mode plan` | `--model <id>` | *(none)* | Plan mode can't edit/run mutating tools. No reasoning-effort flag in `-p` mode. |
| `codex` | `codex exec -` (stdin) | `--sandbox read-only` | `-m <id>` | `-c model_reasoning_effort="<lvl>"` | Trailing `-` makes `exec` read the prompt from stdin. Effort is a config override (precede the `-`). |
| `agy` | `agy -p ""` (stdin) | `--sandbox` (soft) | `--model <id>` | *(none)* | Google Antigravity CLI. No hard read-only mode; `--sandbox` is terminal-restricted **and** auto-approves so a headless run can't hang. No reasoning-effort flag. **Not yet live-verified.** |
| `opencode` | `opencode run ""` (stdin) | *(none)* | `-m provider/model` | `--variant <lvl>` | No hard read-only; rely on prompt + git check. Model needs `provider/` prefix. `--variant` is provider-specific reasoning effort. |
| `copilot` | `copilot -p ""` (stdin) | *(none)* | `--model <id>` | `--effort <lvl>` | Needs `--allow-all-tools` for non-interactive; no read-only switch. Add `--no-color` and `--context long_context`. |
| `agency` | `agency copilot -- -p ""` (stdin) | *(none)* | via pass-through `-- --model <id>` | via pass-through `-- --effort <lvl>` | `agency copilot` wraps Copilot CLI; everything after `--` is forwarded (incl. `--context long_context`). |

**Effort values differ per tool** — the orchestrator must supply one the chosen tool accepts:
`copilot`/`agency` `--effort` → `none, low, medium, high, xhigh, max`; `codex`
`model_reasoning_effort` → `minimal, low, medium, high`; `opencode` `--variant` → provider-specific
(e.g. `minimal, low, high, max`). `claude`/`agy` have none — an `--effort` passed for them is
ignored with a note on stderr and the reviewer still runs.

## Per-tool detail

### claude
```bash
claude -p --permission-mode plan [--model "$MODEL"] < prompt-file
```
Plan mode is genuinely read-only — it cannot apply edits or run mutating bash. Safest of the
panel. Reads the prompt from stdin in `-p` mode, so no argv size limit applies.

### codex
```bash
codex exec --sandbox read-only --skip-git-repo-check --color never [-m "$MODEL"] [-c model_reasoning_effort="$EFFORT"] - < prompt-file
```
`codex exec` is the non-interactive entry point (alias `codex e`); the trailing `-` makes it read
the prompt from stdin. `--sandbox read-only` blocks writes. `--skip-git-repo-check` avoids a
refusal if run from an unusual cwd. Put `-m <model>` *before* the `-` so the flag is
unambiguously a flag and not mistaken for the stdin positional (verified: codex parses `… -m X -`
fine, but flags-before-positional is the safe convention). Reasoning effort has no dedicated flag;
it's a config override (`-c model_reasoning_effort="high"`, values `minimal|low|medium|high`),
which must likewise precede the `-`.

### agy
```bash
agy -p "" --sandbox --print-timeout "${TIMEOUT}s" [--model "$MODEL"] < prompt-file
```
`agy` is the Google **Antigravity CLI** (gemini-cli lineage — note the `~/.gemini/antigravity-cli`
config path). `agy -p ""` (alias `--print`/`--prompt`) runs a single prompt non-interactively and
reads the prompt from stdin, so no argv size limit applies. Unlike the old gemini CLI it has **no
hard read-only mode** (`--approval-mode plan` does not exist here); `--sandbox` is the closest —
the binary documents it as "a sandbox with terminal restrictions" that also **auto-approves** tool
calls ("Sandbox mode: auto-approve in sandbox") and overrides the per-file "Allow access?" prompt,
so a headless run reads source freely without hanging on a confirmation. That makes agy *soft*
read-only (writes aren't hard-blocked), so it sits with copilot/opencode/agency on the trust
boundary, not with claude/codex. `--print-timeout` (default 5m) is pinned to the outer timeout so a
long review isn't truncated. Avoid `--dangerously-skip-permissions` — that auto-approves shell too,
the opposite of what a reviewer wants. **Caveat:** agy starts a local language-server process and
binds a localhost port, which the offline test sandbox blocks, so its stdin round-trip is wired by
analogy to gemini-cli and **not yet live-verified** — smoke-test on a real plan + repo.

### opencode
```bash
opencode run "" [-m "$MODEL"] [--variant "$EFFORT"] < prompt-file
```
`opencode run ""` takes an empty positional and reads the prompt from **stdin** (verified — a
450 KiB stdin prompt with its instruction at the tail round-trips intact). No hard read-only flag;
the prompt forbids edits and we diff `git status` before/after the panel. Model id must be
`provider/model` (e.g. `anthropic/claude-sonnet-4-5`), unlike the others — if the user gives a bare
model name for opencode, ask which provider or leave it default. `--variant` selects the
provider-specific reasoning-effort level (e.g. `high`, `max`, `minimal`).

### copilot
```bash
copilot -p "" --allow-all-tools --no-color --context long_context [--model "$MODEL"] [--effort "$EFFORT"] < prompt-file
```
The prompt is read from **stdin** (verified — `copilot -p "" < file`, and even `copilot < file`,
both work; a 450 KiB tail-token prompt round-trips). `--allow-all-tools` is required for
non-interactive mode or it hangs waiting for permission confirmations. `--context long_context`
selects the larger context-window tier so a large plan + embedded files fit without changing the
configured model. `--effort` (alias `--reasoning-effort`) sets the reasoning level —
`none|low|medium|high|xhigh|max` ("extra high" = `xhigh`). No read-only switch exists, so the
prompt's "do not modify files" instruction plus the post-run git check are the guardrails — see the
trust-boundary note in SKILL.md, since this tool runs fully enabled and is now pointed at a live
repo it can explore.

### agency (agency copilot)
```bash
agency copilot -- -p "" --allow-all-tools --no-color --context long_context [--model "$MODEL"] [--effort "$EFFORT"] < prompt-file
```
`agency copilot` runs GitHub Copilot CLI through Microsoft's Agency wrapper. Arguments after
`--` are forwarded to the underlying copilot, so the flags match the `copilot` row — including
reading the prompt from **stdin** (verified) and forwarding `--context long_context` and `--effort`,
and running fully tool-enabled (see the trust-boundary note in SKILL.md). This is the tool the user
most often wants to run **twice with different models** (e.g. a Claude model and a GPT model) — each
run becomes its own panel entry with a distinct label.

On its **first** invocation, `agency` downloads and caches the Copilot CLI binary (you'll see
"Downloading copilot-darwin-arm64.tar.gz…" on stderr), which adds a few seconds one time — not
a hang. Subsequent runs reuse the cached binary. Budget a generous `--timeout` for the panel
regardless, since each reviewer is a full agent run; 900s worked comfortably in testing.

Because `agency copilot` *is* Copilot underneath, an unguided review self-identifies as "GitHub
Copilot CLI" and blurs with the standalone `copilot` entry. `run_reviewer.sh` prepends an
identity line ("You are the panel member labelled `<label>`…") to every reviewer's prompt so
the review heading uses the assigned label instead. Give `agency` and `copilot` distinct labels
when both are in the panel (e.g. `agency-opus` vs `copilot`).

## Adding a new reviewer

1. Add its binary name to the `CANDIDATES` array in `scripts/detect_reviewers.sh`.
2. Add a `case` branch in `scripts/run_reviewer.sh` building its headless + read-only + model command (and its reasoning-effort flag, if it has one — else `err` a note when `--effort` is non-empty).
3. Add a row here.

Keep the three in sync — the script is what runs, this table is how a human checks it. After
adding one, smoke-test it on a real plan + repo: confirm its `.md.status` is `ok` (not
`ok-empty`, which means it ignored the sentinels) and that `<label>.md` contains a clean
review rather than tool-call traces.
