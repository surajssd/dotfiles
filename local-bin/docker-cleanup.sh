#!/usr/bin/env bash
# Cleans up the docker environment

set -euo pipefail

docker rm $(docker ps -a | grep Exited | awk '{print $1}') || true
docker rmi $(docker images | grep none | awk '{print $3}') || true
docker buildx prune -af
docker system prune -af
