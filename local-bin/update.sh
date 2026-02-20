#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=util.sh
source "${SCRIPT_DIR}/util.sh"

OS="$(uname -s)"

update_brew() {
    if ! command -v brew &>/dev/null; then
        echo "ℹ️ Homebrew not found, skipping brew update."
        return
    fi

    echo "⏳ Running brew update..."
    brew update

    echo "⏳ Running brew upgrade..."
    brew upgrade

    echo "✅ Brew update and upgrade complete."
}

update_apt() {
    if [[ "${OS}" != "Linux" ]]; then
        return
    fi

    if ! command -v apt &>/dev/null; then
        echo "ℹ️ apt not found, skipping apt update."
        return
    fi

    if ! [[ -f /etc/os-release ]] || ! grep -qi "ubuntu" /etc/os-release; then
        echo "ℹ️ Not an Ubuntu machine, skipping apt update."
        return
    fi

    echo "⏳ Running apt update..."
    sudo apt update -y

    echo "⏳ Running apt upgrade..."
    sudo apt upgrade -y

    echo "✅ APT update and upgrade complete."
}

update_omz() {
    local omz_dir="${HOME}/.oh-my-zsh"

    if [[ ! -d "${omz_dir}" ]]; then
        echo "ℹ️ Oh My Zsh not found, skipping OMZ update."
        return
    fi

    echo "⏳ Updating Oh My Zsh..."
    zsh -ic "omz update"

    echo "✅ Oh My Zsh update complete."

    echo "⏳ Updating Oh My Zsh plugins..."
    for plugin in "${omz_dir}"/custom/plugins/*/.git; do
        [[ -d "${plugin}" ]] || continue
        git -C "${plugin%/.git}" pull
    done
}

echo "ℹ️ Detected OS: ${OS}"

update_brew
update_apt
update_omz

echo "✅ System update complete."
