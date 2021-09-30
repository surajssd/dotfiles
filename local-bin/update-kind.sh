#!/usr/bin/env bash

set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "${BASH_SOURCE[0]}")"/util.sh

# create this directory which is in path in linux distros
# like fedora, centos and ubuntu.
mkdir -p ~/.local/bin/

# find what the latest version is
get_latest_release kubernetes-sigs/kind

cd $(mktemp -d)
curl -sLO "https://github.com/kubernetes-sigs/kind/releases/download/${version}/kind-linux-amd64"
chmod +x kind-linux-amd64
mv kind-linux-amd64 ~/.local/bin/kind

echo "Downloaded successfully in ~/.local/bin/"
