#!/bin/bash

set -euo pipefail

# referenced from https://gist.github.com/lukechilds/a83e1d7127b78fef38c2914c4ececc3c
function get_latest_release() {
  version=$(curl --silent "https://api.github.com/repos/$1/releases/latest" |
    grep '"tag_name":' |
    sed -E 's/.*"([^"]+)".*/\1/')
}

# find what the latest version is
get_latest_release golangci/golangci-lint
echo "Downloading minikube ${version}"
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
