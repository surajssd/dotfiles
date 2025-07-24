#!/usr/bin/env bash
# generates random password for you

set -euo pipefail

# Get the length of the password from the first argument or default to 20
LENGTH=${1:-20}

password=$(tr </dev/urandom -dc 'A-Za-z0-9!@#$%&*_-' | head -c "${LENGTH}" || true)
echo -n "${password}" | pbcopy
echo "Generated password: ${password}"
echo "✅ Password also copied to clipboard!"
