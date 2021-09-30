#!/usr/bin/env bash

set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "${BASH_SOURCE[0]}")"/util.sh

# find what the latest version is
get_latest_release golangci/golangci-lint
echo "Downloading golagnci ${version}"
onlyversion=$(echo ${version} | cut -d'v' -f2)

# goto temp dir to download artifacts and then move it
cd /tmp || exit 1

# create this directory which is in path in linux distros
# like fedora, centos and ubuntu.
mkdir -p ~/.local/bin/
curl -OL https://github.com/golangci/golangci-lint/releases/download/${version}/golangci-lint-${onlyversion}-linux-amd64.tar.gz
tar -xvzf golangci-lint-*-linux-amd64.tar.gz
mv golangci-lint-*-linux-amd64/golangci-lint ~/.local/bin/
echo "Downloaded successfully in ~/.local/bin/"
