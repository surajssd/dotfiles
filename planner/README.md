# Planner

[Back to the README](../README.md#tools)

Manage Markdown plans by repository, track implementation status, and check metadata and links.

## Setup

Run `make install-planner` from the repository root. Rebuild after changing Go code.

Set the plan root in `~/.planner.yaml`:

```yaml
root: ~/plans
```

Use `--root <dir>` to override it.

## Plan format

Plans live at `<root>/github.com/<org>/<repo>/<YYMMDDHHMMSS>-<name>.md`. Front matter keys are case-sensitive.

| Fields | Purpose |
| --- | --- |
| `Type`, `ImplementationStatus`, `StatusChecked`, `StatusNote` | Record the plan type, status, last check date, and summary. |
| `Parent`, `SupersededBy` | Link related plans using a wikilink, path, or URL. |
| `Issues`, `PullRequests` | Store lists of tracker and pull request URLs. |

## Browse plans

| Command | Result |
| --- | --- |
| `planner get [<plan>...]` | Lists active plans, or the named plans regardless of status. |
| `planner tree [<plan>...]` | Shows an effort tree, or a named plan's subtree. |
| `planner describe [--github] <plan>...` | Shows fields, children, notes, and findings. `--github` adds pull request states from `gh`. |
| `planner get repos` | Lists every repository with plans. |

A plan reference can be a path, wikilink, basename, short name from the table, or listed URL. URLs select every matching plan. Unmatched references exit 1 after found results.

### Filters and output

| Option | Behavior |
| --- | --- |
| `--all` | Includes inactive plans and plans with missing or invalid front matter. |
| `--repo <text>` | Matches repository names by case-insensitive substring. |
| `--status <status>` | Matches any case, with or without hyphens. Includes matching inactive plans without `--all`. |
| `-o wide` | Adds `TYPE`, `PATH`, and `NOTE` to `NAME REPO STATUS CHECKED TITLE`. Paths abbreviate home as `~`. |
| `-o json` / `-o yaml` | Prints all fields. One named plan is an object; other selections use an `items` list. |
| `-o name` | Prints basenames. |
| `--no-headers` | Omits table headers. |

`tree` supports only `-o wide`. `CHECKED` shows days since the last check; `!` marks active plans older than a week. Invalid front matter shows `-` in status and checked columns.

## Create and update plans

| Command | Action |
| --- | --- |
| `planner create <name...>` | Creates a plan for the current repository and prints its path. |
| `planner log <plan> <title...> < entry.md` | Appends a dated progress entry from a file. |
| `planner set status <plan> <status> --note "<text>"` | Updates status, checked date, and summary. |
| `planner set parent <plan> <ref>` | Sets the parent plan. |
| `planner set pr <plan> <url>...` | Adds pull request URLs. |
| `planner set issue <plan> <url>...` | Adds tracker URLs. |

- `create` accepts `--parent` and an initial `--status` with `--note`. Both `create` and `log` accept repeatable `--issue` and `--pr` flags; duplicate URLs are skipped.
- `log` updates `StatusChecked` to today. Use `--keep-checked` for administrative entries and `--note` to replace the summary in the same write.
- `set status` accepts `--note-file <path>` or `--note-file -` for stdin. Use `--superseded-by <ref>` when setting the status to `Superseded`.

## Validate plans

Run `planner check [<plan>...]` to find metadata and link problems. Add `--github` to warn when a note calls a closed or merged pull request open. Error findings exit 1.

Run `planner check --help` for schema keys, status values, and validation rules. Duplicate-title warnings exclude plans explicitly superseded by another plan with the same title in the same repository.

## Help and completion

Use `--help` on any command for all flags and output fields. The shell configs load `planner completion` when the binary is available; it completes commands, flags, plans, repositories, statuses, and formats.
