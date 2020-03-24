#!/bin/bash

set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "${BASH_SOURCE[0]}")"/util.sh

os=$(go env GOOS)
arch=$(go env GOARCH)

# find what the latest version is
get_latest_release kubernetes-sigs/kubebuilder
echo "Downloading kubebuilder ${version}"
onlyversion=$(echo "${version}" | cut -d'v' -f2)

# download kubebuilder and extract it to tmp
curl -L "https://go.kubebuilder.io/dl/${onlyversion}/${os}/${arch}" | tar -xz -C /tmp/

# move to a long-term location and put it on your path
# (you'll need to set the KUBEBUILDER_ASSETS env var if you put it somewhere else)
mv "/tmp/kubebuilder_${onlyversion}_${os}_${arch}/bin/kubebuilder" ~/.local/bin/
echo "Downloaded successfully in ~/.local/bin/"
