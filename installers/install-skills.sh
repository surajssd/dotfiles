#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib.sh"

DEST=~/.claude/skills

echo "⏳ Installing skills from ${REPO_DIR}/skills dir ..."
link_tree dir "${REPO_DIR}/skills" "$DEST"
echo "✅ Installation successful for public skills!"

if [[ -d "${REPO_DIR}/dotfilesprivate/skills" ]]; then
    echo "⏳ Installing skills from ${REPO_DIR}/dotfilesprivate/skills ..."
    link_tree dir "${REPO_DIR}/dotfilesprivate/skills" "$DEST"
    echo "✅ Installation successful for private skills!"
fi

# Clean up dead symlinks.
prune_dead_symlinks "$DEST"
