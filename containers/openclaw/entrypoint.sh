#!/usr/bin/env bash

set -euo pipefail

# entrypoint.sh — Fix "I have no name!" when the container runs with a
# UID override (e.g. --uid 501) that doesn't exist in /etc/passwd.
#
# All tool directories (Homebrew, uv, bun, pip, home) are made
# world-writable at build time, so no chown is needed at runtime.
# This script only adds a passwd entry for the runtime UID.

RUNTIME_UID="$(id -u)"
RUNTIME_GID="$(id -g)"

# Add a passwd entry if the runtime UID is unknown.
if ! id -un &>/dev/null 2>&1; then
    echo "node:x:${RUNTIME_UID}:${RUNTIME_GID}::/home/node:/bin/bash" >>/etc/passwd
fi

# Seed bind-mounted directories from image seed copies on first run.
# When these dirs are bind-mounted from the host, they start empty.
# The cp may warn about preserving timestamps on VirtioFS mount roots,
# which is harmless and can be ignored.
seed_dir_if_needed() {
    local seed_dir="${1}"
    local target_dir="${2}"
    local marker="${target_dir}/.seed-initialized"

    if [[ -d "${seed_dir}" ]] && [[ ! -f "${marker}" ]]; then
        echo "entrypoint: seeding ${target_dir} from image..."
        cp -a "${seed_dir}"/. "${target_dir}/" 2>/dev/null || true
        touch "${marker}"
    fi
}

seed_dir_if_needed "/opt/node-home-seed" "/home/node"
seed_dir_if_needed "/opt/linuxbrew-seed" "/home/linuxbrew"

exec "$@"
