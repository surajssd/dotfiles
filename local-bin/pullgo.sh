#!/bin/bash

set -euo pipefail

# check if git is installed, if not install it
if ! [[ -x "$(command -v git)" ]]; then
  echo "Installing git..."
  sudo dnf -y install git
fi

# referenced from https://github.com/udhos/update-golang/blob/master/update-golang.sh
function get_latest_release() {
  version=$(curl --silent https://storage.googleapis.com/golang |
    grep -E -o 'go[0-9\.]+' |
    grep -E -o '[0-9]\.[0-9]+(\.[0-9]+)?' |
    sort -V |
    uniq |
    tail -1)
}

# find what the latest version is
get_latest_release
echo "Latest golang ${version}"

# if latest is already installed no need to download again
if ls /usr/local/go >/dev/null 2>&1; then
  if [[ $(go version) == *"${version}"* ]]; then
    echo "Latest $(go version) already installed!"
    exit 0
  fi
fi

# download it in random temporary location
rand=$RANDOM
path=/tmp/goinstall-$rand
mkdir -p $path
cd $path
file=go$version.linux-amd64.tar.gz
url=https://dl.google.com/go/$file

echo "Downloading from $url"
if ! curl --silent -LO "${url}"; then
  echo "Go downloading failed!"
  exit 1
fi

# if already go is present uninstall it
if ls /usr/local/go >/dev/null 2>&1; then
  echo "Uninstalling $(go version)"
  sudo rm -rf /usr/local/go
fi

echo "Installing new $version"
if ! sudo tar -C /usr/local -xzf "${file}"; then
  echo "Go installing failed!"
  exit 1
fi

sudo ln -sf /usr/local/go/bin/go /usr/local/sbin/go
