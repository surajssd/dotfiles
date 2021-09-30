#!/usr/bin/env bash

set -euo pipefail

# Detect OS
os=$(uname)
case $os in
    Darwin)
    brew install kubectl
    ;;

    Linux)
    cd $(mktemp -d)
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
    chmod +x ./kubectl
    mv ./kubectl ~/.local/bin/
    ;;

    *)
    echo "unsupported OS: ${os}"
    exit 1
esac

