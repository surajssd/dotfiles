#!/bin/bash

set -euo pipefail


echo "var allowPaste = function(e){
  e.stopImmediatePropagation();
  return true;
};
document.addEventListener('paste', allowPaste, true);" | xclip -select c

echo "The code has been copied into the clip board."
echo "Now goto chrome and press Ctrl + Shift + I and paste into the console window."
