#!/bin/bash
#
# Start monitoring the node with node-exporter
# And display the information as dashboard

set -euo pipefail

docker run -d \
  --net="host" \
  --pid="host" \
  --restart="always" \
  -v "/:/host:ro,rslave" \
  quay.io/prometheus/node-exporter \
  --path.rootfs=/host \
  --web.listen-address="127.0.0.1:9100"

mkdir -p /tmp/prom
# TODO: fix this URL
curl -o /tmp/prom/prometheus-config.yaml https://raw.githubusercontent.com/surajssd/dotfiles/master/staticfiles/prometheus/prometheus-config.yaml

docker run -d \
  --net="host" \
  -v "/tmp/prom/:/prom:ro" \
  prom/prometheus \
  --web.listen-address="127.0.0.1:9090" \
  --config.file=/prom/prometheus-config.yaml

# TODO: Add grafana
