#!/usr/bin/env bash
# generates random password for you

password=$(yes | pass generate foo 50 | tail -1)
echo "${password}"
