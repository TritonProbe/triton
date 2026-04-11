VERSION ?= dev
LDFLAGS = -s -w -X main.version=$(VERSION)

.PHONY: build build-all test test-race fmt lint docker release-snapshot smoke

build:
	go build -ldflags="$(LDFLAGS)" -o bin/triton ./cmd/triton

build-all:
	$$(if [ -d bin ] ; then true ; else mkdir -p bin ; fi)
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/triton-linux-amd64 ./cmd/triton
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/triton-windows-amd64.exe ./cmd/triton

test:
	go test ./...

test-race:
	CGO_ENABLED=1 go test -race ./...

fmt:
	go fmt ./...

lint:
	go vet ./...
	staticcheck ./...

docker:
	docker build -t triton:$(VERSION) .

release-snapshot:
	goreleaser release --snapshot --clean

smoke: build
	bash ./scripts/ci-smoke.sh
