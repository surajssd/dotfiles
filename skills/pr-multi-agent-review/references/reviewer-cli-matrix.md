# Reviewer CLI Matrix

How each candidate reviewer is invoked headlessly, read-only, with an optional model override.
`run_reviewer.sh` encodes all of this — this doc is the human-readable source of truth behind
that script. Update both together if a CLI's flags change.

The PR context is **embedded in the prompt** (see `build_prompt.sh`), not read from files, so
reviewers don't need filesystem access to a temp dir — this matters because `opencode` hard-
rejects reads outside its working directory. Each reviewer is still run with the repo as its
working directory so it can read the actual source for surrounding context. The prompt asks
every tool to wrap its review in `===PR-REVIEW-BEGIN===`/`===PR-REVIEW-END===` sentinels;
`run_reviewer.sh` extracts between them, which is how the TUI progress chatter that `copilot`
and `agency` print to stdout gets stripped out of the saved review.

| Tool | Headless invocation | Read-only flag | Model flag | Notes |
|---|---|---|---|---|
| `claude` | `claude -p "<prompt>"` | `--permission-mode plan` | `--model <id>` | Plan mode can't edit/run mutating tools. |
| `codex` | `codex exec "<prompt>"` | `--sandbox read-only` | `-m <id>` | `exec` is the non-interactive subcommand. |
| `gemini` | `gemini -p "<prompt>"` | `--approval-mode plan` | `-m <id>` | `plan` approval mode is read-only. |
| `opencode` | `opencode run "<prompt>"` | *(none)* | `-m provider/model` | No hard read-only; rely on prompt + git check. Model needs `provider/` prefix. |
| `copilot` | `copilot -p "<prompt>"` | *(none)* | `--model <id>` | Needs `--allow-all-tools` for non-interactive; no read-only switch, so rely on prompt + git check. Add `--no-color`. |
| `agency` | `agency copilot -- -p "<prompt>"` | *(none)* | via pass-through `-- --model <id>` | `agency copilot` wraps Copilot CLI; everything after `--` is forwarded. Same caveats as copilot. |

## Per-tool detail

### claude
```bash
claude -p "$PROMPT" --permission-mode plan [--model "$MODEL"]
```
Plan mode is genuinely read-only — it cannot apply edits or run mutating bash. Safest of the
panel. Reads stdin too, but passing the prompt as an arg is simplest.

### codex
```bash
codex exec "$PROMPT" --sandbox read-only --skip-git-repo-check [-m "$MODEL"]
```
`codex exec` is the non-interactive entry point (alias `codex e`). `--sandbox read-only` blocks
writes. `--skip-git-repo-check` avoids a refusal if run from an unusual cwd. Add `--color never`
to keep output clean if needed.

### gemini
```bash
gemini -p "$PROMPT" --approval-mode plan [-m "$MODEL"]
```
`--approval-mode plan` is the read-only mode ("prompt for approval" vs "plan = read-only").
Avoid `-y/--yolo` here — that's the opposite of what we want for a reviewer.

### opencode
```bash
opencode run "$PROMPT" [-m "$MODEL"]
```
No hard read-only flag. The prompt forbids edits; we additionally diff `git status` before/after
the whole panel to catch stray writes. Model id must be `provider/model` (e.g.
`anthropic/claude-sonnet-4-5`), unlike the others — if the user gives a bare model name for
opencode, ask which provider or leave it default.

### copilot
```bash
COPILOT_ALLOW_ALL=1 copilot -p "$PROMPT" --allow-all-tools --no-color [--model "$MODEL"]
```
Non-interactive mode requires `--allow-all-tools` (or the env var) or it will hang waiting for
permission confirmations. No read-only switch exists, so the prompt's "do not modify files"
instruction plus the post-run git check are the guardrails.

### agency (agency copilot)
```bash
agency copilot -- -p "$PROMPT" --allow-all-tools --no-color [--model "$MODEL"]
```
`agency copilot` runs GitHub Copilot CLI through Microsoft's Agency wrapper. Arguments after
`--` are forwarded to the underlying copilot, so the flags match the `copilot` row. This is the
tool the user most often wants to run **twice with different models** (e.g. a Claude model and a
GPT model) — each run becomes its own panel entry with a distinct label.

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
2. Add a `case` branch in `scripts/run_reviewer.sh` building its headless + read-only + model command.
3. Add a row here.

Keep the three in sync — the script is what runs, this table is how a human checks it. After
adding one, smoke-test it on a real checked-out PR: confirm its `.md.status` is `ok` (not
`ok-empty`, which means it ignored the sentinels) and that `<label>.md` contains a clean
review rather than tool-call traces.
