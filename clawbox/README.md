# clawbox

A macOS CLI tool for managing [OpenClaw](https://github.com/anthropics/openclaw) gateway containers using Apple's native `container` CLI. Define sessions in a single YAML config file and manage the full container lifecycle — setup, start, stop, restart, remove — with persistent volumes, automatic health checks, and optional HTTP proxy support.

## Prerequisites

- **macOS** — clawbox only runs on Darwin (enforced at runtime)
- **Apple `container` CLI** — see [Using macOS Containerization](https://suraj.io/post/2026/using-osx-containerization/) for setup instructions
- **Go 1.25+** — required to build from source
- **tinyproxy** *(optional)* — only needed if you enable the HTTP proxy feature: `brew install tinyproxy`

Before running clawbox, make sure the container system is started:

```bash
container system start
```

## Installation

### Using `go install`

```bash
go install github.com/surajssd/dotfiles/clawbox@latest
```

This installs the `clawbox` binary to your `$GOPATH/bin` (or `$GOBIN`). Make sure that directory is in your `PATH`.

### From source

```bash
git clone https://github.com/surajssd/dotfiles.git
cd dotfiles/clawbox
make install
```

Or build a local binary without installing:

```bash
make build
./clawbox --help
```

## Getting Started

### 1. Create a sessions config

Create the config file at `~/.config/openclaw/sessions.yaml`. Here's a minimal example with one session:

```yaml
dev:
  ports:
    gateway: 18789
    bridge: 18790
```

Only `ports.gateway` and `ports.bridge` are required — everything else has sensible defaults.

Here's a full-featured example with two sessions:

```yaml
work:
  image: ghcr.io/surajssd/dotfiles/openclaw:latest
  resources:
    cpus: 4
    memory: 4g
  ports:
    gateway: 18789
    bridge: 18790
  proxy:
    enabled: true
    port: 8080
  env:
    MY_VAR: hello
  mounts:
    - source: /Users/me/projects
      target: /home/node/projects
      readonly: false
  skills:
    - /Users/me/.claude/skills

dev:
  ports:
    gateway: 19789
    bridge: 19790
```

### 2. Run initial setup

Run the one-time onboarding for your session. This creates volumes, initializes the state directory, runs OpenClaw onboarding, and auto-starts the container:

```bash
clawbox setup dev
```

### 3. Use your session

```bash
# Shell into the container
clawbox exec dev

# View connection info (dashboard URL, health endpoint)
clawbox info dev

# Tail container logs
clawbox logs dev

# Approve a device
clawbox exec dev openclaw devices approve

# Stop the session
clawbox stop dev

# Start it again later
clawbox start dev
```

## Usage

### Commands

| Command | Alias | Description |
|---|---|---|
| `clawbox setup <session>` | | Run initial onboarding for a new session |
| `clawbox start <session>` | | Start a session container |
| `clawbox stop <session>` | | Stop a session container |
| `clawbox restart <session>` | | Stop and start a session container |
| `clawbox remove <session>` | `rm` | Stop and remove a session container |
| `clawbox exec <session> [cmd...]` | `e` | Exec into a running container (defaults to `bash -l`) |
| `clawbox logs <session>` | | Follow container logs |
| `clawbox info <session>` | | Show dashboard URL, health endpoint, and usage hints |
| `clawbox config <session>` | | Print path to the session's `openclaw.json` |
| `clawbox status [session]` | | Show status of one session or all containers |
| `clawbox list` | `ls` | List all defined sessions with port and status |
| `clawbox proxy start <session>` | | Start the HTTP proxy for a session |
| `clawbox proxy stop <session>` | | Stop the HTTP proxy for a session |
| `clawbox proxy status <session>` | | Show proxy status for a session |
| `clawbox completion` | | Generate shell completion scripts |

### Shell Completion

Generate completion scripts for your shell:

```bash
# Bash
clawbox completion bash > /usr/local/etc/bash_completion.d/clawbox

# Zsh
clawbox completion zsh > "${fpath[1]}/_clawbox"

# Fish
clawbox completion fish > ~/.config/fish/completions/clawbox.fish
```

## Configuration Reference

The config file lives at `~/.config/openclaw/sessions.yaml`. It is a YAML map of session names to their configuration.

### Session Name Rules

Session names must start with a letter or digit and may only contain letters, digits, hyphens (`-`), and underscores (`_`).

### Fields

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `image` | string | No | `ghcr.io/surajssd/dotfiles/openclaw:latest` | Container image to use |
| `resources.cpus` | int | No | `2` | Number of CPUs |
| `resources.memory` | string | No | `2g` | Memory limit (e.g., `4g`) |
| `ports.gateway` | int | **Yes** | — | Host port mapped to container gateway (18789) |
| `ports.bridge` | int | **Yes** | — | Host port mapped to container bridge (18790) |
| `proxy.enabled` | bool | No | `false` | Enable tinyproxy HTTP proxy |
| `proxy.port` | int | No | `11080` | Proxy listen port |
| `mounts` | list | No | `[]` | Additional bind mounts |
| `mounts[].source` | string | Yes* | — | Host path |
| `mounts[].target` | string | Yes* | — | Container path |
| `mounts[].readonly` | bool | No | `false` | Mount as read-only |
| `env` | map | No | `{}` | Extra environment variables |
| `skills` | list | No | `[]` | Host directories whose subdirectories are mounted as skills |

### Data Paths

| Path | Purpose |
|---|---|
| `~/.config/openclaw/sessions.yaml` | Sessions config file |
| `~/.custom-openclaw-setup/<session>/` | Per-session state and config |
| `~/.custom-openclaw-setup/proxy/` | Proxy PID and config files |

Each session gets two 20 GB persistent volumes (home and linuxbrew) that survive container removal.

## Key Features

- **Multi-session support** — run multiple independent OpenClaw sessions simultaneously on different port pairs
- **Persistent volumes** — each session gets dedicated 20 GB volumes for `/home/node` and `/home/linuxbrew`
- **Automatic health checking** — polls the gateway health endpoint after startup, waiting up to 60 seconds
- **HTTP proxy injection** — when enabled, starts a tinyproxy on the vmnet gateway and injects `http_proxy`/`https_proxy` env vars into the container
- **Skills mounting** — mount host skill directories into the container for plugin/extension support
- **Token-aware dashboard URLs** — `clawbox info` reads the gateway auth token and prints a one-click dashboard URL with the token embedded
