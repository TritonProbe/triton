VERSION ?= dev
LDFLAGS = -s -w -X main.version=$(VERSION)

.PHONY: build test fmt

build:
	go build -ldflags="$(LDFLAGS)" -o bin/triton ./cmd/triton

test:
	go test ./...

fmt:
	go fmt ./...
