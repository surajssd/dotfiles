#!/usr/bin/env bash

set -euo pipefail

echo "Installing configs from ./configs dir"

# Install zshrc for OSX and bashrc for the rest.
os=$(uname)
case $os in
Darwin)
  ln -sf $(pwd)/configs/zshrc ~/.zshrc

  # Install gpg-agent config for OSX
  mkdir -p ~/.gnupg/
  ln -sf $(pwd)/configs/gpg-agent-mac.conf ~/.gnupg/gpg-agent.conf
  ;;
*)
  ln -sf $(pwd)/configs/bashrc ~/.bashrc

  # Install gpg-agent config for OSX
  mkdir -p ~/.gnupg/
  ln -sf $(pwd)/configs/gpg-agent-linux.conf ~/.gnupg/gpg-agent.conf
  ;;
esac

ln -sf $(pwd)/configs/gitconfig ~/.gitconfig
ln -sf $(pwd)/configs/gitignore ~/.gitignore
ln -sf $(pwd)/configs/terraformrc ~/.terraformrc
ln -sf $(pwd)/configs/tmux.conf ~/.tmux.conf
ln -sf $(pwd)/configs/ssh_config ~/.ssh/config

echo "Installation successful."
