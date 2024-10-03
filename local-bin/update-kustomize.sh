#!/usr/bin/env bash

set -euo pipefail

pushd $(mktemp -d)
curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | bash

mv kustomize ~/.local/bin/
echo "Downloaded successfully in ~/.local/bin/"
popd
