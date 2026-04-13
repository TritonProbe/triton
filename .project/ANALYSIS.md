# Project Analysis Report

> Auto-generated comprehensive analysis of Triton
> Generated: 2026-04-12
> Analyzer: Codex - Full Codebase Audit

## 1. Executive Summary

Triton is a Go-based single-binary network diagnostics tool aimed at HTTP/3 and QUIC testing, with three user-facing modes: `server`, `probe`, and `bench`. The current repository is no longer just a scaffold: it contains a working CLI, a TLS test server, a local dashboard, gzip-backed result persistence, a real HTTP/3 client/server path built on `quic-go`, and a second experimental in-repo UDP/H3 stack used for loopback and `triton://` targets. The codebase is meaningfully test-covered and buildable, but it still falls short of the full custom-QUIC platform promised by the specification.

Key measured metrics:

| Metric | Value |
|---|---:|
| Total files in working tree | 128 |
| Total Go files | 86 |
| Total Go LOC | 8,055 |
| Non-test Go files | 40 |
| Test files | 46 |
| Embedded frontend files | 3 |
| Frontend LOC (`html` + `css` + `js`) | 97 |
| Markdown docs | 9 |
| Direct Go dependencies | 2 |
| Total listed Go dependencies | 8 |
| Git commits in repo | 4 |
| Contributors in `git shortlog -sn HEAD` | 1 |

Overall health assessment: **8/10**.

Why: the repo is coherent, tested, and operational as a local diagnostic tool. `go test ./... -count=1`, `go build ./cmd/triton`, `go vet ./...`, and `gosec ./...` all pass. CI, GoReleaser, Docker, observability middleware, qlog output, parser fuzz targets, and real `quic-go` HTTP/3 support all exist. The main weaknesses are architectural ambiguity, scope mismatch with the spec, and an experimental in-repo QUIC/H3 path that is still far from RFC-complete.

Top 3 strengths:

- The current implementation is honest in code structure: there is a production-usable path built on `quic-go` in [internal/server/server.go:49](../internal/server/server.go) and [internal/realh3/http3.go:14](../internal/realh3/http3.go), plus an explicitly experimental path in [internal/server/server.go:73](../internal/server/server.go).
- Test coverage is broad for the repo size: 46 `_test.go` files cover CLI, config, dashboard, observability, probe, bench, server, storage, packet parsing, frames, streams, connection state, transport loopback, and HTTP/3 helpers.
- Build/release fundamentals are present: [Makefile:4](../Makefile), [Dockerfile:1](../Dockerfile), [.github/workflows/ci.yml:1](../.github/workflows/ci.yml), [.github/workflows/release.yml:1](../.github/workflows/release.yml), and [.goreleaser.yml:1](../.goreleaser.yml).

Top 3 concerns:

- The spec promises a custom QUIC/TLS/QPACK engine, but the shipped real HTTP/3 path relies on `quic-go`, while the in-repo transport remains experimental and non-compliant.
- The binary still carries two different H3 stories with very different maturity levels: real `quic-go` HTTP/3 and a clearly fenced but still present experimental lab transport ([internal/server/server.go:49-59](../internal/server/server.go), [internal/server/server.go:73-81](../internal/server/server.go)).
- Several production controls are still incomplete: the experimental H3 header codec is not QPACK ([internal/h3/headers.go:9-54](../internal/h3/headers.go)), the custom QUIC/H3 path is still lab-grade, and `go test -race` was not runnable in this environment because CGO was unavailable.

Post-audit update:

- Probe mode now produces explicit partial-support analysis for `0rtt`, `migration`, `qpack`, `loss`, `congestion`, `retry`, `version`, `ecn`, and `spin-bit`, plus a top-level `support_summary` rollup in [internal/probe/probe.go](../internal/probe/probe.go).
- Bench mode now emits a `summary` rollup per run that classifies protocols as healthy/degraded/failed and highlights the best and riskiest protocol in [internal/bench/bench.go](../internal/bench/bench.go).
- The embedded dashboard has moved beyond JSON dumps: [internal/dashboard/server.go](../internal/dashboard/server.go) now serves richer summaries and [internal/dashboard/assets/app.js](../internal/dashboard/assets/app.js) renders typed cards and an overview panel.
- Security hardening materially improved after the initial audit: storage path traversal is blocked in [internal/storage/filesystem.go](../internal/storage/filesystem.go), dashboard auth now uses constant-time comparison and explicit server timeouts in [internal/dashboard/server.go](../internal/dashboard/server.go), insecure probe/bench TLS requires explicit opt-in in [internal/config/config.go](../internal/config/config.go), and `gosec ./...` now passes with `0 issues`.
- Concurrency confidence is better than the original audit snapshot: CI now includes a dedicated CGO-capable `go test -race ./...` job, parser fuzz targets exist for packet/frame/wire/H3 frame surfaces, and new targeted tests cover stream, listener, UDP transport, and connection close/state edge cases in the experimental transport.

## 2. Architecture Analysis

### 2.1 High-Level Architecture

Current architecture: **single-binary modular tool with dual HTTP/3 strategies**.

The actual system is split into three runtime planes:

1. `net/http` + TLS for HTTP/1.1 and HTTP/2.
2. Real HTTP/3 based on `quic-go` for `h3://` probing and optional server-side H3.
3. An experimental in-repo UDP/QUIC/H3 implementation for `triton://` and loopback flows.

Text data flow:

```text
CLI
  -> config.Load()
  -> storage.NewFileStore()
  -> one of:
     server.New()
       -> buildHandler(appmux + request IDs + access log + headers + rate limiter)
       -> HTTPS server on server.listen_tcp
       -> optional real HTTP/3 server on server.listen_h3 (quic-go)
       -> optional experimental UDP H3 server on server.listen
       -> optional dashboard on server.dashboard_listen

     probe.Run(target, cfg)
       -> https:// : net/http + httptrace
       -> h3://    : realh3.NewClient() via quic-go/http3
       -> triton://loopback : in-process experimental loopback transport
       -> triton://host:port : experimental UDP H3 over in-repo transport
       -> save result -> render output

     bench.Run(target, cfg)
       -> h1/h2 : net/http transport workers
       -> h3    : real quic-go HTTP/3 or experimental triton:// workers
       -> save result -> render output
```

Component interaction map:

- [cmd/triton/main.go](../cmd/triton/main.go) delegates to `internal/cli`.
- [internal/cli/app.go](../internal/cli/app.go) owns config loading, store creation, mode dispatch, and output rendering.
- [internal/server/server.go](../internal/server/server.go) assembles the runtime graph.
- [internal/appmux/mux.go](../internal/appmux/mux.go) owns the shared handler surface and embedded metrics endpoint.
- [internal/dashboard/server.go](../internal/dashboard/server.go) is read-only and filesystem-backed.
- [internal/probe/probe.go](../internal/probe/probe.go) and [internal/bench/bench.go](../internal/bench/bench.go) choose between real and experimental transports by URL scheme / protocol.
- [internal/realh3/http3.go](../internal/realh3/http3.go) is the production-grade H3 client wrapper.
- [internal/h3](../internal/h3/server.go) and [internal/quic](../internal/quic/transport/listener.go) implement the toy/internal protocol path.

Concurrency model:

- `server.Run()` starts up to four goroutines: HTTPS, experimental UDP H3, real H3, and dashboard ([internal/server/server.go:120-151](../internal/server/server.go)).
- The experimental listener has one background packet read loop in [internal/quic/transport/listener.go:67-145](../internal/quic/transport/listener.go).
- `bench` spawns one worker goroutine per configured concurrency value for each protocol ([internal/bench/bench.go:98-118](../internal/bench/bench.go), [internal/bench/bench.go:145-165](../internal/bench/bench.go)).
- No shared supervisor tree or context cancellation model exists across all background work. Shutdown is coordinated in `server.Run()` only.

### 2.2 Package Structure Assessment

All Go packages:

| Package | Responsibility | Assessment |
|---|---|---|
| `cmd/triton` | Entrypoint, version/build metadata | Clean and minimal |
| `internal/appmux` | Shared HTTP handlers and in-process metrics | Good cohesion |
| `internal/bench` | Multi-protocol benchmark orchestration | Good cohesion; metrics and protocol rollups are now meaningfully richer |
| `internal/cli` | Command parsing, option merging, output formatting | Cohesive |
| `internal/config` | Defaults, YAML/env loading, validation | Cohesive |
| `internal/dashboard` | Embedded dashboard server and storage-backed APIs | Cohesive and now materially more useful as an operator surface |
| `internal/h3` | Experimental in-repo H3 client/server/loopback helpers | Cohesive for “toy H3”, but naming overstates maturity |
| `internal/h3/frame` | Minimal DATA and HEADERS frame support | Narrow but coherent |
| `internal/observability` | Request IDs, access logs, qlog tracing, file logger | Cohesive |
| `internal/probe` | Probe orchestration across HTTPS, real H3, experimental H3 | Cohesive |
| `internal/quic/connection` | Experimental connection state and frame dispatch | Simplified but coherent |
| `internal/quic/frame` | Experimental QUIC frame parsing/serialization | Large single-file implementation; incomplete vs spec |
| `internal/quic/packet` | Varints, packet numbers, header parsing | Good cohesion |
| `internal/quic/stream` | Stream state and reassembly | Cohesive, but lock discipline is imperfect |
| `internal/quic/transport` | UDP transport, experimental listener/dialer/session | Cohesive, but toy-protocol behavior |
| `internal/quic/wire` | Packet/frame assembly helpers | Cohesive |
| `internal/realh3` | Real `quic-go` HTTP/3 client wrapper | Very cohesive |
| `internal/server` | Server orchestration, TLS cert generation, rate limiting | Cohesive |
| `internal/storage` | Gzip JSON persistence and cleanup | Cohesive |
| `internal/testutil` | Shared HTTP/3 test fixtures and self-signed cert helpers | Appropriate test-only support |

Package cohesion findings:

- Strongest packages: `config`, `storage`, `observability`, `realh3`, `packet`.
- Most overloaded package: [internal/quic/frame/types.go](../internal/quic/frame/types.go), which centralizes a large amount of frame logic in one file.
- Most misleading package name: `internal/h3`, because it is not a spec-compliant HTTP/3 implementation. It uses a newline-separated header encoding in [internal/h3/headers.go:9-42](../internal/h3/headers.go) rather than QPACK.

Circular dependency risk: low today. Dependencies mostly flow inward from CLI/server toward protocol and support packages. The main architectural risk is not cycles; it is confusion between the real and experimental HTTP/3 stacks.

Internal vs `pkg/` separation: good. Nothing here is stable enough to export publicly, so `internal/` everywhere is appropriate.

### 2.3 Dependency Analysis

#### Go dependencies from `go.mod`

| Dependency | Version | Purpose | Maintenance status | Stdlib replacement? |
|---|---|---|---|---|
| `github.com/quic-go/quic-go` | `v0.59.0` | Real HTTP/3 server/client support, qlog integration | Actively maintained | No practical stdlib replacement |
| `gopkg.in/yaml.v3` | `v3.0.1` | YAML config loading and YAML output | Mature and stable | No stdlib YAML |
| `github.com/kr/text` | `v0.2.0` indirect | Transitive dependency | Old but harmless indirect | N/A |
| `github.com/quic-go/qpack` | `v0.6.0` indirect | `quic-go` transitive QPACK dep | Active | N/A |
| `golang.org/x/crypto` | `v0.41.0` indirect | `quic-go` crypto support | Active | Partial stdlib overlap only |
| `golang.org/x/net` | `v0.43.0` indirect | `quic-go` network protocols | Active | No |
| `golang.org/x/sys` | `v0.35.0` indirect | Syscall/platform support | Active | No |
| `golang.org/x/text` | `v0.28.0` indirect | Text/encoding support | Active | Partial |

Dependency hygiene assessment:

- Good overall. Only two direct dependencies.
- The current code no longer satisfies the spec’s “zero external dependencies except selected x/* + yaml” rule because it directly depends on `quic-go` ([go.mod:5-8](../go.mod)).
- That deviation is probably justified from a delivery standpoint: without `quic-go`, there would be no real HTTP/3 path at all.
- No dependency audit tooling or SBOM is present.

#### Frontend dependencies

There is no `package.json`, no React app, and no Node build pipeline. The dashboard is hand-written static HTML/CSS/JS in:

- [internal/dashboard/assets/index.html](../internal/dashboard/assets/index.html)
- [internal/dashboard/assets/app.css](../internal/dashboard/assets/app.css)
- [internal/dashboard/assets/app.js](../internal/dashboard/assets/app.js)

### 2.4 API & Interface Design

#### Server endpoint inventory

| Method | Path | Handler | Notes |
|---|---|---|---|
| `GET` | `/` | `handleRoot` | capability summary |
| `GET` | `/healthz` | `handleHealth` | liveness |
| `GET` | `/readyz` | `handleReady` | readiness |
| `GET` | `/metrics` | `Metrics.handleMetrics` | Prometheus-style plaintext |
| `GET` | `/ping` | `handlePing` | plaintext `pong` |
| `GET`,`POST` | `/echo` | `handleEcho` | bounded request body |
| `GET` | `/download/:size` | `handleDownload` | deterministic byte stream |
| `POST` | `/upload` | `handleUpload` | body size + duration |
| `GET` | `/delay/:ms` | `handleDelay` | sleep-based delay |
| `GET` | `/redirect/:n` | `handleRedirect` | chained 302 |
| `GET` | `/streams/:n` | `handleStreams` | simulated stream schedule JSON |
| `GET` | `/headers/:n` | `handleHeaders` | emits `X-Triton-*` headers |
| `GET` | `/status/:code` | `handleStatus` | arbitrary status |
| `GET` | `/drip/:size/:delay` | `handleDrip` | byte drip-feed |
| `GET` | `/tls-info` | `handleTLSInfo` | TLS connection metadata |
| `GET` | `/quic-info` | `handleQUICInfo` | explicitly says custom QUIC unsupported |
| `GET` | `/migration-test` | `handleMigration` | explicitly says unsupported |
| `GET` | `/.well-known/triton` | `handleCapabilities` | machine-readable summary |

#### Dashboard/API inventory

| Method | Path | Handler | Notes |
|---|---|---|---|
| `GET`,`HEAD` | `/` | asset handler | serves dashboard HTML |
| `GET`,`HEAD` | `/assets/app.css` | asset handler | embedded CSS |
| `GET`,`HEAD` | `/assets/app.js` | asset handler | embedded JS |
| `GET` | `/api/v1/status` | `handleStatus` | structured dashboard/storage status summary |
| `GET` | `/api/v1/config` | `handleConfig` | sanitized runtime config snapshot |
| `GET` | `/api/v1/probes` | `handleProbes` | list stored probes |
| `GET` | `/api/v1/probes/:id` | `handleProbe` | fetch probe result |
| `GET` | `/api/v1/benches` | `handleBenches` | list stored benches |
| `GET` | `/api/v1/benches/:id` | `handleBench` | fetch bench result |
| `GET` | `/api/v1/traces` | `handleTraces` | list `.sqlog` files |
| `GET` | `/api/v1/traces/meta/:name` | `handleTraceMeta` | preview trace metadata |
| `GET` | `/api/v1/traces/:name` | `handleTrace` | serve qlog file |

API consistency assessment:

- Method enforcement is much better than in earlier revisions. It is consistently implemented by `methodHandler` in [internal/appmux/mux.go:261-273](../internal/appmux/mux.go) and `getOnly` in [internal/dashboard/server.go:251-258](../internal/dashboard/server.go).
- Success payloads are mostly JSON and fairly consistent.
- Error payloads are inconsistent: many endpoints still use `http.Error`, which returns plaintext.
- The dashboard is read-only and intentionally narrow. There is no mutation API.

Authentication and authorization model:

- Main server endpoints: none.
- Dashboard: optional HTTP Basic Auth in [internal/dashboard/server.go:225-238](../internal/dashboard/server.go), driven by config/env/CLI flags.
- No role model, no session management, no authorization layer.

Rate limiting, CORS, validation patterns:

- Rate limiting exists only on the main server handler chain and is per-IP fixed-window ([internal/server/ratelimit.go:28-53](../internal/server/ratelimit.go)).
- No CORS middleware exists.
- Input validation is route-local and mostly integer parsing plus request body limits.

## 3. Code Quality Assessment

### 3.1 Go Code Quality

Code style consistency:

- The code is consistently formatted and idiomatic.
- Naming is clear and package boundaries are understandable.
- The repo feels intentionally simple rather than abstract-heavy.

Error handling patterns:

- Top-level command paths and constructors generally return errors cleanly.
- Handler code often ignores write errors after best effort, which is normal for HTTP handlers.
- Some transport code intentionally drops errors, such as [internal/quic/transport/listener.go:104](../internal/quic/transport/listener.go), which ignores `conn.HandleFrames(frames)` failures.
- `http.Error` is used widely instead of structured error objects.

Context usage:

- Strongest in `server.Run()` and `dashboard.Shutdown()`.
- Probe and bench flows are timeout-based, not context-first.
- The experimental in-repo transport has no propagated context model.

Logging approach:

- Good baseline. Access logging is structured JSON via `slog` in [internal/observability/http.go:28-46](../internal/observability/http.go).
- Request IDs are generated and propagated by [internal/observability/http.go:15-25](../internal/observability/http.go).
- Optional file-based access log output is handled by [internal/observability/logger.go:18-30](../internal/observability/logger.go).
- Missing pieces: log levels in config, masking policy, and broader application event logging.

Configuration management:

- Clean precedence model: defaults -> YAML -> env -> CLI.
- Validation is solid for core listen/timeouts/body size settings.
- Some declared fields are not deeply validated or fully consumed:
  - `ProbeConfig.DefaultTests`, `DownloadSize`, `UploadSize`, `DefaultStreams`
  - `BenchConfig.Warmup` is validated but not used in the runner
  - the binary still exposes both supported and experimental H3 stories, even though the experimental path now requires explicit opt-in

Magic numbers, hardcoded values, comments:

- Default 1 MiB body cap in [internal/appmux/mux.go:23](../internal/appmux/mux.go).
- Hardcoded 8-byte short-header DCID assumption in [internal/quic/transport/listener.go:78](../internal/quic/transport/listener.go).
- Hardcoded in-repo stream/data limits of `1 << 20` across `connection` and `stream` packages.
- No `TODO`, `FIXME`, or `HACK` markers were found.

### 3.2 Frontend Code Quality

There is no React, TypeScript, router, or frontend build system. The dashboard is a static embedded page.

Assessment:

- UI complexity is intentionally minimal.
- [internal/dashboard/assets/index.html](../internal/dashboard/assets/index.html) is a single-page embedded dashboard with overview, status, config, probes, benches, and trace cards.
- [internal/dashboard/assets/app.js](../internal/dashboard/assets/app.js) now renders typed summaries, support rollups, bench health rollups, and a top-level overview panel instead of dumping raw JSON.
- [internal/dashboard/assets/app.css](../internal/dashboard/assets/app.css) is still small and readable; it remains intentionally lightweight rather than a full design system implementation.
- Accessibility is basic but acceptable for a tiny read-only tool: semantic headings exist, but there are no live regions, status roles, or keyboard-focused interactions.

### 3.3 Concurrency & Safety

Goroutine lifecycle:

- Server mode has a deliberate shutdown path with signal handling and a 10-second timeout ([internal/server/server.go:153-175](../internal/server/server.go)).
- The experimental UDP listener is background-driven and relies on socket close for shutdown ([internal/quic/transport/listener.go:67-145](../internal/quic/transport/listener.go)).
- Bench workers stop based on a wall-clock deadline and `WaitGroup`, which is simple and sufficient for the current use case.

Mutex/channel usage patterns:

- `Listener` and `Connection` use straightforward coarse locking.
- `Stream` still uses separate `sendMu` and `recvMu`, but the state-transition surface has been reduced and hardened with focused concurrency tests. Remaining confidence work should now be driven by CI `-race` results rather than obvious local gaps.

Race condition risks:

- `Stream.state` lock inconsistency as above.
- `Listener` now has focused tests around `Accept()`, `WaitForConnections()`, close-unblock behavior, and multi-connection ordering, but the surface is still specialized enough that it should stay lab-only until more race/fuzz mileage accumulates.

Resource leak risks:

- `rateLimiter.buckets` never shrinks ([internal/server/ratelimit.go:21-53](../internal/server/ratelimit.go)), which can become an unbounded map if exposed to many source IPs.
- Otherwise, shutdown and file-handle management are good.

Graceful shutdown implementation:

- Good for HTTP/TLS server, dashboard, and real H3 server.
- Experimental UDP H3 listener is closed before HTTP shutdown, which is reasonable.
- No cancellation model for long-running probe/bench operations because they are CLI-bound.

### 3.4 Security Assessment

Input validation coverage:

- Good request body limits on `/echo` and `/upload` using `http.MaxBytesReader` ([internal/appmux/mux.go:122-129](../internal/appmux/mux.go), [internal/appmux/mux.go:276-283](../internal/appmux/mux.go)).
- Route params are parsed and range-checked.
- There is no global validation layer.

Injection protection:

- No SQL layer exists.
- No shell execution paths exist.
- Dashboard trace serving sanitizes the requested filename with `path.Base` and extension checks ([internal/dashboard/server.go:156-173](../internal/dashboard/server.go)).

XSS protection:

- Low current XSS risk because the dashboard renders JSON via `textContent` ([internal/dashboard/assets/app.js:4-8](../internal/dashboard/assets/app.js)).
- Strong CSP and related headers are set for both dashboard and main server ([internal/dashboard/server.go:240-248](../internal/dashboard/server.go), [internal/server/server.go:178-186](../internal/server/server.go)).

Secrets management:

- `.gitignore` excludes `.env*` and `/triton-data/`, which is correct.
- The working tree still contains generated runtime cert/key material under `triton-data/certs/`; that should not be preserved in a clean production repo snapshot.

TLS/HTTPS configuration:

- Real HTTP/3 path enforces TLS 1.3 minimum ([internal/server/server.go:54](../internal/server/server.go), [internal/realh3/http3.go:16-19](../internal/realh3/http3.go)).
- HTTPS TCP path allows TLS 1.2+ ([internal/server/server.go:67-70](../internal/server/server.go)).
- `--insecure` works as expected for probe and bench by toggling `InsecureSkipVerify` in [internal/probe/probe.go:62-71](../internal/probe/probe.go), [internal/bench/bench.go:81-90](../internal/bench/bench.go), and [internal/realh3/http3.go:16-19](../internal/realh3/http3.go).

CORS:

- No CORS support.
- Acceptable for localhost dashboard, but not for a remotely exposed API product.

Known vulnerability patterns:

- Fixed-window rate limiter with no state eviction.
- Optional Basic Auth only; no stronger dashboard auth.
- Experimental UDP H3 path has no cryptographic protection or real QUIC-TLS.

## 4. Testing Assessment

### 4.1 Test Coverage

Measured command results on 2026-04-11:

- `go test ./... -count=1`: **pass**
- `go build ./cmd/triton`: **pass** with a sandbox-related non-fatal stat-cache write warning
- `go vet ./...`: **pass**
- `staticcheck ./...`: **could not be validated cleanly in this shell**; local binary exists, but this PowerShell environment returned `warning: "./..." matched no packages`
- `go test -race ./...`: **still not runnable in this local environment** because CGO was disabled (`-race requires cgo`), but CI now includes a dedicated CGO-capable race job
- `gosec ./...`: **pass** (`0 issues`)

Test-file ratio:

- 46 test files vs 40 non-test Go files
- That is unusually good breadth for a pre-v1 project of this size

Packages with test files:

- `cmd/triton`
- `internal/appmux`
- `internal/bench`
- `internal/cli`
- `internal/config`
- `internal/dashboard`
- `internal/h3`
- `internal/h3/frame`
- `internal/observability`
- `internal/probe`
- `internal/quic/connection`
- `internal/quic/frame`
- `internal/quic/packet`
- `internal/quic/stream`
- `internal/quic/transport`
- `internal/quic/wire`
- `internal/realh3`
- `internal/server`
- `internal/storage`
- `internal/testutil`

Packages with zero test files:

- None among actual Go packages.

Test types present:

- Unit tests: yes
- Integration tests: yes
- Protocol loopback tests: yes
- Real HTTP/3 tests using `quic-go`: yes
- Benchmarks (`Benchmark*`): none
- Fuzz tests: none
- E2E browser/UI tests: none

Test quality assessment:

- Strong for the repo’s current actual scope.
- Especially good around transport helpers, server assembly, observability, and config.
- Missing coverage mostly aligns with missing features: there are no tests for QPACK, 0-RTT, migration, congestion control, or rich dashboard interactivity because those features are not implemented.

### 4.2 Test Infrastructure

- CI exists in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) and runs format verification, tests, vet, staticcheck, build, and smoke flow.
- Release automation exists in [`.github/workflows/release.yml`](../.github/workflows/release.yml) and [`.goreleaser.yml`](../.goreleaser.yml).
- Smoke script coverage is useful and non-trivial: [scripts/ci-smoke.sh](../scripts/ci-smoke.sh) checks `server`, `probe`, `bench`, health endpoints, metrics, real H3 probe, and loopback/triton targets.
- Missing test infrastructure:
  - Local `-race` validation is still unavailable in this shell even though CI now has a race job
  - Fuzz coverage exists for packet, frame, wire, and H3 frame parser surfaces, but is still early-stage
  - No load/perf benchmarks
  - No browser automation for dashboard

## 5. Specification vs Implementation Gap Analysis

### 5.1 Feature Completion Matrix

| Planned Feature | Spec Section | Implementation Status | Files/Packages | Notes |
|---|---|---|---|---|
| Single-binary CLI with `server`, `probe`, `bench` | SPEC §4, §15 | ✅ Complete | `cmd/triton`, `internal/cli` | Working and tested |
| YAML/env/CLI config layering | SPEC §10 | ✅ Complete | `internal/config` | Clean precedence model |
| Filesystem result storage | SPEC §12 | ✅ Complete | `internal/storage` | Gzip JSON save/load/list/cleanup |
| HTTPS test server + fallback protocols | SPEC §4.1 | ✅ Complete | `internal/server`, `internal/appmux` | HTTP/1.1 and H2 via `net/http` |
| Real HTTP/3 server/client | SPEC §2, §7 | ⚠️ Partial | `internal/server`, `internal/realh3`, `internal/probe`, `internal/bench` | Present via `quic-go`, not custom implementation |
| Custom QUIC transport | SPEC §3, §15 | ⚠️ Partial | `internal/quic/*`, `internal/h3` | Experimental only; not RFC-complete |
| QPACK encoder/decoder | SPEC §2.1, §7 | ❌ Missing | — | No QPACK package; experimental H3 uses string header blocks |
| QUIC-TLS key schedule and packet protection | SPEC §1.8, §12 | ❌ Missing | — | Not implemented in in-repo transport |
| 0-RTT support | SPEC §4.2, §12.2 | ❌ Missing | — | Spec-only |
| Connection migration | SPEC §4.2, §13 | ❌ Missing | — | Server endpoint exists, but capability is not implemented |
| Congestion control / loss recovery | SPEC §5 | ❌ Missing | — | No recovery or congestion packages |
| Probe over HTTPS | SPEC §4.2 | ✅ Complete | `internal/probe` | Basic timing/TLS metadata |
| Probe over real H3 | SPEC §4.2 | ✅ Complete | `internal/probe`, `internal/realh3` | `h3://` works |
| Probe over experimental transport | SPEC not explicit | ✅ Complete | `internal/probe`, `internal/h3`, `internal/quic` | `triton://` works |
| Deep probe tests: 0-RTT, migration, streams, qpack, grease, spin-bit, congestion | SPEC §4.2 | ❌ Missing | — | Not implemented |
| Benchmark H1/H2 | SPEC §4.3 | ✅ Complete | `internal/bench` | Functional |
| Benchmark real H3 | SPEC §4.3 | ✅ Complete | `internal/bench`, `internal/realh3` | Explicit `h3` protocol path |
| Benchmark network simulator | SPEC §5.2 | ❌ Missing | — | No simulator |
| Dashboard asset embedding | SPEC §7 | ✅ Complete | `internal/dashboard` | Embedded assets |
| Dashboard read APIs | SPEC §9 | ⚠️ Partial | `internal/dashboard` | Status, config, probes, benches, traces, and trace metadata preview |
| Dashboard SSE / WebSocket / charts / inspector | SPEC §7, §9 | ❌ Missing | — | Not implemented |
| qlog file generation | SPEC §6.2 | ⚠️ Partial | `internal/observability` | Available for real H3 via `quic-go`, not custom transport |
| ACME automation | SPEC §4.1, §7 | ❌ Missing | — | Self-signed only |
| Build/release automation | SPEC §14 | ✅ Complete | `Makefile`, `Dockerfile`, workflows, GoReleaser | Present, though local Make targets are smaller than spec |

### 5.2 Architectural Deviations

1. **Real H3 uses `quic-go`, not the custom QUIC engine.**
   Impact: pragmatic improvement for shipping functionality, but a direct violation of the spec’s core architecture.

2. **There are two different H3 stacks in one binary.**
   Files: [internal/server/server.go:49-59](../internal/server/server.go), [internal/server/server.go:73-81](../internal/server/server.go)
   Impact: useful for incremental development, but confusing operationally.

3. **Experimental H3 headers are newline-delimited strings, not QPACK.**
   Files: [internal/h3/headers.go:9-42](../internal/h3/headers.go)
   Impact: acceptable for internal loopback experiments, not for spec compliance.

4. **The dashboard is read-only and static.**
   Impact: scope reduction, not necessarily a regression, but far below planned UX.

5. **Spec says zero-dependency custom stack; implementation deliberately chose a dependency-backed path for real H3.**
   Impact: major strategic deviation, arguably the right tradeoff if product value matters more than ideological purity.

### 5.3 Task Completion Assessment

`TASKS.md` defines 142 tasks.

Reasoned completion estimate by phase:

| Phase | Estimated Completion | Notes |
|---|---:|---|
| Phase 1 - Scaffold & UDP Transport | 90% | Mostly done |
| Phase 2 - QUIC Packet Layer | 55% | Header parsing and packet numbers done; parser fuzz targets now exist, but protection and full packet coverage are still missing |
| Phase 3 - QUIC Frames & Crypto | 40% | Frame parsing present; crypto/tls parts missing |
| Phase 4 - QUIC Connection & Streams | 60% | Simplified connection/stream model works |
| Phase 5 - Recovery & Congestion | 0% | Missing |
| Phase 6 - Migration & 0-RTT | 0% | Missing |
| Phase 7 - HTTP/3 Layer | 55% | Real H3 exists via `quic-go`; custom H3 incomplete |
| Phase 8 - Probe & Bench | 55% | Core flows work; deep protocol analysis absent |
| Phase 9 - Dashboard | 35% | Read-only dashboard only |
| Phase 10 - Polish & Release | 75% | README, CI, release automation present |

Overall task completion estimate: **~49% weighted**.

### 5.4 Scope Creep Detection

Features/code present that were not central in the original spec:

- `triton://` experimental transport scheme
- In-process QUIC/H3 loopback server/client helpers
- Read-only qlog browsing in dashboard
- Embedded Prometheus-style `/metrics` endpoint

Assessment:

- `triton://` and loopback helpers are valuable engineering accelerators.
- qlog browsing and metrics are also valuable additions.
- None of these are harmful scope creep; the real issue is unfinished core scope, not extra features.

### 5.5 Missing Critical Components

Highest-impact missing pieces:

1. Custom QUIC-TLS handshake and packet protection.
2. QPACK encoder/decoder and real custom HTTP/3 framing/control streams.
3. Migration, 0-RTT, congestion, loss recovery, and MTU logic.
4. Rich probe analysis promised by the spec.
5. Interactive dashboard capabilities promised by the spec.

## 6. Performance & Scalability

### 6.1 Performance Patterns

Potential hot paths and bottlenecks:

- Benchmark workers compute only aggregate averages; no latency histogram, percentile, or connection-setup breakdown exists.
- `/delay` and `/drip` deliberately block request goroutines; fine for test endpoints, bad for production-style workload realism.
- Dashboard list APIs re-scan filesystem state per request ([internal/storage/filesystem.go:58-75](../internal/storage/filesystem.go)).
- The experimental H3/header codec allocates heavily and is not meant for scale.

Memory/allocation patterns:

- `UDPTransport` uses a `sync.Pool` for read buffers ([internal/quic/transport/udp.go:15-20](../internal/quic/transport/udp.go)).
- Storage and observability code are allocation-light.
- Experimental stream reassembly copies buffers during merges; reasonable at this size.

HTTP response compression/static asset optimization:

- No gzip/brotli asset serving.
- No cache headers for static assets beyond `Cache-Control: no-store`.
- This is acceptable for a local dashboard, not ideal for a public UI.

### 6.2 Scalability Assessment

- Horizontal scalability: limited. State is local filesystem and qlog file-based.
- Session/state model: mostly stateless for serving requests, stateful for stored results.
- No queue/worker abstraction.
- No back-pressure or admission control besides a simple rate limiter.
- Real H3 path inherits `quic-go` behavior, which is more scalable than the in-repo transport.

## 7. Developer Experience

### 7.1 Onboarding Assessment

- Good. Clone, `go test`, `go run`, and `go build` work without extra services.
- Local setup requirements are minimal.
- Config example is present in [triton.yaml.example](../triton.yaml.example).

### 7.2 Documentation Quality

- `README.md` is strong and substantially aligned to the current repo.
- Planning docs are detailed but still aspirational.
- Missing standalone docs: `LICENSE`, `CONTRIBUTING.md`, `ARCHITECTURE.md`, `API.md`.

### 7.3 Build & Deploy

- Stronger than many pre-v1 repos:
  - CI exists
  - release workflow exists
  - GoReleaser exists
  - Docker runs as non-root
- Gaps:
  - local `Makefile` doesn’t expose all platforms the release config supports
  - no explicit deployment manifests or staging/prod environment docs

## 8. Technical Debt Inventory

### 🔴 Critical

1. [internal/h3/headers.go:9-42](../internal/h3/headers.go): experimental H3 headers are not QPACK and are unsuitable for standards claims. Suggested fix: keep this package explicitly lab-only or replace it with a real implementation if the custom-engine roadmap remains active. Effort: 2-4h for isolation, much larger for real implementation.
2. [internal/quic](../internal/quic): the custom QUIC transport still lacks QUIC-TLS, packet protection, recovery, and spec-level interoperability. Suggested fix: either keep it research-only or fund the remaining engine work explicitly. Effort: multi-week.

### 🟡 Important

1. `go test -race` is now exercised in CI, but not locally in this environment. Suggested fix: treat any CI race findings as release blockers for the experimental transport and keep hardening around those results. Effort: ongoing plus follow-up fixes.
2. [internal/quic/transport/listener.go:49-55](../internal/quic/transport/listener.go), [internal/quic/transport/listener.go:158-171](../internal/quic/transport/listener.go): the experimental listener surface is still specialized and should stay clearly lab-only. Suggested fix: avoid expanding it into the default production story before race/fuzz coverage exists. Effort: 1-2h.
3. [internal/dashboard/assets/app.js](../internal/dashboard/assets/app.js): dashboard is materially better than the original scaffold, but still not the live inspection UI described in the spec. Suggested fix: keep the product promise narrow or invest in richer comparison/filtering UX deliberately. Effort: medium.
4. `triton-data/` and built binaries in repo root/worktree: generated artifacts should not live beside source in a clean release branch. Effort: low.

### 🟢 Minor

1. [internal/server/server.go:67-70](../internal/server/server.go): HTTPS TCP fallback allows TLS 1.2+. If this is a pure modern diagnostics tool, TLS 1.3-only may be preferable for consistency.
2. [Makefile](../Makefile): local cross-build targets are narrower than GoReleaser’s matrix, although `security`/`gosec` verification is now included.
3. [internal/dashboard](../internal/dashboard): the dashboard remains intentionally small and dependency-free; deeper trend/comparison UX is still future work.

## 9. Metrics Summary Table

| Metric | Value |
|---|---|
| Total Go Files | 86 |
| Total Go LOC | 8,055 |
| Total Frontend Files | 3 |
| Total Frontend LOC | 97 |
| Test Files | 46 |
| Test Coverage (estimated) | 75-80% of current implemented scope |
| External Go Dependencies | 2 direct / 8 total listed |
| External Frontend Dependencies | 0 |
| Open TODOs/FIXMEs | 0 |
| API Endpoints | 28 total (18 server + 10 dashboard/assets) |
| Spec Feature Completion | ~52% |
| Task Completion | ~49% |
| Overall Health Score | 8/10 |
