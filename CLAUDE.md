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
- `installers/` - Installation automation scripts (public repo only)

## Common Commands

### Installation

```bash
# Install all configs, scripts, skills, and rules
make install-all

# Install only scripts to ~/.local/bin
make install-local-bin

# Install only config files
make install-configs

# Install only agent skills to ~/.claude/skills and ~/.agents/skills
make install-skills

# Install only agent rules to ~/.claude/rules
make install-rules

# Download external skills (mattpocock, bastos, blader) into skills/ — also run by 'make update'
make fetch-external-skills

# Download external rules (abatilo) into rules/ — also run by 'make update'
make fetch-external-rules

# Pull latest from both public and private repos
make pull-master

# Update from upstream and reinstall (pull-master + fetch-external-skills + fetch-external-rules + install-all)
make update

# Clone the private dotfiles repository
make clone-private
```

### How Installation Works

- **Scripts**: Symlinked from `local-bin/` and `dotfilesprivate/local-bin/` to `~/.local/bin/`
- **Configs**: Symlinked from `configs/` to home directory with OS-specific handling:
  - macOS: Uses `zshrc`, `gpg-agent-mac.conf`, `gpg.conf`, k9s skin to `~/Library/Application Support/k9s/skins/`
  - Linux: Uses `bashrc`, `gpg-agent-linux.conf`, k9s skin to `~/.config/k9s/skins/`
  - Both: `gitignore`, `terraformrc`, `tmux.conf`, `starship.toml`
- **Skills**: Symlinked from `skills/` and `dotfilesprivate/skills/` to `~/.claude/skills/` (Claude Code) and `~/.agents/skills/` (vendor-neutral path read by Codex, Gemini, opencode, and Copilot CLI)
- **Rules**: Symlinked from `rules/` and `dotfilesprivate/rules/` to `~/.claude/rules/` (Claude Code's global rules path)

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

All installers create symlinks (not copies) so that `git pull` immediately updates active configs and scripts. Installers use absolute paths via `realpath` or `pwd` for reliable symlinking and handle both public and private repositories in sequence. The shared symlink-loop logic (`link_tree`, `prune_dead_symlinks`) and the vendoring helpers (`die`, clone cache, `inject_attribution`) live in `installers/lib.sh`, sourced by `install-local-bin.sh`, `install-skills.sh`, `install-rules.sh`, and the `fetch-external-*.sh` scripts.

### OS-Specific Config Handling

`installers/install-configs.sh` detects the OS and symlinks the appropriate files:

- macOS (Darwin): `zshrc` → `~/.zshrc`, `gpg-agent-mac.conf` → `~/.gnupg/gpg-agent.conf`, `gpg.conf` → `~/.gnupg/gpg.conf`
- Linux: `bashrc` → `~/.bashrc`, `gpg-agent-linux.conf` → `~/.gnupg/gpg-agent.conf`
- Both: `gitignore`, `terraformrc`, `tmux.conf`, `starship.toml`

`installers/install-configs.sh` then invokes `dotfilesprivate/install-configs.sh` to symlink the private configs (see the private repo's `CLAUDE.md`).

### PATH Configuration

Shell configs (zshrc/bashrc) add these to PATH:

- `~/.local/bin` - Custom scripts from this repo
- `~/go/bin` - Go binaries
- `/opt/homebrew/bin` and `/opt/homebrew/sbin` - Homebrew on macOS

## Working with This Repository

### Adding New Scripts

1. Add executable script to `local-bin/` (or `dotfilesprivate/local-bin/` for private scripts)
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

Fetched skills are committed to the repo. Run `make fetch-external-skills` to refresh them; the script prints the upstream commit SHA(s), which should be recorded in the commit message. This script is intentionally NOT part of `install-all` (so plain installs stay offline), but `make update` does run it — after `pull-master` and before `install-all` — so a full update also refreshes the vendored skills. `install-skills.sh` then symlinks the vendored directories like any other local skill.

## Adding Agent Rules

Each rule is a standalone `.md` file under `rules/` containing plain-markdown instructions read by Claude Code as global guidance. After adding or modifying rules, run `make install-rules` to symlink them to `~/.claude/rules/`.

### Vendored external rules

Some rules are vendored (copied) from upstream repos. `installers/fetch-external-rules.sh` drives this via the same pipe-delimited registry / two-mode (`fetch`/`preserve`) pattern as `fetch-external-skills.sh`, but operates per-file on `.md` rules:

- **`fetch`**: clone the upstream repo, copy the rule `.md` into `rules/<name>` verbatim (no frontmatter or attribution is injected — rules are plain markdown). The `simple`, `comments`, `commit-notes`, `simplified-technical-english`, and `subtractive-engineering` rules are vendored this way from [`abatilo/vimrc`](https://github.com/abatilo/vimrc).
- **`preserve`**: the rule is already vendored and locally customised; the script verifies it exists and reports its source but never overwrites it.

Fetched rules are committed to the repo. Run `make fetch-external-rules` to refresh them; the script prints the upstream commit SHA, which should be recorded in the commit message. This script is intentionally NOT part of `install-all` (so plain installs stay offline), but `make update` does run it — after `fetch-external-skills` and before `install-all` — so a full update also refreshes the vendored rules. `install-rules.sh` then symlinks the vendored files like any other local rule.

Attribution for vendored rules is recorded in a hand-maintained `rules/README.md` (not generated by the fetch script). When the registry changes, update that README alongside the fetch. `install-rules.sh` skips `README.md` (listed in its `RULES_SKIP` array) so it is never symlinked into `~/.claude/rules/`.

## Important Notes

- The private repository (`dotfilesprivate/`) is a separate standalone git clone, not a submodule
- Installation scripts assume both repos are present and will attempt to process both
- Symlinks mean changes in this repo are immediately reflected in home directory
- The `make update` command pulls latest from both repositories, refreshes the vendored external skills and rules, and reinstalls
- The `install-local-bin.sh` installer skips files listed in its `LOCAL_BIN_SKIP` array (e.g. `util.sh`, a shared library, and `git-autopush-post-commit`, a git hook script — neither belongs in PATH). If you add another library or hook file to `local-bin/`, add its basename to `LOCAL_BIN_SKIP`
- The `install-rules.sh` installer skips files listed in its `RULES_SKIP` array (e.g. `README.md`, the hand-maintained attribution file — not a rule). If you add another non-rule file to `rules/`, add its basename to `RULES_SKIP`
