#!/usr/bin/env bash

set -euo pipefail

# Detect OS
os=$(uname)
case $os in
Darwin)
  command="pbcopy"
  ;;
Linux)
  command="xclip -select c"
  ;;
*)
  echo "unsupported OS: ${os}"
  exit 1
  ;;
esac

echo "var allowPaste = function(e){
  e.stopImmediatePropagation();
  return true;
};
document.addEventListener('paste', allowPaste, true);" | "${command}"

echo "The code has been copied into the clip board."
echo "Now goto chrome and press Ctrl + Shift + I and paste into the console window."
