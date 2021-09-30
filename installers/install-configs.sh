#!/usr/bin/env bash

set -euo pipefail

echo "Installing configs from ./configs dir"

ln -sf `pwd`/configs/bashrc ~/.bashrc
ln -sf `pwd`/configs/gitconfig ~/.gitconfig
ln -sf `pwd`/configs/gitignore ~/.gitignore
ln -sf `pwd`/configs/terraformrc ~/.terraformrc

echo "Installation successful."
