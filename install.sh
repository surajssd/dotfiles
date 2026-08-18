#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

for required_command in make go; do
    if ! command -v "${required_command}" >/dev/null 2>&1; then
        echo "❌ Required command not found: ${required_command}" >&2
        exit 1
    fi
done

required_go_version=""
while read -r directive value; do
    if [[ "${directive}" == "go" ]]; then
        required_go_version="${value}"
        break
    fi
done <"${SCRIPT_DIR}/clawbox/go.mod"

installed_go_version="$(go env GOVERSION)"
installed_version="${installed_go_version#go}"

if [[ ! "${installed_version}" =~ ^([0-9]+)\.([0-9]+)(\.([0-9]+))? ]]; then
    echo "❌ Unable to determine the installed Go version: ${installed_go_version}" >&2
    exit 1
fi

installed_major="${BASH_REMATCH[1]}"
installed_minor="${BASH_REMATCH[2]}"
installed_patch="${BASH_REMATCH[4]:-0}"

if [[ ! "${required_go_version}" =~ ^([0-9]+)\.([0-9]+)(\.([0-9]+))? ]]; then
    echo "❌ Unable to determine the required Go version from clawbox/go.mod" >&2
    exit 1
fi

required_major="${BASH_REMATCH[1]}"
required_minor="${BASH_REMATCH[2]}"
required_patch="${BASH_REMATCH[4]:-0}"

if ((installed_major < required_major)) ||
    ((installed_major == required_major && installed_minor < required_minor)) ||
    ((installed_major == required_major && installed_minor == required_minor && installed_patch < required_patch)); then
    echo "❌ Go ${required_go_version} or newer is required; found ${installed_go_version}" >&2
    exit 1
fi

make -C "${SCRIPT_DIR}" install-all
