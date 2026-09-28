# Reviewer CLI Matrix

How each candidate reviewer is invoked headlessly, read-only, with an optional model override.
`run_reviewer.sh` encodes all of this — this doc is the human-readable source of truth behind
that script. Update both together if a CLI's flags change.

The PR context is **embedded in the prompt** (see `build_prompt.sh`), not read from files, so
reviewers don't need filesystem access to a temp dir — this matters because `opencode` hard-
rejects reads outside its working directory. Each reviewer is still run with the repo as its
working directory so it can read the actual source for surrounding context. The prompt asks
every tool to wrap its review in `===PR-REVIEW-BEGIN===`/`===PR-REVIEW-END===` sentinels;
`run_reviewer.sh` extracts between them, which is how any TUI progress chatter a tool prints
to stdout gets stripped out of the saved review.

## Prompt delivery: stdin for every tool except agy and cursor

Early versions of this skill treated `opencode` as **argv-only** and passed the prompt as a
single command-line argument — bounded on **Linux to 128 KiB per argv element**
(`MAX_ARG_STRLEN`). For a large PR the argv path silently dropped the diff (telling the agent to
run `git diff` itself) or hard-truncated the prompt, so that reviewer reviewed a crippled input.

That split was based on a wrong assumption: `opencode` reads the full prompt from stdin —
verified by piping a 450 KiB prompt whose only real instruction sat at the very *tail* and
confirming the tool acted on it (so nothing was truncated). The other CLIs (`claude`, `codex`)
were verified the same way. `run_reviewer.sh` therefore delivers those three tools' prompt —
instructions + context + the full embedded diff — on **stdin** via a file redirect
(`tool < file`, not a pipe, so a tool that exits without draining stdin doesn't trigger
SIGPIPE). There is no argv cap, no diff-pointer fallback, and no truncation for those tools.

`agy` (Google Antigravity CLI) was wired the same way by analogy to its gemini-cli lineage, and
live runs proved that wrong: agy 1.2.5 and 1.2.6 exit 1 on `agy -p "" < prompt` with
`error: Error: empty prompt. Usage: agy --print "your prompt here"`. It never reads the prompt
from stdin, so its branch now passes the assembled prompt as the `-p` argv string, exactly like
cursor below, with the same argv-size caveat.

**`cursor` reopens exactly the argv problem this section describes, by design, not oversight.**
`agent --help` documents its prompt as a positional argv argument
(`agent [options] [prompt...]`) with no documented stdin support, so its `run_reviewer.sh` branch
reads the assembled prompt (instructions + context + full diff) into the argv instead of using the
stdin redirect every other tool gets. On a large PR this risks the OS argv-size limit (`E2BIG`),
the same failure class this section exists to avoid for the rest of the panel — smoke-test cursor
against a realistically large diff before trusting it here.

| Tool | Delivery |
|---|---|
| `claude` | full prompt + diff on **stdin** |
| `codex` | full prompt + diff on **stdin** (trailing `-`) |
| `agy` | full prompt + diff on **argv** (`-p "<prompt>"`) — 1.2.x rejects `-p ""` + stdin |
| `opencode` | full prompt + diff on **stdin** (`run ""`) |
| `cursor` | full prompt + diff on **argv** (`-p "<prompt>"`) — no stdin support |

| Tool | Headless invocation | Read-only flag | Model flag | Effort flag | Notes |
|---|---|---|---|---|---|
| `claude` | `claude -p` (prompt on stdin) | `--permission-mode plan` | `--model <id>` | *(none)* | Plan mode can't edit/run mutating tools. No reasoning-effort flag in `-p` mode. |
| `codex` | `codex exec -` (stdin) | `--sandbox read-only` | `-m <id>` | `-c model_reasoning_effort="<lvl>"` | Trailing `-` makes `exec` read the prompt from stdin. Effort is a config override (precede the `-`). |
| `agy` | `agy -p "<prompt>"` (argv) | `--sandbox` (soft) | `--model <id>` | *(none)* | Google Antigravity CLI. No hard read-only mode; `--sandbox` is terminal-restricted **and** auto-approves so a headless run can't hang. Headless mode auto-denies tools missing from `permissions.allow` in `~/.gemini/antigravity-cli/settings.json` and needs the repo in `trustedWorkspaces`. The runner prepends a delivery note so it does not `echo` the review through a shell. No reasoning-effort flag. Live-verified on 1.2.6 (2026-09-28). |
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
printf '%s' "$PROMPT" | claude -p --permission-mode plan [--model "$MODEL"]
```
Plan mode is genuinely read-only — it cannot apply edits or run mutating bash. Safest of the
panel. Reads the prompt from stdin in `-p` mode, so no argv size limit applies.

### codex
```bash
printf '%s' "$PROMPT" | codex exec --sandbox read-only --skip-git-repo-check --color never [-m "$MODEL"] [-c model_reasoning_effort="$EFFORT"] -
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
agy -p "$(cat prompt-file)" --sandbox --print-timeout "${TIMEOUT}s" [--model "$MODEL"]
```
`agy` is the Google **Antigravity CLI** (gemini-cli lineage — note the `~/.gemini/antigravity-cli`
config path). `-p` (alias `--print`/`--prompt`) runs a single prompt non-interactively; the prompt
must be the argument itself, because 1.2.x rejects an empty `-p ""` with the prompt on stdin
(`error: Error: empty prompt`). That puts agy on the argv path with cursor, so the OS argv-size
ceiling applies — smoke-test on a large diff. Unlike the old gemini CLI it has **no hard read-only
mode** (`--approval-mode plan` does not exist here); `--sandbox` is the closest — the binary
documents it as "a sandbox with terminal restrictions" that also **auto-approves** tool calls
("Sandbox mode: auto-approve in sandbox"). That makes agy *soft* read-only (writes aren't
hard-blocked), so it sits with opencode on the trust boundary, not with claude/codex.
`--print-timeout` (default `0s`, unbounded) is pinned to the outer timeout so agy can end its turn
before the watchdog kills it. Avoid `--dangerously-skip-permissions` — that auto-approves shell
too, the opposite of what a reviewer wants.

Three more things headless agy needs, all learned from live runs (1.2.5 on 2026-09-17, 1.2.6 on
2026-09-28):

1. **Permission allow list.** Even under `--sandbox`, headless mode auto-*denies* any tool not
   allowed in `~/.gemini/antigravity-cli/settings.json`. The symptom is exit 0, empty stdout, and
   stderr `jetski: no output produced — a tool required the "read_file" permission that headless
   mode cannot prompt for, so it was auto-denied` (a later run names `"command"`). The runner
   reports this as `errored: … auto-denied a tool permission …`. Before dispatching agy, back up
   that file, then add:
   ```json
   "permissions": {"allow": ["read_file(*)", "list_dir(*)", "grep_search(*)", "find_by_name(*)", "view_file(*)", "command(*)"]}
   ```
   and restore the backup after the panel finishes. `command(*)` auto-approves shell, which is why
   agy shares opencode's trust tier and why the list is temporary rather than permanent.
2. **Trusted workspace.** The repo's real path must appear in that file's `trustedWorkspaces`
   array or agy prompts and hangs. Each new worktree is a new path.
3. **Delivery note.** Told to "write your review to stdout", agy 1.2.6 ran a shell `echo` of the
   review inside a tool call, headless mode discarded that output, and its final message was three
   lines claiming the review "has been successfully printed to stdout" (status `ok-empty`).
   `run_reviewer.sh` now prepends a `DELIVERY NOTE` telling it that its final response text is the
   captured stdout; the same PR then returned a sentinel-clean review in 75 s. This is the one
   per-tool prompt deviation in the panel — disclose it in the final report.

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
here, `cursor-agent` takes its prompt as an argv string (`agent --help` documents no stdin
support), so the assembled prompt (instructions + context + full diff) is read into the argv
rather than piped via stdin — this reintroduces the argv-size ceiling the rest of this doc exists
to avoid. Smoke-test with a realistically large PR diff. **Stale login:** `cursor-agent status` can
print `Login successful! Logged in (unable to fetch user details)` while every `-p` run exits 1 with
`Error: Authentication required. Please run 'agent login' first, or set CURSOR_API_KEY environment
variable.` (observed 2026-09-28). The runner reports this as `errored: cursor is not logged in; run
'cursor-agent login' …`; the user runs `cursor-agent login` interactively (the orchestrator cannot),
then the label is re-run once.

## Adding a new reviewer

1. Add its binary name to the `CANDIDATES` array in `scripts/detect_reviewers.sh`.
2. Add a `case` branch in `scripts/run_reviewer.sh` building its headless + read-only + model command (and its reasoning-effort flag, if it has one — else `err` a note when `--effort` is non-empty).
3. Add a row here.

Keep the three in sync — the script is what runs, this table is how a human checks it. After
adding one, smoke-test it on a real checked-out PR: confirm its `.md.status` is `ok` (not
`ok-empty`, which means it ignored the sentinels) and that `<label>.md` contains a clean
review rather than tool-call traces.
