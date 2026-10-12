#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib.sh"

if ! command -v go >/dev/null 2>&1; then
    echo "ℹ️ Skipping yt-playlist-creator install: go not found in PATH"
    exit 0
fi

echo "⏳ Installing yt-playlist-creator Go command from ${REPO_DIR}/yt-playlist-creator ..."
go -C "${REPO_DIR}/yt-playlist-creator" install . || die "yt-playlist-creator build failed"
echo "✅ yt-playlist-creator installed into $(go env GOPATH)/bin"
