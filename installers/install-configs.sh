#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIGS_DIR="$(realpath "${SCRIPT_DIR}/../configs")"

echo "⏳ Installing configs from ${CONFIGS_DIR} dir"

# Install zshrc for OSX and bashrc for the rest.
os=$(uname)

# Setup gnupg directory (shared across OS)
mkdir -p ~/.gnupg/

case $os in
Darwin)
    ln -sf "${CONFIGS_DIR}"/zshrc ~/.zshrc

    # Install gpg configs for macOS
    ln -sf "${CONFIGS_DIR}"/gpg-agent-mac.conf ~/.gnupg/gpg-agent.conf
    ln -sf "${CONFIGS_DIR}"/gpg.conf ~/.gnupg/gpg.conf

    K9S_DIR="$HOME/Library/Application Support/k9s"
    ;;
*)
    ln -sf "${CONFIGS_DIR}"/bashrc ~/.bashrc

    # Install gpg-agent config for Linux
    ln -sf "${CONFIGS_DIR}"/gpg-agent-linux.conf ~/.gnupg/gpg-agent.conf

    K9S_DIR="$HOME/.config/k9s"
    ;;
esac

# Set correct permissions on gnupg directory
chmod 700 ~/.gnupg
# Only chmod config files if they are regular files (not symlinks to read-only mounts)
[ -f ~/.gnupg/gpg-agent.conf ] && [ ! -L ~/.gnupg/gpg-agent.conf ] && chmod 600 ~/.gnupg/gpg-agent.conf
[ -f ~/.gnupg/gpg.conf ] && [ ! -L ~/.gnupg/gpg.conf ] && chmod 600 ~/.gnupg/gpg.conf

ln -sf "${CONFIGS_DIR}"/gitignore ~/.gitignore
mkdir -p ~/.terraform.d/plugin-cache && ln -sf "${CONFIGS_DIR}"/terraformrc ~/.terraformrc
ln -sf "${CONFIGS_DIR}"/tmux.conf ~/.tmux.conf
mkdir -p ~/.config && ln -sf "${CONFIGS_DIR}"/starship.toml ~/.config/starship.toml
mkdir -p ~/.claude && ln -sf "${CONFIGS_DIR}"/global-claude-config.md ~/.claude/CLAUDE.md

# Install k9s skin
mkdir -p "${K9S_DIR}/skins"
ln -sf "${CONFIGS_DIR}"/k9s/skins/vscode-light.yaml "${K9S_DIR}/skins/vscode-light.yaml"

echo "✅ Config files installed successfully!"

if [[ -f "${SCRIPT_DIR}"/../dotfilesprivate/install-configs.sh ]]; then
    "${SCRIPT_DIR}"/../dotfilesprivate/install-configs.sh
fi
