#!/usr/bin/env bash

set -euo pipefail

if [ $# -eq 0 ]; then
    echo "❌ Please provide the list of PDF files."
    echo "join-pdfs.sh 1.pdf 2.pdf"
    exit 1
fi

# Convert all file paths to absolute paths
abs_files=()
for f in "$@"; do
    if [ ! -f "$f" ]; then
        echo "❌ File not found: $f"
        exit 1
    fi
    abs_files+=("$(realpath "$f")")
done

output_dir="$(pwd)"
combined_name="combined-pdf-$(ssd date).pdf"

pdfunite "${abs_files[@]}" "${output_dir}/${combined_name}"
echo "✅ Combined PDF created: ${output_dir}/${combined_name}"
