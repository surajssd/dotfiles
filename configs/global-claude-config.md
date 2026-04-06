# System Wide Instructions for Claude

## Local Tools Usage

- Whenever you are searching something locally using grep, use `rg` instead of `grep`.
- When on macOS, prefer using `container` CLI for running containers instead of `docker`, until and unless user explicitly requires `docker`.

## Shell Scripts

- After writing a shell script, make sure you run the command `shellfmt.sh <filename>` to format the script properly.

## Go Programming

- Always run `gofmt` on the go code you modify to ensure it is properly formatted.
