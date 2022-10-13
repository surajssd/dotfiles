#!/usr/bin/env bash

print_usage() {
  echo "Please provide the suffix name of the key"
  echo "e.g. ssh-key-gen.sh github personal"
  echo "e.g. ssh-key-gen.sh github work"
}

name=$1
if [ -z "${name}" ]; then
  print_usage
  exit 1
fi

email_type=$2
case $email_type in
  personal)
  email="surajd.service@gmail.com"
  ;;
  work)
  email="suraj.deshmukh@microsoft.com"
  ;;
  *)
  print_usage
  exit 1
  ;;
esac

set -euo pipefail

# Detect OS
os=$(uname)
case $os in
  Darwin)
  key_type="ed25519"
  ;;
  Linux)
  key_type="rsa"
  ;;
  *)
  echo "unsupported OS: ${os}"
  exit 1
esac

# The file name of the SSH key.
filename="${HOME}/.ssh/id_${key_type}.${name}.${email_type}"

# Generate new key
ssh-keygen -t "${key_type}" -b 8192 -f "${filename}" -N "" -C "${email}"

echo "Add the key in ${filename}.pub to https://github.com/settings/keys"
echo "After that run -> ssh -T git@github.com"
cat "${filename}.pub"
