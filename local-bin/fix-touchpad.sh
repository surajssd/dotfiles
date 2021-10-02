#!/usr/bin/env bash
#
# This is used to fix the touchpad as mentioned here:
# https://askubuntu.com/q/1091635/246019

machine=$(cat /sys/devices/virtual/dmi/id/chassis_vendor 2>/dev/null)
if [[ "${machine}" != "LENOVO" ]]; then
  echo "Not a Lenovo Thinkpad. Cannot execute."
  exit 1
fi

set -euo pipefail

sudo sh -c 'echo -n "elantech"> /sys/bus/serio/devices/serio1/protocol'
