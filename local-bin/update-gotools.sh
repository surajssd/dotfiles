#!/usr/bin/env bash

set -euo pipefail

go get -u -v mvdan.cc/sh/cmd/shfmt
go get -u -v github.com/golang/dep/cmd/dep


mkdir -p $HOME/git/hugo
cd $HOME/git
git clone https://github.com/gohugoio/hugo.git hugo
cd hugo
git pull --ff origin master
go install --tags extended

