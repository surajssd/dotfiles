#!/usr/bin/env bash

set -x

#===================================================================
# add fastest mirror
cat /etc/dnf/dnf.conf | grep 'fastestmirror'
if [ $? -ne 0 ]; then
    echo 'fastestmirror=true
deltarpm=true
' | sudo tee -a /etc/dnf/dnf.conf

fi

