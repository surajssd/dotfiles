#!/usr/bin/env bash

cat /etc/dnf/dnf.conf > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "Not a Fedora machine. Cannot execute."
    exit 1
fi

set -euo pipefail
#===================================================================
# add fastest mirror
cat /etc/dnf/dnf.conf | grep 'fastestmirror'
if [ $? -ne 0 ]; then
    echo 'fastestmirror=true
deltarpm=true
' | sudo tee -a /etc/dnf/dnf.conf

fi

