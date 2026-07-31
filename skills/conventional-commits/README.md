# Conventional Commits Skill

A Claude Code skill that formats commit messages according to the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) specification.

Each drafted message is linted with `commitlint` (run via [`bun`](https://bun.sh)) using the same rules as CI, and the skill fixes any reported violations before presenting the message. The first run performs a one-time `bun install` into `~/.cache/conventional-commits-skill`; later runs are offline. See `scripts/validate-commit-msg.sh`.

## Usage (in Claude)

```
/conventional-commits look at the cached diff using `git diff --cached` **only**, **don't look at the uncached changes**, and write a commit message. **Don't commit yourself, just output the the message and copy it to clipboard using pbcopy.** Keep the format of the message as markdown, so that code is in backticks. Always use bullets or separate lines when the changes are not related, so that there is a distinction as to what the changes are.
```

## Usage (in the terminal)

```bash
claude -p "/conventional-commits look at the cached diff using `git diff --cached` **only**, **don't look at the uncached changes**, and write a commit message. **Don't commit yourself, you cannot copy to pbcopy since you are running in a claude session started with `claude -p` which is 'single-instruction-print-and-exit-mode', just output the the commit message as-is that I can pipe to the pbcopy and commit** Keep the format of the commit message as markdown, so that code is in backticks. Always use bullets or separate lines when the changes are not related, so that there is a distinction as to what the changes are." | pbcopy && gcmt
```
