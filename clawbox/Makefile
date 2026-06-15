.DEFAULT_GOAL := test
.PHONY: test lint build install

test:
	go test ./... -cover

lint:
	go vet ./...
	@echo "Checking gofmt..."
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

build:
	go build .

install:
	go install .
