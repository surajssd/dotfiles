#!/usr/bin/env bash

set -euo pipefail

# Detect OS
os=$(uname)
case $os in
Darwin)
  brew install hugo
  exit 0
  ;;

Linux)
  # no op
  ;;

*)
  echo "unsupported OS: ${os}"
  exit 1
  ;;
esac

# shellcheck source=/dev/null
source "$(dirname "${BASH_SOURCE[0]}")"/util.sh

# create this directory which is in path in linux distros
# like fedora, centos and ubuntu.
mkdir -p ~/.local/bin/

# find what the latest version is
get_latest_release gohugoio/hugo
# shellcheck disable=SC2154
onlyversion=$(echo "${version}" | cut -d'v' -f2)

# goto temp dir to download artifacts and then move it
cd "$(mktemp -d)"

echo "Downloading hugo ${version}"
curl --silent -LO https://github.com/gohugoio/hugo/releases/download/"${version}"/hugo_extended_"${onlyversion}"_Linux-64bit.tar.gz &&
  tar -xvzf hugo_extended_"${onlyversion}"_Linux-64bit.tar.gz &&
  mv hugo ~/.local/bin

echo "Downloaded successfully in ~/.local/bin/"
