# Agent Rules

Vendored rule files (plain markdown) read by Claude Code as global guidance. `make install-rules` symlinks each `.md` file below into `~/.claude/rules/`. This `README.md` is not a rule and is skipped by the installer.

## Vendored rules

All rules below are copied **verbatim** from upstream (no frontmatter or attribution is injected into the `.md` files). Attribution is recorded here and in the commit that vendored them.

| Rule | Upstream | Author |
| --- | --- | --- |
| [simple.md](simple.md) | [abatilo/vimrc](https://github.com/abatilo/vimrc/blob/master/rules/simple.md) | [@abatilo](https://github.com/abatilo) |
| [comments.md](comments.md) | [abatilo/vimrc](https://github.com/abatilo/vimrc/blob/master/rules/comments.md) | [@abatilo](https://github.com/abatilo) |
| [commit-notes.md](commit-notes.md) | [abatilo/vimrc](https://github.com/abatilo/vimrc/blob/master/rules/commit-notes.md) | [@abatilo](https://github.com/abatilo) |
| [simplified-technical-english.md](simplified-technical-english.md) | [abatilo/vimrc](https://github.com/abatilo/vimrc/blob/master/rules/simplified-technical-english.md) | [@abatilo](https://github.com/abatilo) |
| [subtractive-engineering.md](subtractive-engineering.md) | [abatilo/vimrc](https://github.com/abatilo/vimrc/blob/master/rules/subtractive-engineering.md) | [@abatilo](https://github.com/abatilo) |

Vendored at upstream commit [`d4e1614`](https://github.com/abatilo/vimrc/tree/d4e1614ae1e63bafcc88ae88dfb63ff78b9f3ea6/rules).

> **Note:** `abatilo/vimrc` ships no `LICENSE` file; these rules are used here as verbatim copies for personal configuration. See the upstream repo for any licensing terms.

## Updating

This file is **hand-maintained** — the fetch script does not generate it. When you change the registry in `installers/fetch-external-rules.sh`:

1. Run `make fetch-external-rules` to refresh the `.md` files. The script prints the upstream commit SHA.
2. Update the table and the "Vendored at" SHA above to match.
3. Record the SHA in your commit message (e.g. `feat(rules): vendor abatilo/vimrc rules @ d4e1614`).
