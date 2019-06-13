#!/bin/bash
# This script gives you custom date format like e.g.
#
# $ date.sh
# 2019-06-13-18-45-40

set -euo pipefail

echo $(date '+%Y-%m-%d-%H-%M-%S')
