#!/usr/bin/env bash

set -euo pipefail

# Determine k9s config directory based on OS
os=$(uname)
case $os in
Darwin)
    K9S_DIR="$HOME/Library/Application Support/k9s"
    ;;
*)
    K9S_DIR="$HOME/.config/k9s"
    ;;
esac

CONFIG_FILE="${K9S_DIR}/config.yaml"

if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "ℹ️ k9s config not found at ${CONFIG_FILE}, skipping skin setup"
    exit 0
fi

echo "⏳ Setting k9s skin to vscode-light in ${CONFIG_FILE}"
yq eval '.k9s.ui.skin = "vscode-light"' -i "$CONFIG_FILE"
echo "✅ k9s skin set to vscode-light"
