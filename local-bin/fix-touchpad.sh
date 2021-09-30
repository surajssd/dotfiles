#!/usr/bin/env bash
#
# This is used to fix the touchpad as mentioned here:
# https://askubuntu.com/q/1091635/246019

set -euo pipefail

sudo sh -c 'echo -n "elantech"> /sys/bus/serio/devices/serio1/protocol'
