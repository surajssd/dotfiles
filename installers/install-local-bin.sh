#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib.sh"

DEST=~/.local/bin

# Files in local-bin/ that must NOT be symlinked into PATH
# (sourceable libraries, git hook scripts, etc.). Keep in sync — see CLAUDE.md.
readonly LOCAL_BIN_SKIP=(util.sh git-autopush-post-commit)

echo "⏳ Installing scripts from ${REPO_DIR}/local-bin dir ..."
link_tree file "${REPO_DIR}/local-bin" "$DEST" "${LOCAL_BIN_SKIP[@]}"
echo "✅ Installation successful for public scripts!"

if [[ -d "${REPO_DIR}/dotfilesprivate/local-bin" ]]; then
    echo "⏳ Installing scripts from ${REPO_DIR}/dotfilesprivate/local-bin ..."
    link_tree file "${REPO_DIR}/dotfilesprivate/local-bin" "$DEST" "${LOCAL_BIN_SKIP[@]}"
    echo "✅ Installation successful for private scripts!"
fi

# Clean up dead symlinks.
prune_dead_symlinks "$DEST"
