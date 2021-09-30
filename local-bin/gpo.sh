#!/usr/bin/env bash

set -euo pipefail
set -x

git push origin $(git branch --show-current) "$@"
