#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"

mkdir -p ~/.local/bin

# Files in local-bin/ that must NOT be symlinked into PATH
# (sourceable libraries, git hook scripts, etc.). Keep in sync — see CLAUDE.md.
readonly LOCAL_BIN_SKIP=(util.sh git-autopush-post-commit)

echo "⏳ Installing scripts from ${REPO_DIR}/local-bin dir ..."
# all the files in the 'local-bin' directory will be
# symlinked in ~/.local/bin
shopt -s nullglob
for filename in "${REPO_DIR}"/local-bin/*; do
    sym=$(basename "$filename")
    skip=false
    for s in "${LOCAL_BIN_SKIP[@]}"; do
        [[ "$sym" == "$s" ]] && {
            skip=true
            break
        }
    done
    [[ "$skip" == true ]] && continue
    ln -sf "$filename" ~/.local/bin/"$sym"
done
shopt -u nullglob
echo "✅ Installation successful for public scripts!"

if [[ -d "${REPO_DIR}/dotfilesprivate/local-bin" ]]; then
    echo "⏳ Installing scripts from ${REPO_DIR}/dotfilesprivate/local-bin ..."
    # Now install the private scripts
    shopt -s nullglob
    for filename in "${REPO_DIR}"/dotfilesprivate/local-bin/*; do
        sym=$(basename "$filename")
        ln -sf "$filename" ~/.local/bin/"$sym"
    done
    shopt -u nullglob
    echo "✅ Installation successful for private scripts!"
fi

# Clean up dead symlinks.
find ~/.local/bin -type l ! -exec test -e {} \; -delete
