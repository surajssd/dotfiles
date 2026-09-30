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

# Build and install the planner Go command (skipped when go is absent)
make install-planner

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
- `planner/` — Go module for the `planner` command (built with `go install`)
- `installers/` — Installation automation scripts
- `containers/` — Container images (e.g. `openclaw`)
- `dotfilesprivate/` — private/sensitive configs and scripts (separate git clone, not a submodule)

## How It Works

Installers create **symlinks** (not copies), so changes in this repo are immediately reflected in the home directory. The one exception is `planner/`, a Go command that `go install` compiles into `$(go env GOPATH)/bin`.

- **Scripts:** Symlinked from `local-bin/` to `~/.local/bin/`
- **Configs:** Symlinked to home directory with OS-specific handling (macOS uses zshrc, Linux uses bashrc)
- **Skills:** Symlinked from `skills/` to `~/.claude/skills/` (Claude Code) and `~/.agents/skills/` (the vendor-neutral path read by Codex, Gemini, opencode, and Copilot CLI)
- **Rules:** Symlinked from `rules/` to `~/.claude/rules/` (Claude Code's global rules path)
- **Planner:** Built from `planner/` with `go -C planner install .` when `go` is on `PATH`; otherwise the installer prints a note and skips. The build may download Go modules or a Go toolchain, and a build failure fails `make install-all`.

## Planner

`planner` keeps a folder of Markdown plans with YAML front matter. Each plan is `<root>/github.com/<org>/<repo>/<YYMMDDHHMMSS>-<name>.md` and starts with a block holding `type`, `implementation_status`, `status_checked`, `status_note`, and an optional `parent`.

```bash
planner get                              # table of active plans
planner get <plan>                       # one plan's row, whatever its status
planner tree                             # the same plans as an effort tree
planner tree <plan>                      # the subtree under one plan
planner describe <plan>                  # every field, children, note, and findings of one plan
planner get --all --wide                 # every plan, legacy ones included, with NOTE and PATH columns
planner tree --repo cks                  # plans whose <org>/<repo> contains "cks", plus ancestors
planner check                            # integrity findings; exit 1 on errors
planner new [--parent <ref>] [--status <status> --note <text>] <name...>   # create a plan for the current repository
planner update status <plan> [<status>] [--note <text>]   # change status, checked date, and note in place
```

The plan root comes from `~/.planner.yaml`:

```yaml
root: ~/plans
```

`--root <dir>` overrides the file. `get` and `tree` print the same `kubectl`-style table (`NAME`, `REPO`, `STATUS`, `AGE`, `TITLE`) and share `--all`, `--wide`, and `--repo`; `tree` adds connectors in `NAME`. Both accept plan names (a path, a wikilink, a basename, or the short `NAME` from the table), and `describe` prints one plan's fields as `Key: Value` lines. `--wide` drops `REPO`, adds `NOTE` and `PATH` (home shown as `~`), and never truncates. `--repo` is a case-insensitive substring match. Without `--all` only active plans are listed and a note on stderr counts the plans without valid front matter; `--all` lists every plan, those with `-` in STATUS and AGE. `planner check --help` lists the recognised status values and every rule with its severity.

## GitHub Codespaces

GitHub Codespaces can install this repository as personal dotfiles. In your GitHub Codespaces settings, enable automatic dotfiles installation and select `surajssd/dotfiles`. Codespaces recognizes the root `install.sh`, which runs `make install-all`: config files, shell scripts, agent skills, agent rules, and the `planner` command when the container provides Go.

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

On the first `litellm-proxy.sh start`, follow the GitHub device-login URL and code printed by the script. The default Claude Code model is `claude-fable-5-1`; set `LITELLM_MODEL` to another model returned by `litellm-proxy.sh models`, such as `claude-sonnet-4-6` or `wandb/zai-org/GLM-5.2`. Every argument after the `claude` subcommand is passed directly to Claude Code.

Each secret is read from its environment variable first, then from the macOS Keychain entry whose service name equals the variable name: `LITELLM_MASTER_KEY` (proxy key), `LITELLM_SALT_KEY` (encrypts provider keys stored in the proxy DB; set it once and never change it), `WANDB_API_KEY` (W&B Inference), `WANDB_QA_API_KEY` (`wandb-qa/` models on `api.qa.inference.wandb.ai`), `WANDB_OPENAI_PROJECT` (production project), and `WANDB_QA_OPENAI_PROJECT` (QA project). Only `LITELLM_MASTER_KEY` and `LITELLM_SALT_KEY` are required to start the proxy. The W&B settings are optional at startup; requests to W&B models need the corresponding API key and project value. The project values supply the upstream `OpenAI-Project` header and stay outside the repository. To add or rotate a Keychain value:

```bash
security add-generic-password -U -a "$USER" -s LITELLM_MASTER_KEY -w '<LiteLLM proxy key>'
security add-generic-password -U -a "$USER" -s LITELLM_SALT_KEY -w '<LiteLLM salt key>'
security add-generic-password -U -a "$USER" -s WANDB_API_KEY -w '<W&B API key>'
security add-generic-password -U -a "$USER" -s WANDB_QA_API_KEY -w '<W&B QA API key>'
security add-generic-password -U -a "$USER" -s WANDB_OPENAI_PROJECT -w '<W&B entity/project>'
security add-generic-password -U -a "$USER" -s WANDB_QA_OPENAI_PROJECT -w '<W&B QA entity/project>'
```

## License

MIT — see [LICENSE](LICENSE).
