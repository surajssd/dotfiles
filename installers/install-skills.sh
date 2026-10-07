#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib.sh"

# Symlink skills into every agent CLI that reads the SKILL.md format.
#   ~/.claude/skills  - Claude Code's native global skills path
#   ~/.agents/skills  - vendor-neutral path read by Codex, Gemini, opencode,
#                       and Copilot CLI (verified against each tool's docs)
DESTS=(
    ~/.claude/skills
    ~/.agents/skills
)

for DEST in "${DESTS[@]}"; do
    echo "⏳ Installing public skills into ${DEST} ..."
    link_tree dir "${REPO_DIR}/skills" "$DEST"

    if [[ -d "${REPO_DIR}/dotfilesprivate/skills" ]]; then
        echo "⏳ Installing private skills into ${DEST} ..."
        link_tree dir "${REPO_DIR}/dotfilesprivate/skills" "$DEST"
    fi

    # Remove only broken symlinks for skills we no longer ship.
    prune_dead_symlinks "$DEST"
    echo "✅ Skills installed into ${DEST}"
done
