#!/usr/bin/env bash
#
# This script helps you with checking if the shell script is valid with all the
# conventions of bash scripting.
# shfmt can be downloaded from https://github.com/mvdan/sh/releases

set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "${BASH_SOURCE[0]}")"/util.sh

# if no cmd line flag provided this should error out
if [[ $# -eq 0 ]]; then
  err "Please provide a filename, usage: shellfmt.sh <filename>"
  exit 1
fi

echo "Doing shellcheck for any errors..."
shellcheck "${1}"

echo "Doing shfmt on the file..."
shfmt -i 2 -w "${1}"

echo "No errors in file: ${1}"
