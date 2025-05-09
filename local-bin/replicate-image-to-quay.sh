#!/usr/bin/env bash
# mirrors an image from one registry to quay

IMAGE_NAME=$1
: "${IMAGE_NAME:?Usage: ${BASH_SOURCE[0]} <image_name>}"

set -euo pipefail

QUAY_REPO="quay.io/surajd"

# An image could be coming from some other registry like bar.io/foo/baz, so remove the bar.io
DOMAIN_LESS_IMAGE_NAME="${IMAGE_NAME#*/}"
QUAY_IMAGE_NAME="${QUAY_REPO}/${DOMAIN_LESS_IMAGE_NAME}"

docker pull "${IMAGE_NAME}"
docker tag "${IMAGE_NAME}" "${QUAY_IMAGE_NAME}"
docker push "${QUAY_IMAGE_NAME}"
