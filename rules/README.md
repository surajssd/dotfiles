# Agent Rules

Vendored rule files (plain markdown) read by Claude Code as global guidance. `make install-rules` symlinks each `.md` file below into `~/.claude/rules/`. This `README.md` is not a rule and is skipped by the installer.

## Vendored rules

All rules below are copied **verbatim** from upstream (no frontmatter or attribution is injected into the `.md` files). Attribution is recorded here and in the commit that vendored them.

| Rule | Upstream | Author |
| --- | --- | --- |
| [simple.md](simple.md) | [abatilo/vimrc](https://github.com/abatilo/vimrc/blob/d4e1614ae1e63bafcc88ae88dfb63ff78b9f3ea6/rules/simple.md) | [@abatilo](https://github.com/abatilo) |
| [comments.md](comments.md) | [abatilo/vimrc](https://github.com/abatilo/vimrc/blob/d4e1614ae1e63bafcc88ae88dfb63ff78b9f3ea6/rules/comments.md) | [@abatilo](https://github.com/abatilo) |
| [commit-notes.md](commit-notes.md) | [abatilo/vimrc](https://github.com/abatilo/vimrc/blob/d4e1614ae1e63bafcc88ae88dfb63ff78b9f3ea6/rules/commit-notes.md) | [@abatilo](https://github.com/abatilo) |
| [simplified-technical-english.md](simplified-technical-english.md) | [abatilo/vimrc](https://github.com/abatilo/vimrc/blob/d4e1614ae1e63bafcc88ae88dfb63ff78b9f3ea6/rules/simplified-technical-english.md) | [@abatilo](https://github.com/abatilo) |
| [subtractive-engineering.md](subtractive-engineering.md) | [abatilo/vimrc](https://github.com/abatilo/vimrc/blob/d4e1614ae1e63bafcc88ae88dfb63ff78b9f3ea6/rules/subtractive-engineering.md) | [@abatilo](https://github.com/abatilo) |

Vendored at upstream commit [`d4e1614`](https://github.com/abatilo/vimrc/tree/d4e1614ae1e63bafcc88ae88dfb63ff78b9f3ea6/rules).

Upstream removed its `rules/` directory in [`17bf559`](https://github.com/abatilo/vimrc/commit/17bf5599abe2045419b48631591a0260df8e7bf4) (2026-09-16) and replaced it with a single [`AGENTS.md`](https://github.com/abatilo/vimrc/blob/master/AGENTS.md) that rewrites the content. The five files above are the last upstream versions and are registered in `preserve` mode, so `make fetch-external-rules` verifies they exist but never overwrites them.

> **Note:** `abatilo/vimrc` ships no `LICENSE` file; these rules are used here as verbatim copies for personal configuration. See the upstream repo for any licensing terms.

## Updating

This file is **hand-maintained** — the fetch script does not generate it. The current registry has no `fetch` entries, so the script downloads nothing. To pick up upstream's new `AGENTS.md`, add a `fetch` entry for it in `installers/fetch-external-rules.sh` and decide which of the frozen files it replaces.

When you add or change a `fetch` entry:

1. Run `make fetch-external-rules` to refresh the `.md` files. The script prints the upstream commit SHA.
2. Update the table and the "Vendored at" SHA above to match.
3. Record the SHA in your commit message (e.g. `feat(rules): vendor abatilo/vimrc rules @ d4e1614`).
