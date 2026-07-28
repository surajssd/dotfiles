#!/usr/bin/env bash
#
# detect_reviewers.sh — report which candidate reviewer CLIs are installed.
#
# Prints one line per candidate:
#   available <label> <tool>
#   missing   <tool>
#
# The orchestrator builds its panel from the `available` lines and tells the user
# which tools were skipped. Invoking a missing binary just wastes a turn, so we
# never do it — we detect first.

set -euo pipefail

# Candidate roster. `agy` is the Google Antigravity CLI. `cursor` is the Cursor CLI —
# its real binary is `cursor-agent` (also aliased as `agent` by the installer, but
# `agent` is too generic a name to safely `command -v` for, so we check the
# distinctive one).
CANDIDATES=(claude codex agy opencode cursor)

is_installed() {
    command -v "$1" >/dev/null 2>&1
}

# Map a candidate's panel identifier to the binary `command -v` should check. Only
# `cursor` differs from its own identifier; every other candidate's binary name IS
# its identifier.
binary_for() {
    case "$1" in
    cursor) echo "cursor-agent" ;;
    *) echo "$1" ;;
    esac
}

for tool in "${CANDIDATES[@]}"; do
    if is_installed "$(binary_for "${tool}")"; then
        # Default label is the tool name; the orchestrator overrides it when a
        # user runs the same tool with multiple models.
        echo "available ${tool} ${tool}"
    else
        echo "missing ${tool}"
    fi
done
