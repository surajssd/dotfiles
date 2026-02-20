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

echo "ℹ️ Detected OS: ${OS}"

update_brew
update_apt

echo "✅ System update complete."
