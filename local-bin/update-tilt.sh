#!/bin/bash

set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "${BASH_SOURCE[0]}")"/util.sh

# create this directory which is in path in linux distros
# like fedora, centos and ubuntu.
mkdir -p ~/.local/bin/

# find what the latest version is
get_latest_release tilt-dev/tilt
onlyversion=$(echo "${version}" | cut -d'v' -f2)

cd $(mktemp -d)
curl -fsSL "https://github.com/tilt-dev/tilt/releases/download/${version}/tilt.${onlyversion}.linux.x86_64.tar.gz" | tar -xzv tilt
mv tilt ~/.local/bin/

echo "Downloaded successfully in ~/.local/bin/"
