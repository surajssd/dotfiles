#!/bin/bash
#
# This script helps you to convert the YouTube video into mp3 file.

set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "${BASH_SOURCE[0]}")"/util.sh

url="$*"
if [[ -z "${url}" ]]; then
  err "error: Please provide the YouTube video URL"
  err ""
  err "Usage:"
  err ""
  err "mp3download.sh https://youtube.com/randomvideo"
  exit 1
fi

youtube-dl --extract-audio --audio-format mp3 "${url}"
