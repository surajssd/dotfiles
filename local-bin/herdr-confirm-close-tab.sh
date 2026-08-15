#!/usr/bin/env bash

set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "$(realpath "${BASH_SOURCE[0]}")")"/util.sh

if [[ -z "${HERDR_ACTIVE_TAB_ID:-}" ]]; then
    err "❌ Unable to determine the active Herdr tab."
    read -r -p "Press Enter to close this message."
    exit 1
fi

printf "Close the current tab? [y/N] "
IFS= read -r -n 1 answer
printf "\n"

if [[ "${answer}" != "y" && "${answer}" != "Y" ]]; then
    exit 0
fi

herdr_bin="${HERDR_BIN_PATH:-}"
if [[ -z "${herdr_bin}" ]]; then
    herdr_bin="$(command -v herdr || true)"
fi
if [[ -z "${herdr_bin}" ]]; then
    err "❌ Unable to find the Herdr executable."
    read -r -p "Press Enter to close this message."
    exit 1
fi

"${herdr_bin}" tab close "${HERDR_ACTIVE_TAB_ID}"
