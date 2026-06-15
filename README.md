# dotfiles

Personal shell configurations, custom utility scripts, and installation automation. Uses a symlink-based approach so that `git pull` immediately updates active configs and scripts.

## Quick Setup

```bash
cd ~/code
git clone https://github.com/surajssd/dotfiles
cd dotfiles
make clone-private   # optional: clone private dotfiles repo (separate clone)
make install-all
```

## Installation

```bash
# Install everything (configs, scripts, and skills)
make install-all

# Install only scripts to ~/.local/bin
make install-local-bin

# Install only config files (shell, git, gpg, starship, tmux, etc.)
make install-configs

# Install only Claude Code skills to ~/.claude/skills
make install-skills

# Pull latest from both public and private repos
make pull-master

# Pull latest and reinstall everything
make update
```

`make` is required to install (the Go tools build via `go install`, which only
the Make targets run).

## Repository Structure

- `configs/` — Shell configs (bashrc/zshrc), git, gpg, starship, tmux, terraform, k9s
- `local-bin/` — Custom utility scripts (symlinked to `~/.local/bin`)
- `skills/` — Claude Code skills (symlinked to `~/.claude/skills/`)
- `installers/` — Installation automation scripts
- `dotfilesprivate/` — private/sensitive configs and scripts (separate git clone, not a submodule)

## How It Works

All installers create **symlinks** (not copies), so changes in this repo are immediately reflected in the home directory.

- **Scripts:** Symlinked from `local-bin/` to `~/.local/bin/`
- **Configs:** Symlinked to home directory with OS-specific handling (macOS uses zshrc, Linux uses bashrc)
- **Skills:** Symlinked from `skills/` to `~/.claude/skills/`
