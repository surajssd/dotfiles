#!/usr/bin/env bash

set -euo pipefail

# Hand focus back to whatever app git was invoked from. macOS sets
# __CFBundleIdentifier to the bundle ID of the launching GUI app, so this works
# for any terminal (iTerm, Terminal, Ghostty, WezTerm, Kitty, Alacritty, ...) and
# is a no-op when unset (SSH sessions, login shells, non-bundle parents).
restore_terminal_focus() {
    if [[ -n "${__CFBundleIdentifier:-}" ]]; then
        open -b "${__CFBundleIdentifier}"
    fi
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
