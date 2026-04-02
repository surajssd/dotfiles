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

exec "$@"
