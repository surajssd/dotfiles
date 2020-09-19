#!/bin/bash

set -euo pipefail

declare -a StringArray=(
  "surajd/fedora32-pgrep"
  "surajd/echoscript"
  "surajd/wrk2-export"
  "surajd/orchestrator"
  "surajd/jobrunner"
  "surajd/gotty"
  "surajd/fedora-networking"
  "surajd/kubeletinfo"
  "surajd/uidenv"
  "surajd/sleepnouid"
  "surajd/feduid"
  "surajd/bird"
  "surajd/l8e-ci-deployment-image"
)

for img in "${StringArray[@]}"; do
  echo "Processing image: ${img}"
  docker pull "${img}"
  docker rmi "${img}" || true
  echo
done


for img in "${StringArray[@]}"; do
  docker rmi -f "${img}"
done

# Remove all containers
# docker rm -f $(docker ps -aq)

# Remove all images
# docker rmi -f $(docker images -q)
