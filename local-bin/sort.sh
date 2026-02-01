#!/bin/bash
# Reads a text file, converts to lowercase, sorts, and keeps unique lines

set -euo pipefail

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 <input_file>"
    echo "  Converts file content to lowercase, sorts, and keeps unique lines (in-place)"
    exit 1
fi

input_file="$1"

if [[ ! -f "$input_file" ]]; then
    echo "Error: File '$input_file' not found"
    exit 1
fi

result=$(tr '[:upper:]' '[:lower:]' <"$input_file" | sort -u)

# Write to temp file first, then move (atomic operation)
temp_file="${input_file}.tmp"
echo "$result" >"$temp_file"
mv "$temp_file" "$input_file"
echo "File '$input_file' updated"
