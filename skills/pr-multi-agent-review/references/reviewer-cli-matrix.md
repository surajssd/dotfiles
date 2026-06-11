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

## Prompt delivery: stdin vs argv

The diff can be large, and on **Linux a single argv element is capped at 128 KiB**
(`MAX_ARG_STRLEN`) — independent of the much larger total `ARG_MAX`. So delivery is split by
what each CLI empirically supports (verified by piping a probe prompt and checking the echoed
token):

| Tool | Reads piped stdin? | Delivery used |
|---|---|---|
| `claude` | ✅ | full prompt + diff on **stdin** |
| `codex` | ✅ (with trailing `-`) | full prompt + diff on **stdin** |
| `gemini` | ✅ (with `-p ""`) | full prompt + diff on **stdin** |
| `opencode` | ❌ (ignores stdin) | prompt as **argv**; oversize diff omitted → agent runs `git diff` itself |
| `copilot` | ❌ (ignores stdin) | prompt as **argv**; same oversize fallback |
| `agency` | ❌ (ignores stdin) | prompt as **argv**; same oversize fallback |

For the argv-only tools, `run_reviewer.sh` measures the **whole assembled prompt** (instructions
+ context + diff) against a safe cap (~96 KiB). If it's over, it omits the embedded diff and tells
the agent — which runs in the repo working tree — to obtain it with `git diff <base>...HEAD`; if
even the diff-less prompt is over the cap, it hard-truncates as a last resort. That's why
`run_reviewer.sh` takes the diff via `--diff-file`/`--base` separately from the diff-less
`--prompt-file`.

| Tool | Headless invocation | Read-only flag | Model flag | Notes |
|---|---|---|---|---|
| `claude` | `claude -p` (prompt on stdin) | `--permission-mode plan` | `--model <id>` | Plan mode can't edit/run mutating tools. |
| `codex` | `codex exec -` (stdin) | `--sandbox read-only` | `-m <id>` | Trailing `-` makes `exec` read the prompt from stdin. |
| `gemini` | `gemini -p "" ` (stdin appended) | `--approval-mode plan` | `-m <id>` | Also needs `--skip-trust` or it refuses in an "untrusted directory". |
| `opencode` | `opencode run "<prompt>"` | *(none)* | `-m provider/model` | argv-only. No hard read-only; rely on prompt + git check. Model needs `provider/` prefix. |
| `copilot` | `copilot -p "<prompt>"` | *(none)* | `--model <id>` | argv-only. Needs `--allow-all-tools` for non-interactive; no read-only switch. Add `--no-color`. |
| `agency` | `agency copilot -- -p "<prompt>"` | *(none)* | via pass-through `-- --model <id>` | argv-only. `agency copilot` wraps Copilot CLI; everything after `--` is forwarded. |

## Per-tool detail

### claude
```bash
printf '%s' "$PROMPT" | claude -p --permission-mode plan [--model "$MODEL"]
```
Plan mode is genuinely read-only — it cannot apply edits or run mutating bash. Safest of the
panel. Reads the prompt from stdin in `-p` mode, so no argv size limit applies.

### codex
```bash
printf '%s' "$PROMPT" | codex exec --sandbox read-only --skip-git-repo-check --color never [-m "$MODEL"] -
```
`codex exec` is the non-interactive entry point (alias `codex e`); the trailing `-` makes it read
the prompt from stdin. `--sandbox read-only` blocks writes. `--skip-git-repo-check` avoids a
refusal if run from an unusual cwd. Put `-m <model>` *before* the `-` so the flag is
unambiguously a flag and not mistaken for the stdin positional (verified: codex parses `… -m X -`
fine, but flags-before-positional is the safe convention).

### gemini
```bash
printf '%s' "$PROMPT" | gemini -p "" --approval-mode plan --skip-trust [-m "$MODEL"]
```
`gemini -p ""` runs headless and appends piped stdin to the (empty) prompt value, so the prompt
arrives via stdin (no argv limit). `--approval-mode plan` is the read-only mode. **`--skip-trust`
is required**: without it gemini refuses to run in a directory it hasn't been told to trust and
exits immediately (this is why an unguarded gemini run shows up as an instant error). Avoid
`-y/--yolo` — that's the opposite of what we want for a reviewer.

### opencode
```bash
opencode run "$PROMPT" [-m "$MODEL"]
```
**argv-only** — `opencode run` ignores stdin and takes the message as a positional argument, so a
huge prompt risks `E2BIG` on Linux; `run_reviewer.sh` caps it and falls back to a "run `git diff`
yourself" pointer. No hard read-only flag; the prompt forbids edits and we diff `git status`
before/after the panel. Model id must be `provider/model` (e.g. `anthropic/claude-sonnet-4-5`),
unlike the others — if the user gives a bare model name for opencode, ask which provider or leave
it default.

### copilot
```bash
copilot -p "$PROMPT" --allow-all-tools --no-color [--model "$MODEL"]
```
**argv-only** — ignores stdin (a piped-only prompt makes it hang or wander), so the prompt goes in
argv with the oversize fallback above. `--allow-all-tools` is required for non-interactive mode or
it hangs waiting for permission confirmations. No read-only switch exists, so the prompt's "do not
modify files" instruction plus the post-run git check are the guardrails — see the trust-boundary
note in SKILL.md, since this tool runs fully enabled.

### agency (agency copilot)
```bash
agency copilot -- -p "$PROMPT" --allow-all-tools --no-color [--model "$MODEL"]
```
`agency copilot` runs GitHub Copilot CLI through Microsoft's Agency wrapper. Arguments after
`--` are forwarded to the underlying copilot, so the flags match the `copilot` row — including
being **argv-only** (ignores stdin) and running fully tool-enabled (see the trust-boundary note
in SKILL.md). This is the tool the user most often wants to run **twice with different models**
(e.g. a Claude model and a GPT model) — each run becomes its own panel entry with a distinct
label.

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
