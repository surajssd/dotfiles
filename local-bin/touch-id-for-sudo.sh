#!/usr/bin/env bash

set -euo pipefail

PAM_FILE="/etc/pam.d/sudo"
PAM_TID_LINE="auth       sufficient     pam_tid.so"

if [[ "$(uname)" != "Darwin" ]]; then
    echo "❌ This script only works on macOS."
    exit 1
fi

if ! grep -q "pam_tid.so" "$PAM_FILE"; then
    echo "⏳ Adding Touch ID authentication to $PAM_FILE..."
    # Insert the pam_tid.so line after the first comment header line
    sudo sed -i '' '1 a\
'"$PAM_TID_LINE"'
' "$PAM_FILE"
    echo "✅ Touch ID for sudo enabled successfully."
else
    echo "ℹ️ Touch ID for sudo is already configured in $PAM_FILE."
fi
