#!/bin/bash

set -euo pipefail

go get -u -v mvdan.cc/sh/cmd/shfmt
go get -u -v github.com/golang/dep/cmd/dep
go get -u -v --tags extended github.com/gohugoio/hugo
