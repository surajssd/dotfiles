#!/usr/bin/env bash

set -euo pipefail

# If user doesn't provide the node name as an argument then exit
if [ $# -ne 1 ]; then
    echo "Usage:"
    echo "debug-node.sh <node-name>"
    exit 1
fi

kubectl debug "node/${1}" --image=quay.io/surajd/ubuntu:latest --profile=sysadmin -it -- chroot /host /bin/bash
