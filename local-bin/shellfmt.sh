#!/bin/bash

set -euo pipefail

echo "Doing shellcheck for any errors..."
shellcheck "${1}"

echo "Doing shfmt on the file..."
shfmt -i 2 -w "${1}"
