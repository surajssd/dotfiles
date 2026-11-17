# Planner

[Back to the README](../../README.md#tools)

Manage Markdown plans by repository, track implementation status, and check metadata and links.

## Setup

Run `make install-planner` from the repository root. Rebuild after changing Go code.

Set the plan root in `~/.planner.yaml`:

```yaml
root: ~/plans
```

Use `--root <dir>` to override it.

## Plan format

Plans use `plan.md` inside a managed directory or a standalone file at `<root>/github.com/<org>/<repo>/<YYMMDDHHMMSS>-<name>.md`. Front matter keys are case-sensitive.

| Fields | Purpose |
| --- | --- |
| `Type`, `ImplementationStatus`, `StatusChecked`, `StatusNote` | Record the plan type, status, last check date, and summary. |
| `Parent`, `SupersededBy` | Link related plans using a wikilink, path, or URL. |
| `Issues`, `PullRequests` | Store lists of tracker and pull request URLs. |

Use `Abandoned` for a plan that will not be implemented. `Implemented`, `Superseded`, and `Abandoned` are inactive statuses.

### Directory plans

A directory with a non-symlinked `plan.md` that declares `ID` is one plan. Planner excludes the directory's other files and descendants from discovery. A legacy `plan.md` without `ID` remains a single-file plan.

Use an ID of twelve digits, a hyphen, and a name without whitespace, path separators, `#`, `|`, brackets, or a trailing `.md`. For example, `ID: 261116150000-add-frame-tests`. The ID stays fixed when the directory moves. Planner reports invalid and duplicate IDs through `check`.

Directory plans accept ID, short name, wikilink, directory path, metadata path, and linked URL references. Parent and successor links use ID-based wikilinks. Metadata commands edit only `plan.md`. `open` opens the directory. JSON and YAML add `ID` and `directory`, while `basename` remains `plan` and `path` points to `plan.md`. `describe` also shows the ID and directory. The repository comes from `<root>/github.com/<org>/<repo>`.

### External OpenSpec stores

Keep the store at `<root>/github.com/<org>/<repo>`, with OpenSpec files in its `openspec/` subtree. Planner reads only `changes/<change>/plan.md` and `changes/archive/<archived-change>/plan.md` within that subtree. Changes without `plan.md`, proposals, designs, tasks, specifications, and templates are excluded, including from `get --all` and `check`. Tracking metadata with a missing or invalid ID remains visible to `check`.

OpenSpec owns its artifacts. Planner owns the tracking metadata and progress log in `plan.md`. Both active and archived changes belong to `<org>/<repo>`. Moving a change into the archive preserves ID-based references. The move does not change implementation status, and old filesystem paths do not redirect.

## Browse plans

| Command | Result |
| --- | --- |
| `planner get [<plan>...]` | Lists active plans, or the named plans regardless of status. |
| `planner tree [<plan>...]` | Shows an effort tree, or a named plan's subtree. |
| `planner describe [--github] <plan>...` | Shows fields, children, notes, and findings. `--github` adds pull request states from `gh`. |
| `planner get repos` | Lists every repository with plans. |

A plan reference can be a path, wikilink, basename, short name from the table, or listed URL. URLs select every matching plan. Unmatched references exit 1 after found results.

### Open a plan in VS Code

Run `planner open <plan>` to open a plan file in VS Code. For example, `planner open improve-performance` opens the plan with that short name. Tab completion works for plan names.

The command accepts the same plan references as `get`, but the reference must match exactly one plan. Inactive plans and plans with missing or invalid front matter can be opened too. The command uses `code` on `PATH` and returns without waiting for the file to be closed. If `code` is missing, install the VS Code command-line launcher.

### Filters and output

| Option | Behavior |
| --- | --- |
| `--all` | Includes inactive plans and plans with missing or invalid front matter. |
| `--repo <text>` | Matches repository names by case-insensitive substring. `--repo '!=<text>'` excludes those repositories instead and keeps plans without a repository. Spaces after `!=` are ignored. |
| `--status <status>` | Matches any case, with or without hyphens. Includes matching inactive plans without `--all`. |
| `--pr <url>` | For `get`, selects plans whose `PullRequests` list contains the exact URL, including inactive plans without `--all`. Uses local metadata. |
| `-o wide` | Adds `TYPE`, `PATH`, and `NOTE` to `NAME REPO STATUS CHECKED TITLE`. Paths abbreviate home as `~`. |
| `-o json` / `-o yaml` | Prints all fields. One named plan is an object; other selections use an `items` list. |
| `-o name` | Prints effective identities: directory IDs or single-file basenames. |
| `--no-headers` | Omits table headers. |

For example, `planner get --repo '!=acme/widgets'` hides plans in `acme/widgets` and in any repository whose name contains that text, such as `acme/widgets-tools`. `planner tree` still shows an excluded parent when it is needed to place an included plan.

Run `planner get --pr https://github.com/acme/widgets/pull/1` to find every plan that lists that pull request. Add `--repo` or `--status` to narrow the results. Unlike a positional URL, `--pr` searches only `PullRequests`. It also filters named plans.

`tree` supports only `-o wide`. `CHECKED` shows days since the last check; `!` marks active plans older than a week. Invalid front matter shows `-` in status and checked columns.

## Create and update plans

| Command | Action |
| --- | --- |
| `planner create <name...>` | Creates an OpenSpec change for the current repository, adds `plan.md`, and prints its path. |
| `planner create --no-openspec <name...>` | Creates a standalone Markdown plan without OpenSpec. |
| `planner log <plan> <title...> < entry.md` | Appends a dated progress entry from a file. |
| `planner set status <plan> <status> --note "<text>"` | Updates status, checked date, and summary. |
| `planner set parent <plan> <ref>` | Sets the parent plan. |
| `planner set pr <plan> <url>...` | Adds pull request URLs. |
| `planner set issue <plan> <url>...` | Adds tracker URLs. |

- `create` accepts `--parent` and an initial `--status` with `--note`. Both `create` and `log` accept repeatable `--issue` and `--pr` flags; duplicate URLs are skipped.
- `log` updates `StatusChecked` to today. Use `--keep-checked` for administrative entries and `--note` to replace the summary in the same write.
- `set status` accepts `--note-file <path>` or `--note-file -` for stdin. Use `--superseded-by <ref>` when setting the status to `Superseded`.

To abandon a plan, run `planner set status <plan> Abandoned --note "Will not be implemented."`. The plan stays available by name, with `--all`, or with `--status Abandoned`.

### Create an OpenSpec change

`planner create add-frame-tests` invokes `openspec new change add-frame-tests --json` from `<root>/github.com/<org>/<repo>`, then writes `plan.md` in the new change directory. The OpenSpec CLI must be on `PATH`. Planner creates the external repository directory when needed and runs `openspec init --tools none --no-animation` if the OpenSpec tree is missing or incomplete. Initialization creates the external OpenSpec structure without agent integrations or store registration. Existing configuration is preserved. Planner verifies the root with `openspec list --json` before creating the change. Each CLI call times out after 30 seconds.

Names use lowercase letters, numbers, and single hyphens. Multiple name arguments are joined with hyphens. OpenSpec creates the change folder and `.openspec.yaml`; its workflow supplies proposal, design, specification, and task content later. Planner keeps piped Markdown in `plan.md`.

When OpenSpec fails or returns an unreadable result, Planner reports that the change may exist and does not retry or delete OpenSpec files. Inspect the reported directory before retrying. Existing change directories are refused. Use `--no-openspec` to create a standalone timestamped Markdown file, including when OpenSpec is unavailable.

Run the default workflow from the code repository:

```sh
planner create add-frame-tests
planner get add-frame-tests
planner set status add-frame-tests InProgress --note "Implementation has started."
```

## Validate plans

Run `planner check [<plan>...]` to find metadata and link problems. Add `--github` to warn when a note calls a closed or merged pull request open. Error findings exit 1.

Run `planner check --help` for schema keys, status values, and validation rules. Duplicate-title warnings exclude plans explicitly superseded by another plan with the same title in the same repository.

## Help and completion

Use `--help` on any command for all flags and output fields. The shell configs load `planner completion` when the binary is available; it completes commands, flags, plans, repositories, statuses, and formats.

## Development

`main.go` starts the command. The implementation uses these internal packages:

| Package | Responsibility |
| --- | --- |
| `internal/cli` | Cobra commands, configuration, completion, output, and coordination of plan and external operations. |
| `internal/plan` | Plan discovery, identity, references, local validation, content rendering, and file editing. |
| `internal/github` | GitHub repository resolution and pull request state queries through Git and `gh`. |
| `internal/openspec` | OpenSpec initialization, root verification, and change creation. |

The plan, GitHub, and OpenSpec packages do not import the CLI or each other. Command integration tests and their fixtures are in `internal/cli`; parsing and editing tests are beside the plan implementation.

Run these checks from `tools/planner/`:

```sh
gofmt -w main.go internal/*/*.go
go vet ./...
golangci-lint run
go test ./...
go test -race ./...
```

After an intentional output change, update the golden files with `go test ./internal/cli -update`.
