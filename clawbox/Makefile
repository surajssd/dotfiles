.PHONY: test
test:
	go test ./... -cover

.PHONY: lint
lint:
	go vet ./...
	@echo "Checking gofmt..."
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)

.PHONY: build
build:
	go build .

.PHONY: install
install:
	go install .
