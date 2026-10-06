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
# Install everything (configs, scripts, skills, rules, and Go commands)
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

# Build and install the asana Go command (skipped when go is absent)
make install-asana

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
- `asana/` - Go module for the `asana` task-creation command (built with `go install`)
- `installers/` — Installation automation scripts
- `containers/` — Container images (e.g. `openclaw`)
- `dotfilesprivate/` — private/sensitive configs and scripts (separate git clone, not a submodule)

## How It Works

Installers create **symlinks** (not copies), so changes in this repo are immediately reflected in the home directory. The exceptions are `planner/` and `asana/`, Go commands that `go install` compiles into `$(go env GOPATH)/bin`.

- **Scripts:** Symlinked from `local-bin/` to `~/.local/bin/`
- **Configs:** Symlinked to home directory with OS-specific handling (macOS uses zshrc, Linux uses bashrc)
- **Skills:** Symlinked from `skills/` to `~/.claude/skills/` (Claude Code) and `~/.agents/skills/` (the vendor-neutral path read by Codex, Gemini, opencode, and Copilot CLI)
- **Rules:** Symlinked from `rules/` to `~/.claude/rules/` (Claude Code's global rules path)
- **Planner:** Built from `planner/` with `go -C planner install .` when `go` is on `PATH`; otherwise the installer prints a note and skips. The build may download Go modules or a Go toolchain, and a build failure fails `make install-all`.
- **Asana:** Built from `asana/` with `go -C asana install .` under the same conditions as Planner. Run `make install-asana` after changing the Go code.

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
planner describe --github <plan>         # the same, with the state gh reports after each pull request URL
planner get --all -o wide                # every plan, legacy ones included, with TYPE, PATH, and NOTE columns
planner tree --repo cks                  # plans whose <org>/<repo> contains "cks", plus ancestors
planner get repos                        # every <org>/<repo> that has plans, one per line
planner get --status implemented         # plans with that ImplementationStatus (any case, hyphens optional), no --all needed
planner check                            # integrity findings; exit 1 on errors
planner check <plan>                     # the findings of one plan only
planner check --github [<plan>...]       # also warn when StatusNote calls a closed or merged pull request open
planner get <url>                        # every plan that lists the URL as issue, pull request, parent, or successor
planner create [--parent <ref>] [--status <status> --note <text>] [--issue <url>]... [--pr <url>]... <name...>   # create a plan for the current repository
planner log <plan> <title...> [--note <text>] [--keep-checked] [--issue <url>]... [--pr <url>]... < entry.md   # append a dated entry; merge links and the note in the same write
planner set status <plan> [<status>] [--note <text> | --note-file <path>] [--superseded-by <ref>]   # change status, checked date, note, and successor in place; --note-file - reads stdin
planner set parent <plan> <ref>       # set the parent of a plan: a dumped plan, a file path, or a URL
planner set pr <plan> <url>...        # add pull request URLs to a plan
planner set issue <plan> <url>...     # add tracker URLs to a plan
planner version                          # build information of the installed binary
source <(planner completion zsh)         # completion of commands, flags, plan names, repositories, and statuses
```

`planner log` sets `StatusChecked` to today by default. Use `--keep-checked` for administrative entries, such as recording a ticket, that do not verify implementation status. The repeatable `--issue` and `--pr` flags add links in the same write and skip duplicates. Redirect a file into stdin with `< entry.md` to supply the entry body. `planner check` omits duplicate-title advisories for plans explicitly replaced through `SupersededBy` by another plan with the same title in the same repository folder.

The plan root comes from `~/.planner.yaml`:

```yaml
root: ~/plans
```

`--root <dir>` overrides the file. `get` and `tree` print the same `kubectl`-style table (`NAME`, `REPO`, `STATUS`, `CHECKED`, `TITLE`) and share `--all`, `-o`, `--no-headers`, and `--repo`; `tree` adds connectors in `NAME`. `CHECKED` is the number of days since `StatusChecked`, with `!` after an active plan older than a week. Both accept plan names (a path, a wikilink, a basename, or the short `NAME` from the table); a name that matches nothing is reported after the rows that were found and the exit status is 1. `describe` prints one plan's fields as `Key: Value` lines. `-o wide` adds `TYPE`, `PATH` (home shown as `~`), and `NOTE`, and never truncates; `-o json` and `-o yaml` print every field of each plan (one named plan as a single object, otherwise a list under `items`); `-o name` prints basenames. `tree` supports only `-o wide`. `--repo` is a case-insensitive substring match. `get repos` prints every `<org>/<repo>` that has plans, one per line, whatever their status. Without `--all` only active plans are listed and a note on stderr counts the plans without valid front matter; `--all` lists every plan, those with `-` in STATUS and CHECKED. `planner check --help` lists the front matter keys, the recognised status values, and every rule with its severity; the link rules require `http(s)` URLs, a pull request path for `github.com` entries under `PullRequests`, a `SupersededBy` on every `Superseded` plan, and no keys outside the schema. `zshrc` and `bashrc` source `planner completion` when the binary is on `PATH`, which completes plan names, repositories, statuses, and output formats.

## Asana

`asana add task` creates one task in one project, assigns it to the authenticated user, and prints the task URL. It accepts one nonblank title and an optional plain-text description. The due date defaults to today in local time.

`asana get task` lists incomplete tasks assigned to the authenticated user across every accessible workspace. By default, it includes overdue tasks and tasks due today in local time, and excludes tasks without a due date.

1. Run `make install-asana` from this repository with Go 1.25 or later on `PATH`. The command installs `asana` into `$(go env GOPATH)/bin`; add that directory to `PATH` if needed. `make install-all` also includes this build. When Go is absent, the installer prints a message and skips the build.
2. Create a personal access token in the [Asana developer console](https://app.asana.com/0/my-apps), following [Asana's token setup guide](https://developers.asana.com/docs/personal-access-token).
3. On macOS, store the token once in Keychain with the command below. Paste the token at the password prompt. The token stays out of shell history.

   ```bash
   security add-generic-password -a "$USER" -s ASANA_ACCESS_TOKEN -w
   ```

4. Save the following configuration as `~/.asana.yaml`. Replace the example URLs with your project URLs. Open each project in Asana and copy its URL from the browser address bar.

   ```yaml
   token_command: [security, find-generic-password, -s, ASANA_ACCESS_TOKEN, -w]
   default_project: work
   projects:
     work: https://app.asana.com/1/123/project/456
     personal: https://app.asana.com/1/123/project/789/list
   ```

The CLI uses a nonempty `ASANA_ACCESS_TOKEN` first. Otherwise, it runs the optional `token_command` list directly, without a shell, and trims trailing whitespace from stdout. The command must return a nonempty token within 30 seconds. On Linux or another system, set the environment variable through your secret manager or replace `token_command` with a command that prints the token. Keep the token out of the YAML file.

`projects` may be omitted or empty. To start without aliases, omit `default_project` and use `projects: {}`. Each value must be a URL of the form `https://app.asana.com/1/<workspace>/project/<gid>`, optionally ending in `/list` or `/board`, or an older `https://app.asana.com/0/<gid>/...` URL. Query strings are accepted. Bare GIDs are rejected. Unknown configuration keys, invalid project URLs, and a `default_project` that is not an alias are errors.

```bash
asana add task "Review the proposal"
asana add task "Fix the build" -p work -d "Investigate the failing release job."
asana add task "Write the runbook" -d - < notes.md
asana add task "Pay the invoice" --due tomorrow
asana add task "File it there" -p https://app.asana.com/1/123/project/456/list
asana --config ./asana.yaml add task "Review the proposal" --due 2026-10-31
```

For `add task`, `--project/-p` selects a configured alias before trying a project URL. Without the flag, `add task` uses `default_project`; if no default is set, pass `--project`. `--description/-d -` reads plain text from stdin. `--due` accepts `YYYY-MM-DD`, `today`, or `tomorrow` and defaults to `today`; the keywords use the local calendar date. `--config` overrides `~/.asana.yaml` for all commands and configuration writes.

`add task` sends one [task-creation request](https://developers.asana.com/reference/createtask). Success prints only the permalink URL to stdout and exits 0. After a transport failure or an unreadable success response from `add task`, check Asana before retrying because the task may already exist.

```bash
asana get task
asana get task --due tomorrow
asana get task --due any -o json
asana get task --due none
asana get task -p work -p personal -o wide --no-headers
```

`get task` accepts no positional arguments. Its flags are:

| Flag | Behavior |
|---|---|
| `--due` | Defaults to `today`. `YYYY-MM-DD`, `yesterday`, `today`, and `tomorrow` select tasks due on or before that local date. `any` includes every date and undated tasks. `none` selects only undated tasks. |
| `--project/-p` | Selects a configured alias or project URL. Repeat the flag to include tasks in any selected project. Without this flag, `get task` includes all projects and ignores `default_project`. |
| `--output/-o` | Selects `table` (default), `wide`, or `json`. |
| `--no-headers` | Omits table headers. JSON ignores this flag. |

Table columns are `GID PROJECT DUE NAME`; `wide` adds `URL`. Project names are comma-separated. A missing project or due date appears as `-`. Names stay on one line in tables. Results sort by due date, with undated tasks last, then by name and GID. When `due_at` is present, its local calendar date takes precedence over `due_on` for filtering, sorting, and display. JSON preserves the original text and date fields, with an `items` array of tasks containing `gid`, `name`, `due_on`, `due_at`, `permalink_url`, and `projects` (each project contains `gid` and `name`). An empty table prints `No tasks found.` to stderr and nothing to stdout. Empty JSON prints `{"items":[]}`. Both exit 0.

`get task` [discovers the user's workspaces](https://developers.asana.com/reference/getuser) and reads each workspace's [paginated task list](https://developers.asana.com/reference/gettasks) in sequence. It uses `assignee=me` and `completed_since=now`, then applies date and project filters locally. It prints results only after all requests succeed. Completed tasks, task editing, and an `--all` flag are not supported.

`asana add project` creates a private project and saves its URL under a required alias. The name must be one nonblank argument, and the alias must be nonblank and unused. Project creation has no description flag. It sends `privacy_setting: private`, without a team or membership changes, and requires the response to confirm private visibility and provide a valid project URL. See the [project-creation API](https://developers.asana.com/reference/createproject) and [teamless-project update](https://forum.asana.com/t/change-teamless-projects/929205).

```bash
asana add project "Research" --alias research
asana add task "Read the proposal" -p research
asana add project "Release planning" --alias releases --workspace 123
asana get project
asana get project --refresh
asana get project --refresh --workspace 123 -o wide
asana get project -o json
```

`asana get project` reads only the configuration file. It does not load credentials or call Asana. `--refresh` fetches unarchived projects and adds those where the authenticated user is a member. Each missing project uses its name verbatim as the alias. Existing aliases and URLs remain unchanged, including aliases for projects absent from the response. Project GIDs identify existing entries even when their URL format or project name differs. If a missing project's name conflicts with an alias for another project, refresh fails without saving any additions or printing a partial list.

`add project` and `get project --refresh` discover the user's workspaces. They use the only workspace automatically. When several workspaces are available, the command lists their GIDs and requires `--workspace <gid>` to select one. `get project --workspace` requires `--refresh`. There is no `default_workspace` configuration key.

Project listing has the same output after a refresh as a local read. Rows sort by alias. The default table columns are `ALIAS GID`; `-o wide` adds `URL`. `--no-headers` omits table headers. `-o json` prints an `items` array containing `alias`, `gid`, and `url`, preserving original text. Empty tables print `No projects configured.` to stderr; empty JSON prints `{"items":[]}`. Both exit 0. Successful nonempty listings write nothing to stderr.

Both project commands that save aliases leave `default_project` unchanged. They update YAML nodes to preserve comments, key order, and existing values. They follow configuration symlinks, preserve the target's permissions, and prepare a temporary file beside the target before any API request. After writing and syncing the temporary file, they atomically replace the target. There is no concurrent-edit check, so avoid editing the same configuration while creation or refresh runs. Refresh leaves the file untouched when it finds no missing projects.

Project creation prints only the URL after the alias is saved. If saving fails after creation, the error includes the created URL and unsaved alias. Register that URL manually instead of repeating creation. After a transport error or an unreadable creation response, check Asana before retrying because the project may already exist. Public-project creation, team sharing, and project editing or deletion are not supported.

Bare `asana add` and `asana get` show help without configuration or credentials. Task commands now require the `task` subcommand; the old task shortcuts are removed.

All commands validate input and configuration before loading credentials. HTTP requests have a 30-second timeout, with no retries or redirects. SIGINT and SIGTERM cancel the command while it waits for stdin, credentials, or an HTTP response. Failures print `error:` and a message to stderr and exit 1. Authentication, permission, missing-resource, and rate-limit errors have specific messages; rate limits include `Retry-After` when Asana supplies it. Help for every command works without configuration or credentials.

From `asana/`, run `gofmt -w *.go`, `go vet ./...`, `golangci-lint run`, and `go test ./...`. Also run `go test -race ./...`. Tests use a local HTTP server and fake credentials; they do not create real Asana resources.

## GitHub Codespaces

GitHub Codespaces can install this repository as personal dotfiles. In your GitHub Codespaces settings, enable automatic dotfiles installation and select `surajssd/dotfiles`. Codespaces recognizes the root `install.sh`, which runs `make install-all`: config files, shell scripts, agent skills, agent rules, and the `planner` and `asana` commands when the container provides Go.

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
