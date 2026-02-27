#!/usr/bin/env bash

set -euo pipefail

mkdir -p ~/.local/bin

echo "Installing scripts from ./local-bin dir ..."
# all the files in the 'local-bin' directory will be
# symlinked in ~/.local/bin
for filename in local-bin/*; do
    sym=$(basename "$filename")
    ln -sf "$(pwd)/$filename" ~/.local/bin/"$sym"
done
echo "Installation successful for public scripts!"

if [[ -d dotfilesprivate/local-bin ]]; then
    echo "Installing scripts from ./dotfilesprivate/local-bin ..."
    # Now install the private scripts
    for filename in dotfilesprivate/local-bin/*; do
        sym=$(basename "$filename")
        ln -sf "$(pwd)/$filename" ~/.local/bin/"$sym"
    done
    echo "Installation successful for private scripts!"
fi
