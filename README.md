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

`planner` keeps a folder of Markdown plans with YAML front matter. Each plan is `<root>/github.com/<org>/<repo>/<YYMMDDHHMMSS>-<name>.md` and starts with a block holding `Type`, `ImplementationStatus`, `StatusChecked`, and `StatusNote`, plus the optional `Parent` and `SupersededBy` (a `[[wikilink]]`, a path, or a URL) and the optional lists `Issues` (GitHub issue, Jira, Asana, or any other tracker URL) and `PullRequests` (pull request URLs).

Front matter keys are case-sensitive and use CamelCase, including in piped input. The `-o json` and `-o yaml` formats use the same spelling for these fields and add the lowercase keys `name`, `basename`, `path`, `repo`, `title`, and, for a plan whose block is missing or broken, `front_matter_error`; `planner get --help` lists them.

```bash
planner get                              # table of active plans
planner get <plan>                       # one plan's row, whatever its status
planner get -o json <plan>               # every field of one plan as JSON, path included
planner tree                             # the same plans as an effort tree
planner tree <plan>                      # the subtree under one plan
planner describe <plan>                  # every field, children, note, and findings of one plan
planner get --all -o wide                # every plan, legacy ones included, with TYPE, PATH, and NOTE columns
planner tree --repo cks                  # plans whose <org>/<repo> contains "cks", plus ancestors
planner get repos                        # every <org>/<repo> that has plans, one per line
planner get --status implemented         # plans with that ImplementationStatus (any case, hyphens optional), no --all needed
planner check                            # integrity findings; exit 1 on errors
planner get <url>                        # every plan that lists the URL as issue, pull request, parent, or successor
planner create [--parent <ref>] [--status <status> --note <text>] [--issue <url>]... [--pr <url>]... <name...>   # create a plan for the current repository
planner set status <plan> [<status>] [--note <text>] [--superseded-by <ref>]   # change status, checked date, note, and successor in place
planner set parent <plan> <ref>       # set the parent of a plan: a dumped plan, a file path, or a URL
planner set pr <plan> <url>...        # add pull request URLs to a plan
planner set issue <plan> <url>...     # add tracker URLs to a plan
planner version                          # build information of the installed binary
source <(planner completion zsh)         # completion of commands, flags, plan names, repositories, and statuses
```

The plan root comes from `~/.planner.yaml`:

```yaml
root: ~/plans
```

`--root <dir>` overrides the file. `get` and `tree` print the same `kubectl`-style table (`NAME`, `REPO`, `STATUS`, `CHECKED`, `TITLE`) and share `--all`, `-o`, `--no-headers`, and `--repo`; `tree` adds connectors in `NAME`. `CHECKED` is the number of days since `StatusChecked`, with `!` after an active plan older than a week. Both accept plan names (a path, a wikilink, a basename, or the short `NAME` from the table); a name that matches nothing is reported after the rows that were found and the exit status is 1. `describe` prints one plan's fields as `Key: Value` lines. `-o wide` adds `TYPE`, `PATH` (home shown as `~`), and `NOTE`, and never truncates; `-o json` and `-o yaml` print every field of each plan (one named plan as a single object, otherwise a list under `items`); `-o name` prints basenames. `tree` supports only `-o wide`. `--repo` is a case-insensitive substring match. `get repos` prints every `<org>/<repo>` that has plans, one per line, whatever their status. Without `--all` only active plans are listed and a note on stderr counts the plans without valid front matter; `--all` lists every plan, those with `-` in STATUS and CHECKED. `planner check --help` lists the front matter keys, the recognised status values, and every rule with its severity; the link rules require `http(s)` URLs, a pull request path for `github.com` entries under `PullRequests`, a `SupersededBy` on every `Superseded` plan, and no keys outside the schema. `zshrc` and `bashrc` source `planner completion` when the binary is on `PATH`, which completes plan names, repositories, statuses, and output formats.

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
