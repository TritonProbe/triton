# Triton

**HTTP/3 (QUIC) Test Server & Benchmarking Platform**

*Three Prongs. One Binary. Every Packet.*

Triton is a pure Go toolkit for observing, testing, and benchmarking HTTP/3 and QUIC behavior. The project is designed around a single binary with three primary operating modes:

- `server`: runs protocol-aware test endpoints
- `probe`: inspects and measures remote targets
- `bench`: compares HTTP/1.1, HTTP/2, and HTTP/3 behavior

This repository is not yet the full RFC-complete final product described in the specification, but it already contains a working CLI, a test server, probe/bench flows, QUIC building blocks, and a minimal in-repo H3 loopback stack with handler dispatch.

## Vision

Triton is intended to become a documentation-first, comparison-driven QUIC and HTTP/3 laboratory with:

- a custom QUIC + HTTP/3 engine aligned with RFC 9000, RFC 9001, RFC 9114, and RFC 9204
- deep visibility into packets, frames, streams, timing, loss, and transport behavior
- embedded dashboard, API, benchmarking, and analysis surfaces in one binary
- educational and scriptable tooling for protocol research, performance testing, and debugging

Core principles from the specification:

- Protocol correctness
- Observable by default
- Comparison-driven output
- CLI-first automation
- Educational protocol visibility

## Three Modes

### 1. Server

Server mode exposes test and benchmark endpoints.

By default, `triton server` starts the HTTPS/TCP server on `:8443` and the dashboard on `127.0.0.1:9090`. The in-repo UDP H3 transport is experimental and now requires both `--listen` and `--allow-experimental-h3`.
Remote dashboard binding is blocked by default; use `--allow-remote-dashboard` only when you intentionally want non-loopback access, and pair it with dashboard auth.

If you want to work with the experimental Triton UDP H3 stack directly, prefer `triton lab` instead of mixing it into the normal `server` command.

Examples:

```bash
triton server
triton server --listen-tcp :8443 --dashboard-listen 127.0.0.1:9090
triton server --listen :4433 --allow-experimental-h3 --listen-tcp :8443
triton server --listen-h3 :4434 --listen-tcp :8443
triton lab
```

Main endpoints currently available:

- `GET /`
- `GET /ping`
- `GET /echo`
- `GET /download/:size`
- `POST /upload`
- `GET /delay/:ms`
- `GET /streams/:n`
- `GET /headers/:n`
- `GET /redirect/:n`
- `GET /status/:code`
- `GET /drip/:size/:delay`
- `GET /tls-info`
- `GET /quic-info`
- `GET /migration-test`
- `GET /.well-known/triton`

The capability document at `/.well-known/triton` now reflects the active runtime configuration, including real HTTP/3 availability, whether the experimental Triton UDP H3 path is enabled, and the current deployment/stability profile.

### 2. Probe

Probe mode measures and analyzes a target.

Examples:

```bash
triton probe --target https://example.com --format json
triton probe --target triton://loopback/ping --format json
triton probe --target h3://localhost:8443/ping --insecure --format json
```

Specification-level probe goals include:

- handshake timing
- 0-RTT resumption
- migration
- concurrent streams
- throughput
- latency percentiles
- MTU / ECN / retry / version negotiation
- TLS / ALPN / certificate analysis
- QPACK behavior
- congestion and loss behavior
- spin bit, GREASE, Alt-Svc, and H3 settings

### 3. Bench

Bench mode produces cross-protocol comparisons.

Examples:

```bash
triton bench --target https://example.com --duration 3s --concurrency 4
triton bench --target https://example.com --protocols h1,h2,h3 --insecure
triton bench --target triton://loopback/ping --protocols h3 --duration 2s
triton probe --target triton://127.0.0.1:4433/ping --format json
```

Specification-level benchmark goals include:

- TTFB
- connection setup time
- request throughput
- download and upload bandwidth
- stream concurrency impact
- head-of-line blocking comparison
- migration and loss resilience
- memory and resource profiles

Current implementation note:

- `h1` and `h2` bench runs work against normal HTTPS targets
- `h3` bench runs work against normal HTTPS targets using real HTTP/3 and also against `triton://...` targets
- `h3://host:port/path` probe targets use a real HTTP/3 client over QUIC
- `triton://host:port/path` targets use Triton's experimental UDP H3 transport

## Architecture

The specification organizes Triton into these major layers:

- UDP socket layer
- custom QUIC engine
- TLS 1.3 integration
- HTTP/3 frame layer
- analytics engine
- embedded dashboard
- REST API + CLI

What exists in this repository today:

- CLI command routing
- config loading and validation
- filesystem-backed result persistence
- HTTPS test server with self-signed certificate generation
- dashboard asset/API scaffold
- tested QUIC packet helpers
- tested QUIC frame parsing and serialization
- stream manager and connection state skeleton
- loopback QUIC listener/dialer
- minimal H3 `HEADERS + DATA` layer
- `http.Handler` dispatch over the in-repo H3 loopback path

## Current Implementation Status

### Implemented now

- `triton version`, `triton server`, `triton probe`, `triton bench`
- self-signed TLS certificate generation
- persisted probe and benchmark result storage under `triton-data/`
- dashboard status and result listing API
- HTTP test endpoints over the HTTPS/TCP fallback path
- QUIC varint, packet number, and header parsing
- QUIC frame support for:
  - `PADDING`
  - `PING`
  - `ACK` / `ACK_ECN`
  - `RESET_STREAM`
  - `STOP_SENDING`
  - `CRYPTO`
  - `NEW_TOKEN`
  - `STREAM`
  - `MAX_DATA`
  - `NEW_CONNECTION_ID`
  - `RETIRE_CONNECTION_ID`
  - `PATH_CHALLENGE`
  - `PATH_RESPONSE`
  - `HANDSHAKE_DONE`
- stream lifecycle, reassembly buffer, and manager
- connection state transitions and frame dispatch
- loopback QUIC handshake-like path
- loopback stream payload exchange
- minimal H3 request/response dispatch
- `triton://loopback/...` probe path using the in-process QUIC/H3 scaffold

### Not yet complete

- full RFC-complete QUIC transport
- crypto key schedule and packet protection
- real TLS-over-QUIC handshake
- QPACK encoder/decoder
- production-grade HTTP/3 server/client
- full analytics/qlog pipeline
- real network simulation and advanced benchmark runners
- ACME and advanced certificate automation
- dashboard real-time protocol visualization

## Quick Start

### Build

```bash
go build ./cmd/triton
```

### Run the server

```bash
go run ./cmd/triton server
go run ./cmd/triton server --listen :4433 --allow-experimental-h3
```

### Probe a public target

```bash
go run ./cmd/triton probe --target https://example.com --format json
```

### Probe the in-process loopback path

```bash
go run ./cmd/triton probe --target triton://loopback/ping --format json
```

### Run a benchmark

```bash
go run ./cmd/triton bench --target https://example.com --duration 3s --concurrency 4 --format json
```

## Command Summary

### Version

```bash
triton version
```

### Server

```bash
triton server [flags]
```

Important flags:

- `--config`
- `--listen` (experimental Triton UDP H3 listener)
- `--allow-experimental-h3`
- `--listen-h3` (real HTTP/3 listener via `quic-go`)
- `--listen-tcp`
- `--cert`
- `--key`
- `--dashboard`
- `--dashboard-listen`
- `--allow-remote-dashboard`
- `--dashboard-user`
- `--dashboard-pass`
- `--max-body-bytes`
- `--access-log`
- `--trace-dir`

### Lab

```bash
triton lab [flags]
```

`lab` runs the experimental Triton UDP H3 listener in isolation. It enables the experimental path, defaults the listener to `127.0.0.1:4433` when unset, and disables the normal HTTPS/dashboard surfaces so experimental work stays separated from the standard server profile.

### Probe

```bash
triton probe --target <url> [flags]
```

Important flags:

- `--config`
- `--target`
- `--format`
- `--timeout`
- `--insecure`
- `--trace-dir`

Probe target schemes:

- `https://...` uses the standard HTTP client path with HTTP/1.1 or HTTP/2 negotiation
- `h3://...` forces real HTTP/3 over QUIC
- `triton://...` uses Triton's in-repo experimental transport and typically expects `triton server --listen ... --allow-experimental-h3`

### Bench

```bash
triton bench --target <url> [flags]
```

Important flags:

- `--config`
- `--target`
- `--format`
- `--duration`
- `--concurrency`
- `--protocols`
- `--insecure`
- `--trace-dir`

## Example Endpoints

The current mux is shared across HTTPS server mode and minimal H3 loopback tests.

### Ping

```bash
curl -k https://localhost:8443/ping
```

### Echo

```bash
curl -k https://localhost:8443/echo -X POST -d "hello"
```

### Fixed status

```bash
curl -k https://localhost:8443/status/204 -i
```

### Download

```bash
curl -k https://localhost:8443/download/1024 --output /dev/null
```

## Configuration

An example configuration is available at [triton.yaml.example](/d:/Codebox/TritonProbe/triton.yaml.example).

The intended configuration model from the specification includes:

- server listen addresses
- TLS settings
- dashboard settings
- QUIC transport settings
- rate limiting and logging
- probe defaults
- benchmark defaults
- storage retention and result limits

Current precedence model:

- CLI flags
- environment variables
- config file
- defaults

Recognized environment variable examples:

- `TRITON_SERVER_LISTEN`
- `TRITON_SERVER_LISTEN_H3`
- `TRITON_SERVER_LISTEN_TCP`
- `TRITON_SERVER_TLS_CERT`
- `TRITON_SERVER_TLS_KEY`
- `TRITON_SERVER_DASHBOARD_USER`
- `TRITON_SERVER_DASHBOARD_PASS`
- `TRITON_SERVER_MAX_BODY_BYTES`
- `TRITON_SERVER_ACCESS_LOG`
- `TRITON_SERVER_TRACE_DIR`
- `TRITON_DASHBOARD_ENABLED`
- `TRITON_PROBE_TIMEOUT`
- `TRITON_PROBE_TRACE_DIR`
- `TRITON_BENCH_DEFAULT_DURATION`
- `TRITON_BENCH_INSECURE`
- `TRITON_BENCH_TRACE_DIR`

## Storage

Current filesystem storage uses gzip-compressed JSON under `triton-data/`.

Structure:

```text
triton-data/
├── benches/
├── certs/
└── probes/
```

Used today for:

- saved probe results
- saved benchmark results
- generated self-signed certificates

## Dashboard

The embedded dashboard is currently a lightweight scaffold that serves:

- static UI assets
- `/api/v1/status`
- `/api/v1/config`
- `/api/v1/probes`
- `/api/v1/probes/:id`
- `/api/v1/benches`
- `/api/v1/benches/:id`
- `/api/v1/traces`
- `/api/v1/traces/:name`

Current hardening features:

- optional HTTP Basic Auth via `server.dashboard_user` / `server.dashboard_pass`
- remote dashboard binding requires explicit opt-in via `server.allow_remote_dashboard` or `--allow-remote-dashboard`
- remote dashboard binding also requires dashboard auth credentials
- experimental UDP H3 requires explicit opt-in via `server.allow_experimental_h3` or `--allow-experimental-h3`
- defensive security headers on dashboard and HTTPS server responses
- bounded request body reads for `/echo` and `/upload`
- benchmark TLS verification enabled by default; `--insecure` is now opt-in
- request ID propagation and JSON access logs
- optional access log file output via `server.access_log` or `--access-log`
- qlog trace file output for real HTTP/3 connections via `server.trace_dir`
- client qlog trace output for real H3 probe and bench runs via `probe.trace_dir` / `bench.trace_dir`

Long-term specification goals include:

- real-time SSE updates
- connection timeline views
- protocol comparison charts
- packet/frame inspection
- benchmark visualization

## Project Layout

High-level current tree:

```text
cmd/triton/           CLI entrypoint
internal/appmux/      Shared Triton HTTP handlers
internal/bench/       Benchmark scaffolding
internal/cli/         CLI parsing/output
internal/config/      Config defaults and loading
internal/dashboard/   Embedded dashboard scaffold
internal/h3/          Minimal H3 layer and loopback service
internal/probe/       Probe orchestration
internal/quic/        QUIC transport, packet, frame, connection, stream, wire helpers
internal/server/      Server orchestration and certificate management
internal/storage/     Filesystem persistence
```

## Testing

The repository currently includes tests for:

- config validation
- storage save/load
- server endpoint behavior
- QUIC varint and packet number helpers
- QUIC header parsing
- QUIC frame parsing/serialization
- stream reassembly and manager behavior
- connection state transitions and frame dispatch
- loopback QUIC listener/dialer
- minimal H3 frame and header handling
- H3 handler dispatch
- Triton mux over H3 loopback

Run everything:

```bash
go test ./...
```

## Build And Distribution Goals

Specification targets:

- Linux `amd64`, `arm64`
- macOS `amd64`, `arm64`, universal
- Windows `amd64`, `arm64`
- single binary distribution
- Docker image
- future install script, `go install`, and package manager support

Current local build helpers:

- [Makefile](/d:/Codebox/TritonProbe/Makefile)
- [Dockerfile](/d:/Codebox/TritonProbe/Dockerfile)
- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`
- [.goreleaser.yml](/d:/Codebox/TritonProbe/.goreleaser.yml)

Current automation now includes:

- CI on pushes and pull requests for `gofmt`, `go test`, `go vet`, `staticcheck`, and binary build verification
- binary smoke verification covering `server`, `probe`, health endpoints, and H3 loopback bench
- tag-based release automation for `v*` tags via GoReleaser
- cross-platform archives for Linux, macOS, and Windows

## Roadmap

High-value next steps from the specification and the current code trajectory:

- expand QUIC packet coverage beyond the current scaffold
- add packet protection and TLS key schedule
- implement richer stream and connection behavior
- add real H3 control streams and SETTINGS
- add QPACK
- connect the loopback H3 service to more runtime paths
- add a local H3 runner into bench mode
- add deeper probe modes on top of the in-repo transport
- improve dashboard observability

## Repo Docs

Project planning and product definition live under `.project/`:

- [SPECIFICATION.md](/d:/Codebox/TritonProbe/.project/SPECIFICATION.md)
- [IMPLEMENTATION.md](/d:/Codebox/TritonProbe/.project/IMPLEMENTATION.md)
- [TASKS.md](/d:/Codebox/TritonProbe/.project/TASKS.md)
- [BRANDING.md](/d:/Codebox/TritonProbe/.project/BRANDING.md)

## Status Note

This repository is already more than a static scaffold, but it is still pre-v1 and mid-implementation relative to the full specification. This README intentionally reflects both:

- the intended final product described by the specification
- the concrete code that exists in this workspace today
