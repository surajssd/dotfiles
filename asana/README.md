# Asana

[Back to the README](../README.md#tools)

Manage assigned tasks and private project aliases from the terminal.

## Setup

1. Run `make install-asana` from the repository root with Go 1.25 or later on `PATH`. Add `$(go env GOPATH)/bin` to `PATH` if needed.
2. Create a token in the [Asana developer console](https://app.asana.com/0/my-apps). Follow [Asana's token setup guide](https://developers.asana.com/docs/personal-access-token).
3. On macOS, store the token in Keychain. Paste it at the password prompt to keep it out of shell history.

   ```bash
   security add-generic-password -a "$USER" -s ASANA_ACCESS_TOKEN -w
   ```

4. Save this configuration as `~/.asana.yaml`. Replace the example URLs with URLs copied from your projects in the browser.

   ```yaml
   token_command: [security, find-generic-password, -s, ASANA_ACCESS_TOKEN, -w]
   default_project: work
   projects:
     work: https://app.asana.com/1/123/project/456
     personal: https://app.asana.com/1/123/project/789/list
   ```

## Configuration

Use `--config <path>` to override `~/.asana.yaml` for all commands and configuration writes.

### Credentials

A nonempty `ASANA_ACCESS_TOKEN` takes precedence over `token_command`. Otherwise, the command must print a nonempty token within 30 seconds. It runs without a shell; trailing whitespace is trimmed.

On other systems, use your secret manager to supply the environment variable or configure a command that prints the token. Keep tokens out of YAML.

### Project aliases and URLs

`projects` maps aliases to project URLs. `default_project` must name an existing alias. To start without aliases, use `projects: {}` and omit `default_project`.

Accept project URLs in either form:

- `https://app.asana.com/1/<workspace>/project/<gid>`, optionally ending in `/list` or `/board`.
- `https://app.asana.com/0/<gid>/...`.

Query strings are accepted. Bare GIDs, invalid URLs, unknown keys, and a default that is not an alias are rejected.

## Tasks

### Add a task

Pass exactly one nonblank title. Tasks are assigned to you and due today in local time by default. Success prints only the task URL.

```bash
asana add task "Review the proposal"
asana add task "Fix the build" -p work -d "Investigate the failing release job."
asana add task "Write the runbook" -d - < notes.md
asana add task "Pay the invoice" --due tomorrow
```

| Flag | Purpose |
| --- | --- |
| `--project/-p` | Selects an alias or project URL. Required when no default is configured. Aliases take precedence. |
| `--description/-d` | Supplies plain text. Use `-` to read stdin. |
| `--due` | Accepts `YYYY-MM-DD`, `today`, or `tomorrow`. Defaults to `today`. |

### List tasks

`get task` lists incomplete tasks assigned to you across all accessible workspaces. By default, it includes overdue tasks and tasks due today, and excludes undated tasks.

```bash
asana get task
asana get task --due any -o json
asana get task -p work -p personal -o wide --no-headers
```

### Filter tasks

| Flag | Selection |
| --- | --- |
| `--due <date>` | Tasks due on or before a local date. Accepts `YYYY-MM-DD`, `yesterday`, `today`, or `tomorrow`. |
| `--due any` | All dates, including undated tasks. |
| `--due none` | Only undated tasks. |
| `--project/-p` | Matches any selected alias or project URL. Repeat for multiple projects. Omit for all projects; `default_project` does not apply. |

Results sort by due date, undated last, then name and GID. The local date of `due_at` takes precedence over `due_on`. No results appear until all requests succeed.

## Projects

### Create a private project

Pass one nonblank name and a required, unused, nonblank alias. The command creates a private project, saves its URL under the alias, then prints the URL.

```bash
asana add project "Research" --alias research
asana add task "Read the proposal" -p research
```

### List and refresh aliases

`get project` reads configured aliases without credentials or network access.

```bash
asana get project
asana get project --refresh
asana get project --refresh --workspace 123 -o wide
```

Use `--refresh` to add unarchived projects where you are a member. Each new alias uses the project name verbatim.

- Existing aliases and URLs stay unchanged, including projects absent from the response.
- Projects are matched by GID, regardless of name or URL format.
- Alias collisions fail before any write or partial output.

### Select a workspace

Creation and refresh use the only workspace automatically. When several exist, pass `--workspace <gid>`; the error lists available GIDs. For `get project`, this flag requires `--refresh`. There is no `default_workspace` setting.

### Save aliases

Writes preserve YAML comments, key order, existing values, symlinks, and permissions. They leave `default_project` unchanged. Refresh does not rewrite the file when no aliases are added.

Avoid concurrent configuration edits because the commands replace the file atomically and can overwrite them.

## Output

| Format | Tasks | Projects |
| --- | --- | --- |
| `-o table` (default) | `GID PROJECT DUE NAME` | `ALIAS GID` |
| `-o wide` | Adds `URL`. | Adds `URL`. |
| `-o json` | An `items` array of tasks. | An `items` array of aliases. |

Use `--no-headers` to omit table headers. JSON ignores this flag and preserves original text and date fields. Project rows sort by alias.

Empty tables print `No tasks found.` or `No projects configured.` to stderr. Empty JSON prints `{"items":[]}`. Both exit 0.

## Errors and recovery

After a transport failure or unreadable creation response, check Asana before retrying because the task or project may already exist.

If saving an alias fails after project creation, register the reported URL and alias manually. Do not repeat creation.

Errors print `error:` to stderr and exit 1. Requests time out after 30 seconds, with no retries or redirects. SIGINT and SIGTERM cancel pending work. Rate-limit errors include `Retry-After` when supplied.

## Help and development

Run any command with `--help`. Bare `asana add` and `asana get` also show help. Help requires no configuration or credentials; task operations require the `task` subcommand.

Rebuild with `make install-asana` after changing Go code. See [the development checks](../CLAUDE.md#asana) for formatting, linting, and tests.
