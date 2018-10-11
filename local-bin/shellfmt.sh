#!/bin/bash

set -euo pipefail

if [[ $# -eq 0 ]]; then
  echo "Please provide a filename, usage: shellfmt.sh <filename>"
  exit 1
fi

echo "Doing shellcheck for any errors..."
shellcheck "${1}"

echo "Doing shfmt on the file..."
shfmt -i 2 -w "${1}"
