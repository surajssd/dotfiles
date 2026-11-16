#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_DIR="$(realpath "${SCRIPT_DIR}/../configs/herdr/app")"
HERDR_APP_DIR="${HERDR_APP_HOME:-${HOME}/.config/herdr-app}"

# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib.sh"

echo "⏳ Installing Herdr desktop configuration"

mkdir -p "${HERDR_APP_DIR}"
if [[ -f "${HERDR_APP_DIR}/keys.json" && ! -L "${HERDR_APP_DIR}/keys.json" ]]; then
    mv "${HERDR_APP_DIR}/keys.json" "${HERDR_APP_DIR}/keys.json.before-dotfiles-$(date +%Y%m%d%H%M%S)"
fi

link_tree dir "${CONFIG_DIR}/mods" "${HERDR_APP_DIR}/mods"
ln -sf "${CONFIG_DIR}/keys.json" "${HERDR_APP_DIR}/keys.json"

export HERDR_APP_HOME="${HERDR_APP_DIR}"
MOD_DIR="${HERDR_APP_HOME}/mods/dotfiles-spaces"
LAYOUT_FILE="${HERDR_APP_HOME}/layouts/dotfiles-main.json"

if command -v herdr-app >/dev/null 2>&1 && herdr-app status >/dev/null 2>&1; then
    if [[ ! -d "${MOD_DIR}" ]]; then
        herdr-app "do" mod.clone '{"name":"spaces","as":"dotfiles-spaces"}'
    fi

    MKTEMP=(mktemp)
    if [[ -x /opt/homebrew/opt/coreutils/libexec/gnubin/mktemp ]]; then
        MKTEMP=(/opt/homebrew/opt/coreutils/libexec/gnubin/mktemp --suffix=.js)
    fi
    MOD_TEMP="$("${MKTEMP[@]}")"
    MKTEMP=(mktemp)
    if [[ -x /opt/homebrew/opt/coreutils/libexec/gnubin/mktemp ]]; then
        MKTEMP=(/opt/homebrew/opt/coreutils/libexec/gnubin/mktemp --suffix=.json)
    fi
    LAYOUT_TEMP="$("${MKTEMP[@]}")"
    trap 'rm -f "${MOD_TEMP}" "${LAYOUT_TEMP}"' EXIT

    BEFORE='.child(g ? this.gitTail(g, tree, w.name, t) : div().flex_1())'
    AFTER='.child(g && !w.named ? this.gitTail(g, tree, w.name, t) : div().flex_1())'
    if ! awk -v before="${BEFORE}" -v after="${AFTER}" '
    {
        start = index($0, before)
        if (start) {
            $0 = substr($0, 1, start - 1) after substr($0, start + length(before))
            matches++
        } else if (index($0, after)) {
            matches++
        }
        print
    }
    END { if (matches != 1) exit 1 }
' "${MOD_DIR}/main.js" >"${MOD_TEMP}"; then
        die "Herdr Spaces code has changed. Review dotfiles-spaces/main.js before applying sidebar names."
    fi
    cat "${MOD_TEMP}" >"${MOD_DIR}/main.js"
    herdr-app check "${MOD_DIR}"

    if [[ ! -f "${LAYOUT_FILE}" ]]; then
        herdr-app "do" layout.clone '{"name":"main","as":"dotfiles-main"}'
    fi

    jq '
    if ([.. | objects | select(.mod? == "spaces" or .mod? == "dotfiles-spaces")] | length) != 1 then
        error("expected one Spaces mod in dotfiles-main")
    else
        (.. | objects | select(.mod? == "spaces") | .mod) = "dotfiles-spaces"
    end
' "${LAYOUT_FILE}" >"${LAYOUT_TEMP}"
    cat "${LAYOUT_TEMP}" >"${LAYOUT_FILE}"
    herdr-app "do" layout.switch '{"name":"dotfiles-main"}'

    echo "✅ Herdr sidebar shows only the custom name for renamed spaces"
else
    echo "ℹ️ Open Herdr and rerun installers/install-herdr-app.sh to configure sidebar names."
fi

echo "✅ Herdr desktop configuration installed"
