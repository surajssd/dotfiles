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

# Install only agent skills to ~/.claude/skills and ~/.agents/skills
make install-skills

# Install only agent rules to ~/.claude/rules
make install-rules

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
- `skills/` — Agent skills in `SKILL.md` format (symlinked to `~/.claude/skills/` and `~/.agents/skills/`)
- `rules/` — Agent rule `.md` files (symlinked to `~/.claude/rules/`)
- `installers/` — Installation automation scripts
- `dotfilesprivate/` — private/sensitive configs and scripts (separate git clone, not a submodule)

## How It Works

All installers create **symlinks** (not copies), so changes in this repo are immediately reflected in the home directory.

- **Scripts:** Symlinked from `local-bin/` to `~/.local/bin/`
- **Configs:** Symlinked to home directory with OS-specific handling (macOS uses zshrc, Linux uses bashrc)
- **Skills:** Symlinked from `skills/` to `~/.claude/skills/` (Claude Code) and `~/.agents/skills/` (the vendor-neutral path read by Codex, Gemini, opencode, and Copilot CLI)
- **Rules:** Symlinked from `rules/` to `~/.claude/rules/` (Claude Code's global rules path)

## GitHub Codespaces

GitHub Codespaces can install this repository as personal dotfiles. In your GitHub Codespaces settings, enable automatic dotfiles installation and select `surajssd/dotfiles`. Codespaces recognizes the root `install.sh` and runs the same `make install-all` setup used for local installation.

The development container must provide `make` and the Go version declared in `clawbox/go.mod`. To rerun the setup in an existing codespace:

```bash
/workspaces/.codespaces/.persistedshare/dotfiles/install.sh
```

The installer uses private configs and tools only when their optional repositories are already present. It does not clone private repositories automatically.

## License

MIT — see [LICENSE](LICENSE).

