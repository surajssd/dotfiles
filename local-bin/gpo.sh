#!/usr/bin/env bash

set -euo pipefail
set -x

git push -u origin $(git branch --show-current) "$@"
