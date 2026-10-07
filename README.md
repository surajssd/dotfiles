# dotfiles

Personal shell configurations, utility scripts, and installation automation for macOS and Linux. Configs and scripts are symlinked, so edits take effect without reinstalling.

[Setup](#quick-setup) · [Installation](#installation) · [Updates](#updates) · [Tools](#tools) · [Codespaces](#github-codespaces)

## Quick Setup

Install with `make`. Go is optional; when it is available, installation also builds `planner` and `asana`.

```bash
cd ~/code
git clone https://github.com/surajssd/dotfiles
cd dotfiles
make install-all
```

To include private dotfiles, run `make clone-private` before installation. The optional `dotfilesprivate/` directory is a separate, ignored Git clone.

## Installation

Use individual targets to install only what you need. Run `make help` for all targets.

| Command | Installs |
| --- | --- |
| `make install-configs` | [Shell and tool configs](configs/) into the home directory; zsh on macOS, bash on Linux. |
| `make install-local-bin` | [Utility scripts](local-bin/) into `~/.local/bin/`. |
| `make install-skills` | [Agent skills](skills/) into `~/.claude/skills/` and `~/.agents/skills/`. |
| `make install-rules` | [Agent rules](rules/) into `~/.claude/rules/`. |
| `make install-planner` | The [Planner command](planner/) into `$(go env GOPATH)/bin`. |
| `make install-asana` | The [Asana command](asana/) into `$(go env GOPATH)/bin`. |

Skills come from both `skills/` and, when present, `dotfilesprivate/skills/`. Private skills take precedence when names match. The `~/.agents/skills/` path is shared by Codex, Gemini, opencode, and Copilot CLI.

Planner and Asana are compiled Go commands. Their installers skip the build when Go is absent. Builds may download Go modules or a toolchain; a build failure stops `make install-all`. Rebuild them with their install targets after changing Go code.

Installation scripts live in [installers/](installers/). Container builds, including OpenClaw, live in [containers/](containers/) and are not installed by `make install-all`.

## Updates

```bash
make update
```

This pulls the public repository and the private clone when present, refreshes vendored skills and rules, then reinstalls everything. Use `make pull-master` to pull without reinstalling.

## Tools

- [Planner](planner/) manages Markdown plans and implementation status.
- [Asana](asana/) manages tasks and private project aliases from the terminal.

### Claude Code with LiteLLM

Run GitHub Copilot and W&B Inference through a local Anthropic-compatible gateway.

[LiteLLM setup and usage](docs/litellm.md)

## GitHub Codespaces

Enable automatic dotfiles installation in your GitHub Codespaces settings and select `surajssd/dotfiles`. The root `install.sh` runs `make install-all`. The development container must provide `make`; Go commands are built only when Go is available.

To rerun setup in an existing codespace:

```bash
/workspaces/.codespaces/.persistedshare/dotfiles/install.sh
```

The installer uses private configs and scripts only when their optional repositories are already present. It does not clone private repositories automatically.

## License

[MIT](LICENSE).
