#!/usr/bin/env bash

files=$@
if [ -z "${files}" ]; then
    echo "Please provide the list of PDF files."
    echo "join-pdfs.sh 1.pdf 2.pdf"
    exit 1
fi

set -euo pipefail

tmpdir="$(mktemp -d)"
cp -r "${files}" "${tmpdir}"

cd "${tmpdir}"
combined_name="combined-pdf-$(date.sh).pdf"
pdfunite "${files}" "${combined_name}"
