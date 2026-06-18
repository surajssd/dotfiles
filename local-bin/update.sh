#!/usr/bin/env bash

set -euo pipefail

OS="$(uname -s)"

update_brew() {
    if command -v zsh &>/dev/null; then
        ZSH_PATH=$(zsh -c 'source ~/.zshrc >/dev/null 2>&1; echo $PATH')
        export PATH="${ZSH_PATH}:${PATH}"
    fi
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

    if ! [[ -f /etc/os-release ]] || ! command grep -qiE "ubuntu|debian" /etc/os-release; then
        echo "ℹ️ Not an Ubuntu/Debian machine, skipping apt update."
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
    # Use Oh My Zsh's unattended updater (tools/upgrade.sh) instead of the
    # 'omz update' CLI. When 'omz update' pulls new commits, _omz::update (in
    # ~/.oh-my-zsh/lib/cli.zsh) ends by re-exec'ing a fresh *interactive* zsh:
    #   [[ "$zsh" = -* || -o login ]] && exec -l "${zsh#-}" || exec "$zsh"
    # Launched from this script, that replacement shell inherits the terminal as
    # stdin and sits at a prompt, blocking the rest of update.sh until you press
    # Ctrl+D — which then looks like the script "running again". upgrade.sh runs
    # the same update but just exits, so control returns here cleanly.
    if [[ -f "${omz_dir}/tools/upgrade.sh" ]]; then
        ZSH="${omz_dir}" zsh -f "${omz_dir}/tools/upgrade.sh"
    else
        # Fallback for unusual installs: redirect stdin from /dev/null so any
        # interactive shell OMZ exec's reads EOF immediately instead of blocking.
        zsh -ic "omz update" </dev/null
    fi

    echo "✅ Oh My Zsh update complete."

    echo "⏳ Updating Oh My Zsh plugins..."
    for plugin in "${omz_dir}"/custom/plugins/*/.git; do
        [[ -d "${plugin}" ]] || continue
        git -C "${plugin%/.git}" pull
    done
}

update_dotfiles() {
    local script_path repo_dir
    # Resolve through the ~/.local/bin symlink to the real script in the repo,
    # then walk up from local-bin/ to the dotfiles repo root.
    script_path="$(realpath "${BASH_SOURCE[0]}")"
    repo_dir="$(dirname "$(dirname "${script_path}")")"

    if [[ ! -f "${repo_dir}/Makefile" ]]; then
        echo "ℹ️ Dotfiles Makefile not found at ${repo_dir}, skipping dotfiles update."
        return
    fi

    if ! command -v make &>/dev/null; then
        echo "ℹ️ make not found, skipping dotfiles update."
        return
    fi

    echo "⏳ Updating dotfiles via 'make update' in ${repo_dir}..."
    (cd "${repo_dir}" && make update)

    echo "✅ Dotfiles update complete."
}

echo "ℹ️ Detected OS: ${OS}"

# Wrap the run sequence in a brace group so Bash parses it fully before
# executing. update_dotfiles runs 'make update', which 'git pull's this very
# script; pre-parsing plus the trailing 'exit' guarantees Bash never re-reads
# the file after it changes on disk.
{
    update_brew
    update_apt
    update_omz
    update_dotfiles

    echo "✅ System update complete."
    exit 0
}
