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

# Candidate roster. `agency` is the `agency copilot` wrapper.
CANDIDATES=(claude codex gemini opencode copilot agency)

is_installed() {
    command -v "$1" >/dev/null 2>&1
}

for tool in "${CANDIDATES[@]}"; do
    if is_installed "${tool}"; then
        # Default label is the tool name; the orchestrator overrides it when a
        # user runs the same tool with multiple models.
        echo "available ${tool} ${tool}"
    else
        echo "missing ${tool}"
    fi
done
