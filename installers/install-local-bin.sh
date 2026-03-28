#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"

mkdir -p ~/.local/bin

echo "⏳ Installing scripts from ${REPO_DIR}/local-bin dir ..."
# all the files in the 'local-bin' directory will be
# symlinked in ~/.local/bin
for filename in "${REPO_DIR}"/local-bin/*; do
    sym=$(basename "$filename")
    ln -sf "$filename" ~/.local/bin/"$sym"
done
echo "✅ Installation successful for public scripts!"

if [[ -d "${REPO_DIR}/dotfilesprivate/local-bin" ]]; then
    echo "⏳ Installing scripts from ${REPO_DIR}/dotfilesprivate/local-bin ..."
    # Now install the private scripts
    for filename in "${REPO_DIR}"/dotfilesprivate/local-bin/*; do
        sym=$(basename "$filename")
        ln -sf "$filename" ~/.local/bin/"$sym"
    done
    echo "✅ Installation successful for private scripts!"
fi
