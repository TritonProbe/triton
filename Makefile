VERSION ?= dev
LDFLAGS = -s -w -X main.version=$(VERSION)

.PHONY: build build-all clean test test-race test-fuzz perf-check check-guard fmt lint security docker release-snapshot smoke

build:
	go build -ldflags="$(LDFLAGS)" -o bin/triton ./cmd/triton

build-all:
	$$(if [ -d bin ] ; then true ; else mkdir -p bin ; fi)
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/triton-linux-amd64 ./cmd/triton
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/triton-windows-amd64.exe ./cmd/triton

clean:
	rm -rf bin triton triton.exe coverage.out triton-data

test:
	go test ./...

test-race:
	CGO_ENABLED=1 go test -race ./...

test-fuzz:
	go test ./internal/quic/packet ./internal/quic/frame ./internal/quic/wire ./internal/h3/frame -run Fuzz -count=1

perf-check: build
	bash ./scripts/ci-bench-guard.sh

check-guard: build
	bash ./scripts/ci-check-guard.sh

fmt:
	go fmt ./...

lint:
	go vet ./...
	staticcheck ./...

security:
	gosec ./...

docker:
	docker build -t triton:$(VERSION) .

release-snapshot:
	goreleaser release --snapshot --clean

smoke: build
	bash ./scripts/ci-smoke.sh
