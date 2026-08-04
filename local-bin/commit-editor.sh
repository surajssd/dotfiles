#!/usr/bin/env bash

set -euo pipefail

if command -v code >/dev/null 2>&1; then
    # The `code` CLI runs headless (ELECTRON_RUN_AS_NODE=1) and hands the file to
    # the running instance over IPC, which macOS won't let it focus. Activate the
    # app first so the commit message opens in an already-frontmost window.
    open -a "Visual Studio Code"
    code --wait "$@"
else
    vi "$@"
fi
