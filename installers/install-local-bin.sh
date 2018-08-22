#!/bin/bash

set -x

mkdir -p ~/.local/bin

# all the files in the 'local-bin' directory will be 
# symlinked in ~/.local/bin
for filename in local-bin/*; do
    sym=$(basename $filename)
    ln -sf `pwd`/$filename ~/.local/bin/$sym
done
