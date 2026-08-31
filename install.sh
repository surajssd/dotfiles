#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if ! command -v make >/dev/null 2>&1; then
    echo "❌ Required command not found: make" >&2
    exit 1
fi

make -C "${SCRIPT_DIR}" install-all
