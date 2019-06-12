#!/bin/bash

name=$1
if [ -z "${name}" ]; then
  echo "Please provide the suffix name of the key"
  echo "e.g. ssh-key-gen.sh github"
  exit 1
fi

set -euo pipefail

# Generate new key
ssh-keygen -t rsa -b 8192 -f ~/.ssh/id_rsa."${name}" -N "" -C "surajd.service@gmail.com"

echo "Add the key in ~/.ssh/id_rsa.${name}.pub to https://github.com/settings/keys"
echo "After that run -> ssh -T git@github.com"
cat ~/.ssh/id_rsa."${name}".pub
