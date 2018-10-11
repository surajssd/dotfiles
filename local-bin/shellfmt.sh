#!/bin/bash

set -euo pipefail

err() {
  echo "$*" >&2
}

if [[ $# -eq 0 ]]; then
  err "Please provide a filename, usage: shellfmt.sh <filename>"
  exit 1
fi

echo "Doing shellcheck for any errors..."
shellcheck "${1}"

echo "Doing shfmt on the file..."
shfmt -i 2 -w "${1}"

echo "No errors in file: ${1}"
