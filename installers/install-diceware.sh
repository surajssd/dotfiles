#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib.sh"

if ! command -v go >/dev/null 2>&1; then
    echo "ℹ️ Skipping diceware install: go not found in PATH"
    exit 0
fi

echo "⏳ Installing diceware Go command from ${REPO_DIR}/tools/diceware ..."
go -C "${REPO_DIR}/tools/diceware" install ./cmd/diceware || die "diceware build failed"
echo "✅ diceware installed into $(go env GOPATH)/bin"
