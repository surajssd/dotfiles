#!/usr/bin/env bash

set -euo pipefail

if command -v code >/dev/null 2>&1; then
    code --wait "$@"
else
    vi "$@"
fi
