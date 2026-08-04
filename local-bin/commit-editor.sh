#!/usr/bin/env bash

set -euo pipefail

# Hand focus back to the terminal git was invoked from. TERM_PROGRAM is unset or
# names something we can't activate (VS Code's own terminal, an SSH session) often
# enough that anything unrecognised is left alone.
restore_terminal_focus() {
    case "${TERM_PROGRAM:-}" in
    iTerm.app) open -b com.googlecode.iterm2 ;;
    Apple_Terminal) open -b com.apple.Terminal ;;
    esac
}

if command -v code >/dev/null 2>&1; then
    # The `code` CLI runs headless (ELECTRON_RUN_AS_NODE=1) and hands the file to
    # the running instance over IPC, which macOS won't let it focus. Activate the
    # app first so the commit message opens in an already-frontmost window.
    open -a "Visual Studio Code"
    # On EXIT, not after `code`, so focus returns even on an aborted commit.
    trap restore_terminal_focus EXIT
    code --wait "$@"
else
    vi "$@"
fi
