#!/usr/bin/env bash

set -euo pipefail

mkdir -p ~/.claude/skills

echo "Installing skills from ./skills dir ..."
for skilldir in skills/*/; do
    [ -d "$skilldir" ] || continue
    skill=$(basename "$skilldir")
    ln -sfn "$(pwd)/$skilldir" ~/.claude/skills/"$skill"
done
echo "Installation successful for public skills!"

if [ -d "dotfilesprivate/skills" ]; then
    echo "Installing skills from ./dotfilesprivate/skills ..."
    for skilldir in dotfilesprivate/skills/*/; do
        [ -d "$skilldir" ] || continue
        skill=$(basename "$skilldir")
        ln -sfn "$(pwd)/$skilldir" ~/.claude/skills/"$skill"
    done
    echo "Installation successful for private skills!"
fi
