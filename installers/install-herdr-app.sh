#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="$(realpath "${SCRIPT_DIR}/../configs/herdr/app")"
HERDR_APP_DIR="${HERDR_APP_HOME:-${HOME}/.config/herdr-app}"

# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib.sh"

echo "⏳ Installing Herdr desktop worktree shortcuts"

mkdir -p "${HERDR_APP_DIR}"
if [[ -f "${HERDR_APP_DIR}/keys.json" && ! -L "${HERDR_APP_DIR}/keys.json" ]]; then
    mv "${HERDR_APP_DIR}/keys.json" "${HERDR_APP_DIR}/keys.json.before-dotfiles-$(date +%Y%m%d%H%M%S)"
fi

link_tree dir "${CONFIG_DIR}/mods" "${HERDR_APP_DIR}/mods"
ln -sf "${CONFIG_DIR}/keys.json" "${HERDR_APP_DIR}/keys.json"

echo "✅ Herdr desktop worktree shortcuts installed"
