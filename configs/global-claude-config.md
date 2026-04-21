# System Wide Instructions for Claude

## Local Tools Usage

- Whenever you are searching something locally using grep, use `rg` instead of `grep`.
- When on macOS, prefer using `container` CLI for running containers instead of `docker`, until and unless user explicitly requires `docker`.

## Shell Scripts

- After writing a shell script, make sure you run the command `shellfmt.sh <filename>` to format the script properly.
- Whenever you are asked to save the analysis or certain content to a file for later reference, use the command `ssd dump -f <appropriate extension> <topic>` to save the content in a well-formatted way.
- Whenever you need to create a temporary file use `mktemp --suffix=.<appropriate extension>` to ensure the file is created with the correct extension.

## Go Programming

- Always run `gofmt` on the go code you modify to ensure it is properly formatted.
