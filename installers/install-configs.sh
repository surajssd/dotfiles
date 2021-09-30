#!/usr/bin/env bash

set -euo pipefail

echo "Installing configs from ./configs dir"

# Install zshrc for OSX and bashrc for the rest.
os=$(uname)
case $os in
  Darwin)
  ln -sf `pwd`/configs/zshrc ~/.zshrc
  ;;
  *)
  ln -sf `pwd`/configs/bashrc ~/.bashrc
  ;;
esac

ln -sf `pwd`/configs/gitconfig ~/.gitconfig
ln -sf `pwd`/configs/gitignore ~/.gitignore
ln -sf `pwd`/configs/terraformrc ~/.terraformrc

echo "Installation successful."
