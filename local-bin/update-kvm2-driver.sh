#!/bin/bash

set -euo pipefail
set -x

cd /tmp
curl -LO https://storage.googleapis.com/minikube/releases/latest/docker-machine-driver-kvm2
install docker-machine-driver-kvm2 "${HOME}"/.local/bin
