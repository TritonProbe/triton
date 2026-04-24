# Triton

**HTTP/3 (QUIC) Test Server & Benchmarking Platform**

*Three Prongs. One Binary. Every Packet.*

Triton is a pure Go toolkit for observing, testing, and benchmarking HTTP/3 and QUIC behavior. The project is designed around a single binary with four primary operating modes:

- `server`: runs protocol-aware test endpoints
- `probe`: inspects and measures remote targets
- `bench`: compares HTTP/1.1, HTTP/2, and HTTP/3 behavior
- `check`: runs reusable profile-based verification flows for CI and recurring checks

This repository is not yet the full RFC-complete final product described in the specification, but it already contains a working CLI, a test server, probe/bench flows, QUIC building blocks, and a minimal in-repo H3 loopback stack with handler dispatch.
Current product positioning is pragmatic: real HTTP diagnostics and real HTTP/3 behavior are supported through `quic-go`, while the in-repo custom QUIC/H3 stack remains lab-only research.

## Supported Today

Canonical reference: [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md)

Lab-only transport reference: [EXPERIMENTAL.md](/d:/Codebox/TritonProbe/EXPERIMENTAL.md)

If any section below conflicts with current running behavior, prefer [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md) first, then [ARCHITECTURE.md](/d:/Codebox/TritonProbe/ARCHITECTURE.md), then the code.

Treat the following as the implementation truth for this repository today:

- Supported server path:
  - HTTPS/TCP test server
  - optional real HTTP/3 listener via `quic-go`
  - optional embedded dashboard
- Supported client paths:
  - `https://...` probe and bench targets
  - `h3://...` probe targets using real HTTP/3 via `quic-go`
  - `https://...` + `h3` bench runs using real HTTP/3 via `quic-go`
- Lab-only path:
  - `triton://...` experimental in-repo UDP H3 transport
  - `triton lab`
  - `internal/quic/*` and `internal/h3/*`

Important caveat for probe output:

- Basic checks such as handshake timing, TLS metadata, latency, throughput, and stream sampling are implemented directly.
- Several advanced probe dimensions are still heuristic or contract-based rather than packet-level QUIC truth. That includes `0rtt`, `migration`, `qpack`, `loss`, `congestion`, `retry`, `version`, `ecn`, and `spin-bit`.
- If you need packet-level QUIC validation, do not treat those advanced fields as RFC-grade transport telemetry yet.

## Vision

This section is intentionally future-looking. It describes target-state goals rather than a promise that every item below already exists in the shipped product surface.

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

## Four Modes

### 1. Server

Server mode exposes test and benchmark endpoints.

By default, `triton server` starts the HTTPS/TCP server on `:8443` and the dashboard on `127.0.0.1:9090`. The in-repo UDP H3 transport is experimental and now requires both `--listen` and `--allow-experimental-h3`.
Experimental UDP H3 is also loopback-only by default; binding it on a non-loopback interface now additionally requires `--allow-remote-experimental-h3` or `server.allow_remote_experimental_h3: true`.
Running real HTTP/3 (`--listen-h3`) and experimental UDP H3 (`--listen`) together now requires explicit mixed-plane opt-in via `--allow-mixed-h3-planes` or `server.allow_mixed_h3_planes: true`.
Remote dashboard binding is blocked by default; use `--allow-remote-dashboard` only when you intentionally want non-loopback access, and pair it with dashboard auth.
When remote dashboard access is enabled, provide explicit `--cert` and `--key`; runtime-generated self-signed fallback is no longer accepted for that mode.

If you want to work with the experimental Triton UDP H3 stack directly, prefer `triton lab` instead of mixing it into the normal `server` command.
For the explicit research-only boundary around that surface, see [EXPERIMENTAL.md](/d:/Codebox/TritonProbe/EXPERIMENTAL.md).

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

The capability document at `/.well-known/triton` now reflects the active runtime configuration, including real HTTP/3 availability, whether the experimental Triton UDP H3 path is enabled, the current deployment/stability profile, and build metadata.

### 2. Probe

Probe mode measures and analyzes a target.

Examples:

```bash
triton probe --target https://example.com --format json
triton probe --target triton://loopback/ping --format json
triton probe --target h3://localhost:8443/ping --insecure --allow-insecure-tls --format json
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

Current probe output includes richer `analysis` sections for response throughput, sampled latency percentiles, stream-concurrency summaries, and a `test_plan` plus `support` matrix that explicitly marks advanced checks as `full`, `partial`, or `unavailable`. The advanced fields are intentionally caveated: the current `qpack` value is an estimated header-block-size approximation, current `loss` and `congestion` values are inferred from repeated request failures and latency spread, current `version` and `retry` values are inferred from observed H3 protocol/alpn and handshake visibility, current `ecn` and `spin-bit` values are inferred from observable protocol metadata and sampled RTT stability, current `0rtt` is resumption timing rather than true early-data proof, and current `migration` is an endpoint-capability check rather than live path rebinding. The deeper spec items such as true 0-RTT, live migration, packet-level loss analysis, congestion-window telemetry, Retry packet observation, QUIC version negotiation telemetry, packet-mark ECN visibility, packet-level spin-bit observation, and real QPACK inspection are still not fully implemented.

### 3. Bench

Bench mode produces cross-protocol comparisons.

Examples:

```bash
triton bench --target https://example.com --duration 3s --concurrency 4
triton bench --target https://example.com --protocols h1,h2,h3 --insecure --allow-insecure-tls
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
- bench output now includes sampled latency percentiles (`p50`, `p95`, `p99`), error-rate/error-summary data, and average request phases such as connect, TLS, first-byte, and transfer time
- bench results now also include a computed `summary` rollup that classifies protocols as healthy/degraded/failed and highlights the best and riskiest protocol in the run

### 4. Check

Check mode turns probe and bench into a reusable verification workflow.

Use it when you want one command that:

- loads named probe and bench profiles from config
- runs one or both checks against the same target family
- emits a combined verdict for CI or recurring operational gates
- optionally writes a combined report file

Examples:

```bash
triton check --profile production-edge
triton check --probe-profile production-edge --bench-profile staging-api --report-out reports/check.md
triton check --profile production-edge --summary-out reports/check-summary.json --junit-out reports/check-junit.xml
triton check --config triton.yaml --target https://example.com
```

Current implementation note:

- `check` reuses the same underlying `probe` and `bench` engines
- threshold failures still persist results, but the command exits non-zero
- shared profile names let one command resolve both `probe_profiles.NAME` and `bench_profiles.NAME`

## Architecture

When there is a conflict between the long-term architecture vision below and the running code, prefer [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md), then [ARCHITECTURE.md](/d:/Codebox/TritonProbe/ARCHITECTURE.md), then the generated audit docs under `.project/`.

The list immediately below is target-state architecture context from the specification, not a claim that all layers are complete in the current supported runtime.

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

- `triton version`, `triton server`, `triton probe`, `triton bench`, `triton check`
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

### Explicitly partial today

- `0rtt`: resumption timing and support signaling, not true 0-RTT early-data validation
- `migration`: endpoint contract probing, not live path migration
- `qpack`: estimated header-block-size analysis, not dynamic-table inspection
- `loss`: request-error and timeout signal approximation, not packet-loss telemetry
- `congestion`: latency-spread approximation, not congestion-window telemetry
- `retry`, `version`, `ecn`, `spin-bit`: observational approximations rather than packet-level transport visibility

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

### Run a reusable verification check

```bash
go run ./cmd/triton check --profile production-edge
go run ./cmd/triton check --profile production-edge --report-out reports/check.md
go run ./cmd/triton check --profile production-edge --summary-out reports/check-summary.json --junit-out reports/check-junit.xml
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
- `--allow-remote-experimental-h3`
- `--allow-mixed-h3-planes`
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
- `--tests`
- `--full`
- `--0rtt`
- `--migration`
- `--timeout`
- `--streams`
- `--insecure`
- `--allow-insecure-tls`
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
- `--allow-insecure-tls`
- `--trace-dir`
- `--profile`
- `--report-out`
- `--report-format`
- threshold flags such as `--threshold-max-error-rate`

### Check

```bash
triton check [flags]
```

Important flags:

- `--config`
- `--profile`
- `--probe-profile`
- `--bench-profile`
- `--target`
- `--format`
- `--report-out`
- `--report-format`
- `--summary-out`
- `--junit-out`
- `--skip-probe`
- `--skip-bench`

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
- `TRITON_PROBE_ALLOW_INSECURE_TLS`
- `TRITON_PROBE_DEFAULT_STREAMS`
- `TRITON_PROBE_TRACE_DIR`
- `TRITON_BENCH_DEFAULT_DURATION`
- `TRITON_BENCH_INSECURE`
- `TRITON_BENCH_ALLOW_INSECURE_TLS`
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

The UI now renders status/config snapshots plus typed summaries for recent probes, benches, and trace files instead of showing only raw JSON blobs, including a top-level overview panel, build/version status context, probe test-plan/skipped-test hints, `0rtt` / `migration` probe summary hints, support-coverage pills, richer benchmark summary pills plus bench health rollups, and selected trace detail with preview and raw/meta links.
The dashboard also supports in-page filtering, sorting, offset-based pagination, and result limits for probe, bench, and trace lists via query-driven API calls.
List endpoints (`/api/v1/probes`, `/api/v1/benches`, `/api/v1/traces`) now accept `q`, `sort`, `limit`, and `offset` query parameters, and probe/bench list endpoints also support `view=summary` to omit heavier raw fields while keeping typed dashboard summaries.
It now includes a compare/trend panel that contrasts recent probe coverage and bench health/best-protocol latency across the latest runs.

Current hardening features:

- optional HTTP Basic Auth via `server.dashboard_user` / `server.dashboard_pass`
- remote dashboard binding requires explicit opt-in via `server.allow_remote_dashboard` or `--allow-remote-dashboard`
- remote dashboard binding also requires dashboard auth credentials
- remote dashboard binding also requires explicit `server.cert` and `server.key`
- experimental UDP H3 requires explicit opt-in via `server.allow_experimental_h3` or `--allow-experimental-h3`
- non-loopback experimental UDP H3 binding additionally requires `server.allow_remote_experimental_h3` or `--allow-remote-experimental-h3`
- enabling both `server.listen` (experimental UDP H3) and `server.listen_h3` (real HTTP/3) requires `server.allow_mixed_h3_planes` or `--allow-mixed-h3-planes`
- defensive security headers on dashboard and HTTPS server responses
- bounded request body reads for `/echo` and `/upload`
- benchmark TLS verification enabled by default; `--insecure` is now opt-in
- `probe.insecure` / `bench.insecure` now also require explicit `allow_insecure_tls` opt-in for lab use
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

Coverage note:

- generated coverage artifacts such as `coverage`, `coverage_review`, and `coverage.out` are local-only verification outputs and should not be committed

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
- a dedicated CI `race` job that runs `go test -race ./...` on a CGO-capable Linux runner
- a dedicated CI `gosec` pass for repo-wide security scanning
- binary smoke verification covering `server`, `probe`, health endpoints, and H3 loopback bench
- tag-based release automation for `v*` tags via GoReleaser
- cross-platform archives for Linux, macOS, and Windows

Container runtime notes:

- the image now runs as non-root user `10001`
- default command is `triton server`
- runtime state is expected under `/var/lib/triton`
- exposed ports cover supported HTTPS/TCP (`8443/tcp`), supported real HTTP/3 (`4434/udp`), and dashboard (`9090/tcp`)
- see [OPERATIONS.md](/d:/Codebox/TritonProbe/OPERATIONS.md) for persistence, cert, and remote-exposure guidance

Developer verification helpers:

- `make clean`
- `make test`
- `make test-race`
- `make test-fuzz`
- `make perf-check`
- `make check-guard`
- `make lint`
- `make security`
- `pwsh -File ./scripts/ci-local.ps1` for local CI-style verification on Windows/PowerShell
- `pwsh -File ./scripts/ci-local.ps1 -Race` to include race tests when a CGO toolchain is available
- `bash ./scripts/ci-smoke.sh` on bash-capable environments
- `pwsh -File ./scripts/ci-smoke.ps1` on Windows/PowerShell
- `bash ./scripts/ci-check-guard.sh` for the combined `triton check` CI gate
- `pwsh -File ./scripts/ci-check-guard.ps1` on Windows/PowerShell

Repository maintenance helper:

- GitHub maintenance is handled directly through standard repo workflows and CI scripts

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

- [CONTRIBUTING.md](/d:/Codebox/TritonProbe/CONTRIBUTING.md)
- [API.md](/d:/Codebox/TritonProbe/API.md)
- [CONFIG.md](/d:/Codebox/TritonProbe/CONFIG.md)
- [OPERATIONS.md](/d:/Codebox/TritonProbe/OPERATIONS.md)
- [TROUBLESHOOTING.md](/d:/Codebox/TritonProbe/TROUBLESHOOTING.md)
- [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md)
- [PRODUCTIONREADY.md](/d:/Codebox/TritonProbe/.project/PRODUCTIONREADY.md)
- [ROADMAP.md](/d:/Codebox/TritonProbe/.project/ROADMAP.md)
- [SPECIFICATION.md](/d:/Codebox/TritonProbe/.project/SPECIFICATION.md)
- [IMPLEMENTATION.md](/d:/Codebox/TritonProbe/.project/IMPLEMENTATION.md)
- [TASKS.md](/d:/Codebox/TritonProbe/.project/TASKS.md)
- [ENGINE_STRATEGY.md](/d:/Codebox/TritonProbe/.project/ENGINE_STRATEGY.md)

## Status Note

For current-state truth, start with [SUPPORTED.md](/d:/Codebox/TritonProbe/SUPPORTED.md).

This repository is already more than a static scaffold, but it is still pre-v1 and mid-implementation relative to the full specification. The docs intentionally separate:

- the supported product path that exists today
- the future-looking target state described in the specification
