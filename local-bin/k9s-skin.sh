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
SKINS_DIR="${K9S_DIR}/skins"

usage() {
    echo "Usage: $(basename "$0") <command> [args]"
    echo ""
    echo "Commands:"
    echo "  set [skin]   Set the k9s skin, or unset if no skin provided"
    echo "  list         List available skins"
    exit 1
}

cmd_set() {
    if [[ ! -f "$CONFIG_FILE" ]]; then
        echo "ℹ️ k9s config not found at ${CONFIG_FILE}, skipping skin setup"
        exit 0
    fi

    if [[ -z "${1:-}" ]]; then
        echo "⏳ Unsetting k9s skin in ${CONFIG_FILE}"
        yq eval 'del(.k9s.ui.skin)' -i "$CONFIG_FILE"
        echo "✅ k9s skin unset"
        return
    fi

    local skin="$1"
    echo "⏳ Setting k9s skin to ${skin} in ${CONFIG_FILE}"
    yq eval ".k9s.ui.skin = \"${skin}\"" -i "$CONFIG_FILE"
    echo "✅ k9s skin set to ${skin}"
}

cmd_list() {
    if [[ ! -d "$SKINS_DIR" ]]; then
        echo "❌ Skins directory not found at ${SKINS_DIR}"
        exit 1
    fi

    echo "Available k9s skins:"
    for skin_file in "${SKINS_DIR}"/*.yaml; do
        [[ -f "$skin_file" ]] || continue
        basename "$skin_file" .yaml
    done
}

COMMAND="${1:-}"
shift || true

case "$COMMAND" in
set)
    cmd_set "$@"
    ;;
list)
    cmd_list
    ;;
*)
    usage
    ;;
esac
