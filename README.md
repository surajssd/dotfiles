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
# Install everything (configs, scripts, skills, and rules)
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

`make` is required to install.

## Repository Structure

- `configs/` — Shell configs (bashrc/zshrc, shared aliases), git, gpg, starship, tmux, terraform, k9s, ghostty, herdr, litellm
- `local-bin/` — Custom utility scripts (symlinked to `~/.local/bin`)
- `skills/` — Agent skills in `SKILL.md` format (symlinked to `~/.claude/skills/` and `~/.agents/skills/`)
- `rules/` — Agent rule `.md` files (symlinked to `~/.claude/rules/`)
- `installers/` — Installation automation scripts
- `containers/` — Container images (e.g. `openclaw`)
- `dotfilesprivate/` — private/sensitive configs and scripts (separate git clone, not a submodule)

## How It Works

All installers create **symlinks** (not copies), so changes in this repo are immediately reflected in the home directory.

- **Scripts:** Symlinked from `local-bin/` to `~/.local/bin/`
- **Configs:** Symlinked to home directory with OS-specific handling (macOS uses zshrc, Linux uses bashrc)
- **Skills:** Symlinked from `skills/` to `~/.claude/skills/` (Claude Code) and `~/.agents/skills/` (the vendor-neutral path read by Codex, Gemini, opencode, and Copilot CLI)
- **Rules:** Symlinked from `rules/` to `~/.claude/rules/` (Claude Code's global rules path)

## GitHub Codespaces

GitHub Codespaces can install this repository as personal dotfiles. In your GitHub Codespaces settings, enable automatic dotfiles installation and select `surajssd/dotfiles`. Codespaces recognizes the root `install.sh`, which runs `make install-all`: config files, shell scripts, agent skills, and agent rules.

The development container must provide `make`. To rerun the setup in an existing codespace:

```bash
/workspaces/.codespaces/.persistedshare/dotfiles/install.sh
```

The installer uses private configs and scripts only when their optional repositories are already present. It does not clone private repositories automatically.

## Claude Code with LiteLLM

The declarative LiteLLM deployment lives in `configs/litellm/`. It exposes GitHub Copilot and W&B Inference through one local Anthropic-compatible gateway. The Compose service is intentionally stateless except for the persistent `litellm-copilot-data` volume that stores GitHub's OAuth credential.

```bash
litellm-proxy.sh start
litellm-proxy.sh status
litellm-proxy.sh models
litellm-proxy.sh test-copilot
litellm-proxy.sh test-wandb
litellm-proxy.sh claude
litellm-proxy.sh claude --dangerously-skip-permissions --allow-dangerously-skip-permissions
```

On the first `litellm-proxy.sh start`, follow the GitHub device-login URL and code printed by the script. The default Claude Code model is `claude-fable-5`; set `LITELLM_MODEL` to another model returned by `litellm-proxy.sh models`, such as `claude-sonnet-4-6` or `wandb/zai-org/GLM-5.2`. Every argument after the `claude` subcommand is passed directly to Claude Code.

Each secret is read from its environment variable first, then from the macOS Keychain entry whose service name equals the variable name: `LITELLM_MASTER_KEY` (proxy key), `LITELLM_SALT_KEY` (encrypts provider keys stored in the proxy DB; set it once and never change it), `WANDB_API_KEY` (W&B Inference), and `WANDB_QA_API_KEY` (`wandb-qa/` models on `api.qa.inference.wandb.ai`). To add or rotate a Keychain value:

```bash
security add-generic-password -U -a "$USER" -s LITELLM_MASTER_KEY -w '<LiteLLM proxy key>'
security add-generic-password -U -a "$USER" -s LITELLM_SALT_KEY -w '<LiteLLM salt key>'
security add-generic-password -U -a "$USER" -s WANDB_API_KEY -w '<W&B API key>'
security add-generic-password -U -a "$USER" -s WANDB_QA_API_KEY -w '<W&B QA API key>'
```

## License

MIT — see [LICENSE](LICENSE).
