#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib.sh"

# Symlink rules into Claude Code's global rules path.
#   ~/.claude/rules - read by Claude Code as global rule instructions
DEST=~/.claude/rules

# Files in rules/ that must NOT be symlinked into ~/.claude/rules
# (not rule files; keep in sync — see CLAUDE.md).
readonly RULES_SKIP=(README.md)

echo "⏳ Installing public rules into ${DEST} ..."
link_tree file "${REPO_DIR}/rules" "$DEST" "${RULES_SKIP[@]}"

if [[ -d "${REPO_DIR}/dotfilesprivate/rules" ]]; then
    echo "⏳ Installing private rules into ${DEST} ..."
    link_tree file "${REPO_DIR}/dotfilesprivate/rules" "$DEST" "${RULES_SKIP[@]}"
fi

# Remove broken symlinks for rules we no longer ship.
prune_dead_symlinks "$DEST"
echo "✅ Rules installed into ${DEST}"
