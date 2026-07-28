# Reviewer CLI Matrix

How each candidate reviewer is invoked headlessly, read-only, with an optional model override.
`run_reviewer.sh` encodes all of this — this doc is the human-readable source of truth behind
that script. Update both together if a CLI's flags change.

The plan context is **embedded in the prompt** (see `build_prompt.sh`), not read from files, so
reviewers don't need filesystem access to a temp dir — this matters because `opencode` hard-
rejects reads outside its working directory. Each reviewer is still run with the **target repo as
its working directory** so it can explore the actual source read-only to verify the plan's claims.
The prompt asks every tool to wrap its review in `===PLAN-REVIEW-BEGIN===`/`===PLAN-REVIEW-END===`
sentinels; `run_reviewer.sh` extracts between them, which is how any TUI progress chatter a tool
prints to stdout gets stripped out of the saved review.

## Prompt delivery: stdin for every tool except cursor

The whole prompt — instructions + repo orientation + every embedded referenced file + the plan
itself — is delivered to **every** tool on **stdin** via a file redirect (`tool < file`, not a
pipe, so a tool that exits without draining stdin doesn't trigger SIGPIPE). stdin has no argv size
limit, so a large plan or large referenced files are fine; there is no truncation for any tool.
(`claude`, `codex`, and `opencode` were verified to read the full prompt from stdin — a 450 KiB
tail-token probe round-tripped intact — see the dated note in the PR sibling skill that first
established this. `agy` is wired by analogy to its gemini-cli lineage and is **not yet
live-verified** — smoke-test it on a real run.)

**`cursor` is the one exception, by design, not omission.** `agent --help` documents its prompt as
a positional argv argument (`agent [options] [prompt...]`) with no documented stdin support, so
`run_reviewer.sh` reads the assembled prompt file into the argv instead
(`cursor-agent -p "$(cat prompt-file)" ...`). This reintroduces the argv-size risk the stdin design
exists to avoid for every other tool: an unusually large plan (many embedded referenced files) could
hit the OS argument-size limit (`E2BIG`) and fail outright, rather than degrading gracefully like
the context-overflow handling below. Smoke-test cursor with a realistically large plan before
relying on it.

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
| `cursor` | full prompt on **argv** (`-p "<prompt>"`) — no stdin support |

| Tool | Headless invocation | Read-only flag | Model flag | Effort flag | Notes |
|---|---|---|---|---|---|
| `claude` | `claude -p` (prompt on stdin) | `--permission-mode plan` | `--model <id>` | *(none)* | Plan mode can't edit/run mutating tools. No reasoning-effort flag in `-p` mode. |
| `codex` | `codex exec -` (stdin) | `--sandbox read-only` | `-m <id>` | `-c model_reasoning_effort="<lvl>"` | Trailing `-` makes `exec` read the prompt from stdin. Effort is a config override (precede the `-`). |
| `agy` | `agy -p ""` (stdin) | `--sandbox` (soft) | `--model <id>` | *(none)* | Google Antigravity CLI. No hard read-only mode; `--sandbox` is terminal-restricted **and** auto-approves so a headless run can't hang. No reasoning-effort flag. **Not yet live-verified.** |
| `opencode` | `opencode run ""` (stdin) | *(none)* | `-m provider/model` | `--variant <lvl>` | No hard read-only; rely on prompt + git check. Model needs `provider/` prefix. `--variant` is provider-specific reasoning effort. |
| `cursor` | `cursor-agent -p "<prompt>"` (argv) | `--plan` | `--model <id>` | *(none — pick a different model id)* | Binary is `cursor-agent` (aliased `agent`). `--plan` is a genuine hard read-only mode, same tier as `claude`/`codex`. `--trust` avoids a workspace-trust prompt hang. Grok 4.5 ids: `cursor-grok-4.5-high\|medium\|low`, each with a `-fast` variant. No reasoning-effort flag — use a different Grok id instead. |

**Effort values differ per tool** — the orchestrator must supply one the chosen tool accepts:
`codex` `model_reasoning_effort` → `minimal, low, medium, high`; `opencode` `--variant` →
provider-specific (e.g. `minimal, low, high, max`). `claude`/`agy`/`cursor` have none — an
`--effort` passed for them is ignored with a note on stderr and the reviewer still runs (for
`cursor`, a different reasoning level means picking a different `--model` id instead).

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
read-only (writes aren't hard-blocked), so it sits with opencode on the trust boundary, not with
claude/codex. `--print-timeout` (default 5m) is pinned to the outer timeout so a long review isn't
truncated. Avoid `--dangerously-skip-permissions` — that auto-approves shell too, the opposite of
what a reviewer wants. **Caveat:** agy starts a local language-server process and binds a localhost
port, which the offline test sandbox blocks, so its stdin round-trip is wired by analogy to
gemini-cli and **not yet live-verified** — smoke-test on a real plan + repo.

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

### cursor
```bash
cursor-agent -p "$(cat prompt-file)" --plan --trust --output-format text [--model "$MODEL"]
```
Binary is `cursor-agent` (the installer also symlinks a bare `agent` alias — too generic a name to
`command -v` for safely, so detection and invocation both use `cursor-agent`). `-p`/`--print` alone
is **not** read-only — cursor-agent's own `--help` says print mode "has access to all tools,
including write and shell" — so `--plan` (a genuine hard read-only mode: "analyze, propose plans,
no edits") is required, putting `cursor` in the same hard-read-only tier as `claude`/`codex`, not
the soft tier with `opencode`/`agy`. `--trust` skips the interactive workspace-trust confirmation
that would otherwise hang a headless run. Confirmed via `agent --list-models` on a real account,
the Grok 4.5 model ids are `cursor-grok-4.5-high` (the plain "Cursor Grok 4.5", and this skill's
default), `cursor-grok-4.5-medium`, `cursor-grok-4.5-low`, each with a `-fast` counterpart — there
is no separate reasoning-effort flag; a different reasoning level means picking a different one of
these ids via `--model`. **Prompt delivery is the one thing to watch**: unlike every other tool
here, `cursor-agent` takes its prompt as an argv string (`agent --help` documents no stdin support),
so the assembled prompt is read into the argv rather than piped via stdin — this reintroduces an
argv-size ceiling that the stdin design elsewhere in this skill exists to avoid. Smoke-test with a
realistically large plan.

## Adding a new reviewer

1. Add its binary name to the `CANDIDATES` array in `scripts/detect_reviewers.sh`.
2. Add a `case` branch in `scripts/run_reviewer.sh` building its headless + read-only + model command (and its reasoning-effort flag, if it has one — else `err` a note when `--effort` is non-empty).
3. Add a row here.

Keep the three in sync — the script is what runs, this table is how a human checks it. After
adding one, smoke-test it on a real plan + repo: confirm its `.md.status` is `ok` (not
`ok-empty`, which means it ignored the sentinels) and that `<label>.md` contains a clean
review rather than tool-call traces.
