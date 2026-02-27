#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIGS_DIR="$(realpath "${SCRIPT_DIR}/../configs")"

echo "⏳ Installing configs from ${CONFIGS_DIR} dir"

# Install zshrc for OSX and bashrc for the rest.
os=$(uname)
case $os in
Darwin)
    ln -sf "${CONFIGS_DIR}"/zshrc ~/.zshrc

    # Install gpg-agent config for OSX
    mkdir -p ~/.gnupg/
    ln -sf "${CONFIGS_DIR}"/gpg-agent-mac.conf ~/.gnupg/gpg-agent.conf
    ln -sf "${CONFIGS_DIR}"/gpg.conf ~/.gnupg/gpg.conf
    find ~/.gnupg -type d -exec chmod 700 {} \;
    find ~/.gnupg -type f -exec chmod 600 {} \;

    K9S_DIR="$HOME/Library/Application Support/k9s"
    ;;
*)
    ln -sf "${CONFIGS_DIR}"/bashrc ~/.bashrc

    # Install gpg-agent config for Linux
    mkdir -p ~/.gnupg/
    ln -sf "${CONFIGS_DIR}"/gpg-agent-linux.conf ~/.gnupg/gpg-agent.conf
    find ~/.gnupg -type d -exec chmod 700 {} \;
    find ~/.gnupg -type f -exec chmod 600 {} \;

    K9S_DIR="$HOME/.config/k9s"
    ;;
esac

ln -sf "${CONFIGS_DIR}"/gitignore ~/.gitignore
ln -sf "${CONFIGS_DIR}"/terraformrc ~/.terraformrc
ln -sf "${CONFIGS_DIR}"/tmux.conf ~/.tmux.conf
ln -sf "${CONFIGS_DIR}"/starship.toml ~/.config/starship.toml
mkdir -p ~/.claude && ln -sf "${CONFIGS_DIR}"/global-claude-config.md ~/.claude/CLAUDE.md

# Install k9s skin
mkdir -p "${K9S_DIR}/skins"
ln -sf "${CONFIGS_DIR}"/k9s/skins/vscode-light.yaml "${K9S_DIR}/skins/vscode-light.yaml"

echo "✅ Config files installed successfully!"

if [[ -f "${SCRIPT_DIR}"/../dotfilesprivate/install-configs.sh ]]; then
    "${SCRIPT_DIR}"/../dotfilesprivate/install-configs.sh
fi
