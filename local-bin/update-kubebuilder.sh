#!/usr/bin/env bash

set -euo pipefail

cd $(mktemp -d)
# download kubebuilder and install locally.
curl -sL -o kubebuilder https://go.kubebuilder.io/dl/latest/$(go env GOOS)/$(go env GOARCH)
chmod +x kubebuilder && mv kubebuilder ~/.local/bin/

echo "Downloaded successfully in ~/.local/bin/"
