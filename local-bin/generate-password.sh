#!/usr/bin/env bash
# generates random password for you

# Check if pass is installed.
which pass > /dev/null 2>&1
if [ $? -ne 0 ]; then
    echo "Install 'pass': https://www.passwordstore.org."
    exit 1
fi

password=$(yes | pass generate foo 50 | tail -1)
echo "${password}"
