# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is a dotfiles repository containing personal shell configurations, custom utility scripts, and installation automation. The repository manages both public and private dotfiles through a dual-repository structure.

## Repository Structure

The repository follows a two-tier architecture:

- **Public repository** (`./`): Contains shareable configurations and scripts
- **Private repository** (`./dotfilesprivate/`): Separate git clone (not a submodule) containing private/sensitive scripts and configs. It is `.gitignore`'d and cloned via `make clone-private`

Both repositories mirror the same structure:

- `configs/` - Shell configurations, git configs, and tool settings
- `local-bin/` - Custom utility scripts
- `skills/` - Agent skills in `SKILL.md` format (symlinked to `~/.claude/skills/` and `~/.agents/skills/`)
- `rules/` - Agent rule `.md` files (symlinked to `~/.claude/rules/`)
- `planner/` - Go module for the `planner` command (public repo only, installed with `go install`)
- `asana/` - Go module for the `asana` task-creation command (public repo only, installed with `go install`)
- `installers/` - Installation automation scripts (public repo only)
- `containers/` - Container image builds, e.g. `openclaw` (public repo only, not installed)

## Common Commands

### Installation

```bash
# Install all configs, scripts, skills, rules, and Go commands
make install-all

# Install only scripts to ~/.local/bin
make install-local-bin

# Install only config files
make install-configs

# Install only agent skills to ~/.claude/skills and ~/.agents/skills
make install-skills

# Install only agent rules to ~/.claude/rules
make install-rules

# Build and install the planner Go command (skipped when go is absent)
make install-planner

# Build and install the asana Go command (skipped when go is absent)
make install-asana

# Download external skills (mattpocock, bastos, blader) into skills/ — also run by 'make update'
make fetch-external-skills

# Verify vendored rules in rules/ (fetches any fetch-mode entries) — also run by 'make update'
make fetch-external-rules

# Pull latest from both public and private repos
make pull-master

# Update from upstream and reinstall (pull-master + fetch-external-skills + fetch-external-rules + install-all)
make update

# Clone the private dotfiles repository
make clone-private
```

### How Installation Works

- **Scripts**: Symlinked from `local-bin/` to `~/.local/bin/`
- **Configs**: Symlinked from `configs/` to home directory with OS-specific handling:
  - macOS: Uses `zshrc`, `gpg-agent-mac.conf`, `gpg.conf`, ghostty config to `~/Library/Application Support/com.mitchellh.ghostty/`, k9s skin to `~/Library/Application Support/k9s/skins/`
  - Linux: Uses `bashrc`, `gpg-agent-linux.conf`, k9s skin to `~/.config/k9s/skins/`
  - Both: `gitignore`, `terraformrc`, `tmux.conf`, `starship.toml`, herdr config to `~/.config/herdr/config.toml`
- **Skills**: Symlinked from `skills/` to `~/.claude/skills/` (Claude Code) and `~/.agents/skills/` (vendor-neutral path read by Codex, Gemini, opencode, and Copilot CLI)
- **Rules**: Symlinked from `rules/` to `~/.claude/rules/` (Claude Code's global rules path)
- **Planner**: Built from `planner/` by `installers/install-planner.sh` with `go -C planner install .` into `$(go env GOPATH)/bin`. When `go` is absent the installer prints an `ℹ️` line and skips; when present, the build may download Go modules or a toolchain and a build failure fails `make install-all`
- **Asana**: Built from `asana/` by `installers/install-asana.sh` with `go -C asana install .`, under the same conditions as Planner
- **Private files**: Installed by the private repository's own entry point when the optional clone exists

## Shell Script Conventions

All shell scripts must follow these standards:

- **Shebang**: `#!/usr/bin/env bash`
- **Error handling**: `set -euo pipefail`
- **Formatting**: 4-space indentation via `shfmt -i 4`
- **Linting**: Must pass `shellcheck`
- **Shared utilities**: Source `util.sh` via `source "$(dirname "${BASH_SOURCE[0]}")"/util.sh` (provides `err()` for stderr output)
- **Output prefixes**: Use emoji for status messages: `❌` errors, `✅` success, `⏳` in-progress, `ℹ️` info
- **Validation**: After writing or modifying any script, always run `shellfmt.sh <script-path>` which runs both `shellcheck` and `shfmt`

## Key Architecture Patterns

### Symlink-Based Installation

Installers normally create symlinks so that `git pull` immediately updates active configs and scripts; `planner` and `asana` are compiled Go commands that need `make install-planner` and `make install-asana` after a pull. Public installers use absolute paths via `realpath` or `pwd`. The `install-all` target invokes the optional private installer once, without exposing private installation details to the public component installers. The shared symlink-loop logic (`link_tree`, `prune_dead_symlinks`) and the vendoring helpers (`die`, clone cache, `inject_attribution`) live in `installers/lib.sh`, sourced by `install-local-bin.sh`, `install-skills.sh`, `install-rules.sh`, and the `fetch-external-*.sh` scripts.

### OS-Specific Config Handling

`installers/install-configs.sh` detects the OS and symlinks the appropriate files:

- macOS (Darwin): `zshrc` → `~/.zshrc`, `gpg-agent-mac.conf` → `~/.gnupg/gpg-agent.conf`, `gpg.conf` → `~/.gnupg/gpg.conf`, `ghostty/config.ghostty` → `~/Library/Application Support/com.mitchellh.ghostty/config.ghostty`
- Linux: `bashrc` → `~/.bashrc`, `gpg-agent-linux.conf` → `~/.gnupg/gpg-agent.conf`
- Both: `gitignore`, `terraformrc`, `tmux.conf`, `starship.toml`, `herdr/config.toml` → `~/.config/herdr/config.toml`

The shell configs share alias files from `configs/aliases/`: `zshrc` sources every file in the directory, while `bashrc` sources only the portable ones (`aliases/k8s` uses zsh-only completion constructs).

### PATH Configuration

Shell configs (zshrc/bashrc) add these to PATH:

- `~/.local/bin` - Custom scripts from this repo
- `~/go/bin` - Go binaries
- `/opt/homebrew/bin` and `/opt/homebrew/sbin` - Homebrew on macOS

## Working with This Repository

### Adding New Scripts

1. Add an executable script to `local-bin/`
2. Ensure it follows the shell script conventions above (shebang, `set -euo pipefail`, etc.)
3. Run `shellfmt.sh <script-path>` to lint and format
4. Run `make install-local-bin` to symlink to `~/.local/bin`

### Modifying Existing Scripts

1. Edit the script in-place (symlinks mean changes are live immediately)
2. Run `shellfmt.sh <script-path>` to lint and format; fix any issues it reports

### Adding New Configs

1. Add config file to `configs/`
2. Add the symlink command to `installers/install-configs.sh` (follow existing patterns for OS-specific handling)
3. Run `make install-configs` to apply

### Modifying Existing Configs

Since configs are symlinked, editing the file in the repo immediately affects the active config. No reinstall needed unless adding new files.

## Commit Convention

This repository uses [Conventional Commits](https://www.conventionalcommits.org/) format. Scope should reflect the component being changed (e.g., `feat(litellm-proxy):`, `fix(shell):`, `docs(conventional-commits):`).

## Adding Agent Skills

Each skill is a subdirectory under `skills/` containing a `SKILL.md` file that defines the skill's behavior, triggers, and allowed tools. After adding or modifying skills, run `make install-skills` to symlink them to `~/.claude/skills/` (Claude Code) and `~/.agents/skills/` (the shared path read by Codex, Gemini, opencode, and Copilot CLI).

### Vendored external skills

Some skills are vendored (copied) from upstream repos rather than authored here. `installers/fetch-external-skills.sh` drives this via a pipe-delimited registry with two modes:

- **`fetch`**: clone the upstream repo, copy the skill directory flat into `skills/<name>/` (dropping any category nesting, excluding repo infrastructure like `.git`/`.github`/`.claude-plugin`), and merge `license: MIT` + `metadata.author` into its `SKILL.md` (idempotent and merge-aware — safe for upstreams that already carry some of these keys). The `grilling`, `domain-modeling`, and `grill-with-docs` skills are vendored this way from [`mattpocock/skills`](https://github.com/mattpocock/skills); `humanizer` is vendored from [`blader/humanizer`](https://github.com/blader/humanizer) (its `SKILL.md` lives at the repo root, so `subpath` is `.`).
- **`preserve`**: the skill is already vendored and locally customised, so the script verifies it exists and reports its source but never overwrites it. `conventional-commits` (from [`bastos/skills`](https://github.com/bastos/skills)) uses this mode — it carries local edits (a macOS clipboard section and a `README.md`) that must not be clobbered.

Fetched skills are committed to the repo. Run `make fetch-external-skills` to refresh them; the script prints the upstream commit SHA(s), which should be recorded in the commit message. This script is intentionally NOT part of `install-all` (so plain installs never fetch skills; public installers use the network only for Go module or toolchain downloads for `planner` and `asana`, and only when Go is present), but `make update` runs it after `pull-master` and before `install-all`, so a full update also refreshes the vendored skills. `install-skills.sh` then symlinks the vendored directories like any other local skill.

## Adding Agent Rules

Each rule is a standalone `.md` file under `rules/` containing plain-markdown instructions read by Claude Code as global guidance. After adding or modifying rules, run `make install-rules` to symlink them to `~/.claude/rules/`.

### Vendored external rules

Some rules are vendored (copied) from upstream repos. `installers/fetch-external-rules.sh` drives this via the same pipe-delimited registry / two-mode (`fetch`/`preserve`) pattern as `fetch-external-skills.sh`, but operates per-file on `.md` rules:

- **`fetch`**: clone the upstream repo, copy the rule `.md` into `rules/<name>` verbatim (no frontmatter or attribution is injected — rules are plain markdown). No rule currently uses this mode.
- **`preserve`**: the rule is already vendored and either locally customised or no longer available upstream; the script verifies it exists and reports its source but never overwrites it. The `simple`, `comments`, `commit-notes`, `simplified-technical-english`, and `subtractive-engineering` rules use this mode. They were vendored from [`abatilo/vimrc`](https://github.com/abatilo/vimrc) at `d4e1614`, and upstream later deleted its `rules/` directory in favour of a single `AGENTS.md`, so the files are frozen at that version.

Fetched rules are committed to the repo. Run `make fetch-external-rules` to refresh them; the script prints the upstream commit SHA, which should be recorded in the commit message. This script is intentionally NOT part of `install-all` (same offline reasoning as the skills fetch), but `make update` does run it — after `fetch-external-skills` and before `install-all` — so a full update also refreshes the vendored rules. `install-rules.sh` then symlinks the vendored files like any other local rule.

Attribution for vendored rules is recorded in a hand-maintained `rules/README.md` (not generated by the fetch script). When the registry changes, update that README alongside the fetch. `install-rules.sh` skips `README.md` (listed in its `RULES_SKIP` array) so it is never symlinked into `~/.claude/rules/`.

## Planner

`planner/` is a flat `package main` Go module (`github.com/surajssd/dotfiles/planner`, `go 1.25`) built on `cobra`, `yaml.v3`, and `x/term`. It manages a folder of Markdown plans with YAML front matter (`Type`, `ImplementationStatus`, `StatusChecked`, `StatusNote`, optional `Parent` and `SupersededBy` references, optional `Issues` and `PullRequests` URL lists), filed as `<root>/github.com/<org>/<repo>/<YYMMDDHHMMSS>-<name>.md`:

- `planner get [--all] [-o wide|json|yaml|name] [--no-headers] [--repo <text>] [--status <status>] [<plan>...]` prints active plans as a `kubectl`-style table (`NAME`, `REPO`, `STATUS`, `CHECKED`, `TITLE`), or every plan with `--all`, including plans without valid front matter (shown with `-` for STATUS and CHECKED); `planner tree` prints the same plans as an effort tree and supports only `-o wide`. Named plans are always shown, a name that matches nothing is reported after the found rows with exit status 1, and `tree <plan>` prints that plan's subtree. `planner describe [--github] <plan>...` prints one block per plan with its fields, children, full note, and check findings; `--github` runs `gh pr view` for each listed pull request and prints its state (OPEN, CLOSED, MERGED, or the reason gh gave none) after the URL. Plan names are a path, a wikilink, a basename, the short `NAME` from the table, or a URL, which selects every plan listing it. `-o wide` adds `TYPE`, `PATH` (home shown as `~`), and `NOTE`; `-o json` and `-o yaml` print every field including the path (one named plan is a single object, otherwise a list under `items`); `--repo` is a case-insensitive substring match on `<org>/<repo>`; `planner get repos` prints every `<org>/<repo>` that has plans, one per line, whatever their status; `--status` keeps plans with that `ImplementationStatus` (any case, hyphens optional) and shows `Implemented` and `Superseded` plans without `--all`. A bare `planner` prints help; `planner version` prints build information; `planner completion <shell>` prints a completion script that `zshrc` and `bashrc` source, with completion of plan names, repositories, statuses, and formats.
- `planner check [<plan>...]` prints a table of findings (`FILE`, `REPO`, `SEVERITY`, `RULE`, `DETAIL`) and exits 1 when an error-severity rule fires; with plan names only their findings are shown, and a name that matches nothing is reported with exit status 1. `--github` fetches the state of every pull request the plans in scope list (active plans, or the named ones) through `gh pr view`, four at a time, and adds two advisory rules: `stale-pr-note` when a sentence of `StatusNote` calls a listed pull request open (by URL or `#<n>`) while GitHub says CLOSED or MERGED, and `pr-state-unknown` when gh could not answer. `planner check --help` lists the front matter keys, the status values, and the sixteen rules, which cover link URLs, unknown keys, successors of `Superseded` plans, and links in `StatusNote` as well as in the body. The `duplicate-title` rule omits plans explicitly replaced through `SupersededBy` by another plan with the same title in the same repository folder.
- `planner create [--parent <ref>] [--status <status> --note <text>] [--issue <url>]... [--pr <url>]... <name...>` creates a plan for the current Git checkout and prints its path. `--status` needs `--note`; with piped front matter the flags rewrite those lines and merge the URLs into the lists it already has.
- `planner log <plan> <title...> [--note <text>] [--keep-checked] [--issue <url>]... [--pr <url>]...` appends `### <today>: <title>` and the text piped on stdin to the end of the plan's `## Progress log` section (before the next level one or two heading, ignoring headings in fenced code), adds the section at the end of the file when it is missing, sets `StatusChecked` to today, and with `--note` replaces `StatusNote` in the same write. Use `--keep-checked` for administrative entries that do not verify implementation status. The repeatable `--issue` and `--pr` flags add links in the same write and skip duplicates. Read the entry from a file with `< entry.md`. Everything else in the file is left byte for byte, with the file's own line endings.
- `planner set status <plan> [<ImplementationStatus>] [--note <text> | --note-file <path>] [--superseded-by <ref>]` rewrites only the status lines of a plan's front matter, sets `StatusChecked` to today, and adds a block to a plan that has none when both a status and a note are given. `--note-file` reads the note from a file, or from stdin with `-`, and drops blank lines at either end. `--superseded-by` needs the status to be `Superseded`. `<plan>` is a path, a wikilink, a basename, the short name from the tree, or a URL the plan lists.
- `planner set parent <plan> <ref>` replaces `Parent` with a dumped plan (stored as a wikilink), a file path, or a URL, refuses a self reference or a cycle, and leaves `StatusChecked` alone. Clearing a parent is a hand edit.
- `planner set pr <plan> <url>...` and `planner set issue <plan> <url>...` append URLs to `PullRequests` and `Issues`, skip duplicates, leave `StatusChecked` alone, and print the resulting list. Removing a URL is a hand edit.

The plan root comes from `~/.planner.yaml` (`root: ~/plans`); `--root <dir>` overrides it. The repository does not ship that file.

Go conventions: run `gofmt`, `go vet ./...`, `golangci-lint run`, and `go test ./...` from `planner/`. Tests compare against golden files under `planner/testdata/`; regenerate them with `go test ./... -update` after an intentional output change. Renovate's existing `gomod` rule covers `planner/go.mod`.

## Asana

`asana/` is a flat `package main` Go module (`github.com/surajssd/dotfiles/asana`, `go 1.25`) built on Cobra and `yaml.v3`. Install it with `make install-asana`; `make install-all` also builds it when Go is available. The binary goes into `$(go env GOPATH)/bin` and must be rebuilt after code changes.

`asana add <title>` creates one task in one project, assigns it to `me`, and prints the permalink URL. The title must be exactly one nonblank argument. `--project/-p` resolves a configured alias first, then a project URL. `--description/-d` supplies plain-text notes, with `-` reading stdin. `--due` accepts `YYYY-MM-DD`, `today`, or `tomorrow` and defaults to `today` in local time. No task listing, editing, arbitrary assignees, OAuth, caching, version command, or completion is provided.

Configuration comes from `~/.asana.yaml`, overridden by global `--config`. The only keys are `token_command` (an optional argument list), `default_project` (an optional alias), and `projects` (a nonempty alias-to-URL map). Copy project URLs from the browser address bar while viewing a project. Accept `https://app.asana.com/1/<workspace>/project/<gid>`, optionally ending in `/list` or `/board`, and the older `https://app.asana.com/0/<gid>/...`, including query strings. Reject bare GIDs, unknown keys, invalid URLs, and defaults that are not aliases. Public configuration examples use fictitious project IDs; user configuration belongs outside the public repository.

Create a personal access token using [Asana's token setup guide](https://developers.asana.com/docs/personal-access-token). On macOS, store it once with `security add-generic-password -a "$USER" -s ASANA_ACCESS_TOKEN -w`, which prompts for the token, and configure `token_command: [security, find-generic-password, -s, ASANA_ACCESS_TOKEN, -w]`. A nonempty `ASANA_ACCESS_TOKEN` takes precedence. Otherwise, execute `token_command` without a shell, trim trailing whitespace from stdout, and fail on a nonzero exit or empty token, including stderr. The command has a 30-second timeout. Never store tokens in YAML or logs. See [README.md](README.md#asana) for the full configuration example and usage.

Validate title, configuration, project, due date, and description before credential lookup. Send one `POST /tasks?opt_fields=permalink_url` with `name`, one project GID, `assignee: "me"`, optional `notes`, and `due_on`. Inject the HTTP client, API base URL, clock, and standard streams through `dependencies`. HTTP requests time out after 30 seconds. SIGINT and SIGTERM cancel the command context. Do not retry or follow redirects. Transport errors and unreadable success responses must say that the task may exist and should be checked before retrying. Success exits 0 and prints only the permalink; errors exit 1 and print an `error:` message to stderr. Help needs no configuration or credentials.

Run `gofmt -w *.go`, `go vet ./...`, `golangci-lint run`, and `go test ./...` from `asana/`. Integration tests run Cobra in process with `httptest.Server`, a temporary `HOME`, and a stub token command. They must not use real credentials or create real Asana tasks. Run `shellfmt.sh` on changed shell scripts.

## Important Notes

- The private repository (`dotfilesprivate/`) is a separate standalone git clone, not a submodule
- Installation scripts assume both repos are present and will attempt to process both
- Symlinks mean changes in this repo are immediately reflected in home directory
- The `make update` command pulls latest from both repositories, refreshes the vendored external skills and rules, and reinstalls
- The `install-local-bin.sh` installer skips files listed in its `LOCAL_BIN_SKIP` array (e.g. `util.sh`, a shared library, and `git-autopush-post-commit`, a git hook script — neither belongs in PATH). If you add another library or hook file to `local-bin/`, add its basename to `LOCAL_BIN_SKIP`
- The `install-rules.sh` installer skips files listed in its `RULES_SKIP` array (e.g. `README.md`, the hand-maintained attribution file — not a rule). If you add another non-rule file to `rules/`, add its basename to `RULES_SKIP`
