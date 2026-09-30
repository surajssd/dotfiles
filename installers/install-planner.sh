#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib.sh"

if ! command -v go >/dev/null 2>&1; then
    echo "ℹ️ Skipping planner install: go not found in PATH"
    exit 0
fi

echo "⏳ Installing planner Go command from ${REPO_DIR}/planner ..."
go -C "${REPO_DIR}/planner" install . || die "planner build failed"
echo "✅ planner installed into $(go env GOPATH)/bin"
