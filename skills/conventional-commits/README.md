# Conventional Commits Skill

A Claude Code skill that formats commit messages according to the [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) specification.

## Usage (in Claude)

```
/conventional-commits look at the cached diff using `git diff --cached` **only**, **don't look at the uncached changes**, and write a commit message. **Don't commit yourself, just output the the message and copy it to clipboard using pbcopy.** Keep the format of the message as markdown, so that code is in backticks. Always use bullets or separate lines when the changes are not related, so that there is a distinction as to what the changes are.
```

## Usage (in the terminal)

```bash
claude -p "/conventional-commits look at the cached diff using `git diff --cached` **only**, **don't look at the uncached changes**, and write a commit message. **Don't commit yourself, just output the the message as-is that I can pipe to the pbcopy and commit** Keep the format of the message as markdown, so that code is in backticks. Always use bullets or separate lines when the changes are not related, so that there is a distinction as to what the changes are." | pbcopy && gcmt
```
