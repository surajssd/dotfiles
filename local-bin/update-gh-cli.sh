#!/usr/bin/env bash

set -euo pipefail

# Detect OS
os=$(uname)
case $os in
    Darwin)
    brew install gh
    exit 0
    ;;

    Linux)
    break
    ;;

    *)
    echo "unsupported OS: ${os}"
    exit 1
esac

# shellcheck source=/dev/null
source "$(dirname "${BASH_SOURCE[0]}")"/util.sh

# create this directory which is in path in linux distros
# like fedora, centos and ubuntu.
mkdir -p ~/.local/bin/

# find what the latest version is
get_latest_release cli/cli
onlyversion=$(echo "${version}" | cut -d'v' -f2)

# goto temp dir to download artifacts and then move it
cd $(mktemp -d)

echo "Downloading gh ${version}"
curl --silent -LO https://github.com/cli/cli/releases/download/"${version}"/gh_"${onlyversion}"_linux_amd64.tar.gz && \
  tar -xzf gh_"${onlyversion}"_linux_amd64.tar.gz && \
  mv gh_"${onlyversion}"_linux_amd64/bin/gh ~/.local/bin

echo "Downloaded successfully in ~/.local/bin/"
