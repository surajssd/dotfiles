#!/bin/bash

readonly path="$HOME/.bash-git-prompt"

ls $path
if [ $? -ne 0 ]; then
    echo "Installing bash git prompt in $path"
    git clone https://github.com/magicmonty/bash-git-prompt.git $path
else
    echo "Updating bash git prompt in $path"
    cd $path
    git pull --ff origin master
fi
