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
- `skills/` - Claude Code skills (symlinked to `~/.claude/skills/`)
- `installers/` - Installation automation scripts (public repo only)

## Common Commands

### Installation

```bash
# Install all configs, scripts, and skills
make install-all

# Install only scripts to ~/.local/bin
make install-local-bin

# Install only config files
make install-configs

# Install only Claude Code skills to ~/.claude/skills
make install-skills

# Pull latest from both public and private repos
make pull-master

# Update from upstream and reinstall (pull-master + install-all)
make update

# Clone the private dotfiles repository
make clone-private
```

### How Installation Works

- **Scripts**: Symlinked from `local-bin/` and `dotfilesprivate/local-bin/` to `~/.local/bin/`
- **Configs**: Symlinked from `configs/` to home directory with OS-specific handling:
  - macOS: Uses `zshrc`, `gpg-agent-mac.conf`, `gpg.conf`, k9s skin to `~/Library/Application Support/k9s/skins/`
  - Linux: Uses `bashrc`, `gpg-agent-linux.conf`, k9s skin to `~/.config/k9s/skins/`
  - Both: `gitignore`, `terraformrc`, `tmux.conf`, `starship.toml`, `global-claude-config.md`
- **Skills**: Symlinked from `skills/` and `dotfilesprivate/skills/` to `~/.claude/skills/`

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

All installers create symlinks (not copies) so that `git pull` immediately updates active configs and scripts. Installers use absolute paths via `realpath` or `pwd` for reliable symlinking and handle both public and private repositories in sequence.

### OS-Specific Config Handling

`installers/install-configs.sh` detects the OS and symlinks the appropriate files:

- macOS (Darwin): `zshrc` → `~/.zshrc`, `gpg-agent-mac.conf` → `~/.gnupg/gpg-agent.conf`, `gpg.conf` → `~/.gnupg/gpg.conf`
- Linux: `bashrc` → `~/.bashrc`, `gpg-agent-linux.conf` → `~/.gnupg/gpg-agent.conf`
- Both: `gitignore`, `terraformrc`, `tmux.conf`, `starship.toml`, and `global-claude-config.md` → `~/.claude/CLAUDE.md`

### Global Claude Config

`configs/global-claude-config.md` is symlinked to `~/.claude/CLAUDE.md` during config installation. This provides system-wide Claude instructions that apply across all projects (e.g., use `rg` instead of `grep`, run `shellfmt.sh` after writing scripts).

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

## Adding Claude Code Skills

Each skill is a subdirectory under `skills/` containing a `SKILL.md` file that defines the skill's behavior, triggers, and allowed tools. After adding or modifying skills, run `make install-skills` to symlink them to `~/.claude/skills/`.

## Important Notes

- The private repository (`dotfilesprivate/`) is a separate standalone git clone, not a submodule
- Installation scripts assume both repos are present and will attempt to process both
- Symlinks mean changes in this repo are immediately reflected in home directory
- The `make update` command pulls latest from both repositories and reinstalls
- The `install-local-bin.sh` installer skips `util.sh` (it's a shared library, not a standalone script). If you add another library file to `local-bin/`, update the installer's skip list
