VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.Version=$(VERSION)

.PHONY: test build vet tidy run-companion run-relay

test:
	go test ./...

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/companion ./cmd/companion
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/relay ./cmd/relay

vet:
	go vet ./...

tidy:
	go mod tidy

run-companion:
	go run ./cmd/companion

run-relay:
	go run ./cmd/relay
