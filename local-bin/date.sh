#!/bin/bash
# This script gives you custom date format like e.g.
#
# $ date.sh
# 2019-06-13-18-45-40

set -euo pipefail

# shellcheck disable=SC2005
echo "$(date '+%Y-%m-%d-%H-%M-%S')"
