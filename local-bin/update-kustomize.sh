#!/usr/bin/env bash

set -euo pipefail

curl -s "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh" | sh

mv kustomize ~/.local/bin/
echo "Downloaded successfully in ~/.local/bin/"
