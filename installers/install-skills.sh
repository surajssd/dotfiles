#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"

mkdir -p ~/.claude/skills

echo "⏳ Installing skills from ${REPO_DIR}/skills dir ..."
shopt -s nullglob
for skilldir in "${REPO_DIR}"/skills/*/; do
    [ -d "$skilldir" ] || continue
    skill=$(basename "$skilldir")
    ln -sfn "$skilldir" ~/.claude/skills/"$skill"
done
shopt -u nullglob
echo "✅ Installation successful for public skills!"

if [ -d "${REPO_DIR}/dotfilesprivate/skills" ]; then
    echo "⏳ Installing skills from ${REPO_DIR}/dotfilesprivate/skills ..."
    shopt -s nullglob
    for skilldir in "${REPO_DIR}"/dotfilesprivate/skills/*/; do
        [ -d "$skilldir" ] || continue
        skill=$(basename "$skilldir")
        ln -sfn "$skilldir" ~/.claude/skills/"$skill"
    done
    shopt -u nullglob
    echo "✅ Installation successful for private skills!"
fi
