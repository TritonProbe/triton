# Project Analysis Report

> Auto-generated comprehensive analysis of Triton
> Generated: 2026-04-11
> Analyzer: Codex - Full Codebase Audit

## 1. Executive Summary

Triton is a Go CLI plus local dashboard intended to become a single-binary HTTP/3 and QUIC test platform with three operating modes: `server`, `probe`, and `bench`. The current repository is not a production HTTP/3 platform. It is a pre-v1 scaffold with a functioning CLI, a TLS TCP test server, a filesystem-backed result store, a minimal dashboard, a basic HTTPS probe/benchmark flow, and an in-process loopback-only QUIC/H3 playground used mainly by tests and the `triton://loopback/...` probe path.

Key measured metrics:

| Metric | Value |
|---|---:|
| Total tracked files in repo working tree | 75 |
| Go source files | 52 |
| Go LOC | 4,792 |
| Test files | 20 |
| Embedded frontend files | 3 |
| Frontend LOC (`html` + `css` + `js`) | 92 |
| Markdown docs | 6 |
| Direct Go dependencies | 1 |
| Contributors in git shortlog | 1 |

Overall health assessment: **4/10**.

Justification: the codebase is small, readable, and testable, and `go build`, `go test ./...`, `go vet ./...`, and `staticcheck ./...` all pass. But the implementation falls far short of the specification: there is no network-capable QUIC/TLS stack, no real HTTP/3 server/client, no H3 benchmarking, no analytics engine, no auth, no production hardening, no CI/CD, and no release tooling. The repository is honest in `README.md`, but the specification-to-code gap is very large.

Top 3 strengths:

- Small codebase with low dependency surface: only `gopkg.in/yaml.v3` is used directly in [go.mod](/d:/Codebox/TritonProbe/go.mod).
- Core scaffolding is coherent and testable: loopback QUIC/H3 helpers, storage, config loading, and endpoint behavior all have focused tests.
- Basic developer workflow works today: `go build ./cmd/triton`, `go test ./... -count=1`, and runtime commands like `go run ./cmd/triton probe --target triton://loopback/ping --format json` succeed.

Top 3 concerns:

- The shipped server is HTTP/1.1 + HTTP/2 over TCP/TLS only; the advertised QUIC/HTTP/3 product is mostly speculative.
- Security and production controls are weak: bench mode disables TLS verification unconditionally in [internal/bench/bench.go:56-69](/d:/Codebox/TritonProbe/internal/bench/bench.go:56), dashboard has no auth or hardening in [internal/dashboard/server.go:25-128](/d:/Codebox/TritonProbe/internal/dashboard/server.go:25), and committed runtime artifacts include a private key under `triton-data/certs/`.
- Architectural drift is already visible: endpoint handlers are duplicated in [internal/appmux/mux.go](/d:/Codebox/TritonProbe/internal/appmux/mux.go:15) and [internal/server/endpoints.go](/d:/Codebox/TritonProbe/internal/server/endpoints.go:15), while old spec-only structure still appears in planning docs.

## 2. Architecture Analysis

### 2.1 High-Level Architecture

Current architecture: **single-process modular scaffold**, not a modular monolith with fully implemented subsystems.

Actual runtime architecture today:

```text
CLI
  -> config.Load()
  -> storage.NewFileStore()
  -> one of:
     server.New() -> net/http TLS server + optional dashboard
     probe.Run()  -> net/http HTTPS probe OR loopback-only H3 path
     bench.Run()  -> net/http benchmark for h1/h2 only

Dashboard
  -> reads saved gzip JSON files from triton-data/
  -> serves minimal embedded assets

Loopback QUIC/H3 path
  -> transport.Listener / transport.Dialer
  -> connection.Connection / stream.Manager
  -> h3 loopback request/response translation
```

Text data flow for each mode:

```text
server:
CLI -> config -> storage -> self-signed certs -> net/http TLS server -> appmux handlers
                                         -> dashboard server -> filesystem listing/load

probe:
CLI -> config -> probe.Run
  -> external target: net/http client + httptrace -> Result -> filesystem save
  -> triton://loopback target: in-process QUIC/H3 loopback -> Result -> filesystem save

bench:
CLI -> config -> bench.Run -> shared http.Client workers -> Result -> filesystem save
```

Component interaction map:

- `cmd/triton/main.go` boots `internal/cli`.
- `internal/cli` owns command selection, flag parsing, config loading, persistence, and output rendering.
- `internal/server` owns TLS server startup, graceful shutdown, and self-signed cert generation.
- `internal/appmux` owns the currently active HTTP handlers.
- `internal/dashboard` is a separate `http.Server` reading from `storage.FileStore`.
- `internal/probe` uses `net/http` for real targets and `internal/h3` + `internal/quic/transport` only for loopback.
- `internal/bench` uses `net/http` only; there is no `h3` benchmark runner.
- `internal/quic/*` provides parsers, a lightweight connection/stream model, and a toy listener/dialer/session.

Concurrency model:

- `internal/server/server.go:58-71` starts one goroutine for the HTTPS server and one optional goroutine for the dashboard.
- `internal/quic/transport/listener.go:41` starts one background packet read loop per listener.
- `internal/bench/bench.go:74-92` spawns one worker goroutine per requested concurrency value.
- There is no shared context tree across modes, no bounded worker pools, and no structured lifecycle supervision beyond signal-based shutdown in server mode.

### 2.2 Package Structure Assessment

Go packages and responsibilities:

| Package | Responsibility | Assessment |
|---|---|---|
| `cmd/triton` | Entrypoint, version/build vars | Good cohesion |
| `internal/appmux` | Shared HTTP handlers used by server and loopback H3 | Reasonable, but duplicates `internal/server/endpoints.go` |
| `internal/bench` | Basic benchmark runner for `h1` and `h2` using `net/http` | Cohesive but far below spec |
| `internal/cli` | Command routing, flag parsing, output rendering | Cohesive, intentionally simple |
| `internal/config` | Defaults, YAML/env loading, validation | Cohesive |
| `internal/dashboard` | Minimal dashboard server, static assets, result lookup APIs | Cohesive but very thin |
| `internal/h3` | Loopback H3 request/response plumbing and naive header codec | Mixed: names imply real H3 implementation, behavior is toy-grade |
| `internal/h3/frame` | Minimal DATA/HEADERS frame encode/parse | Cohesive but incomplete |
| `internal/probe` | HTTPS probe and loopback H3 probe | Cohesive but narrow |
| `internal/quic/connection` | Connection state, frame dispatch, CID store | Cohesive but simplified |
| `internal/quic/frame` | QUIC frame encode/parse | Large single file, incomplete relative to constants declared |
| `internal/quic/packet` | Varint, packet number, packet header parsing | Cohesive |
| `internal/quic/stream` | Stream state and reassembly | Cohesive, some locking issues |
| `internal/quic/transport` | UDP transport, listener, dialer, session | Cohesive, toy protocol behavior |
| `internal/quic/wire` | Packet/frame composition helpers | Cohesive |
| `internal/server` | TLS server orchestration, duplicate endpoints, cert generation | Mixed due to dead/duplicate mux |
| `internal/storage` | Gzip JSON persistence and retention cleanup | Cohesive |

Package cohesion findings:

- Best cohesion: `config`, `storage`, `packet`, `wire`.
- Weakest cohesion-by-name: `h3` and `server`; the names imply production subsystems but the actual code is loopback scaffolding or dead duplication.
- `internal/server/endpoints.go` appears unused after the README-documented shift to `appmux.New()` in [internal/server/server.go:32-43](/d:/Codebox/TritonProbe/internal/server/server.go:32), which is technical debt.

Circular dependency risk:

- No current cycle is visible.
- Risk is low because packages are still shallow and mostly flow inward from CLI to domain packages.
- The biggest future risk is between `server`, `appmux`, `probe`, `dashboard`, and any future analytics layer if shared DTOs and handlers are not separated cleanly.

Internal vs public separation:

- Everything meaningful is in `internal/`, which is appropriate for a pre-v1 single binary.
- There is no `pkg/` surface, which is fine; nothing is yet stable enough to export.

### 2.3 Dependency Analysis

Go dependencies from `go.mod` and `go.sum`:

| Dependency | Version | Purpose | Status | Replaceable with stdlib? |
|---|---|---|---|---|
| `gopkg.in/yaml.v3` | `v3.0.1` | YAML config load and YAML output in CLI | Active and common | Not directly; stdlib has no YAML |
| `gopkg.in/check.v1` | indirect via `go.sum` | Test-time transitive dep from YAML ecosystem | Old but only indirect | N/A |

Dependency hygiene:

- Very good surface area.
- The spec allows `gopkg.in/yaml.v3`; the repo currently uses only that direct external dependency.
- `staticcheck ./...` passed on 2026-04-11.
- No evidence of dependency sprawl, but also no SBOM, no dependency scanning, and no lock-policy beyond `go.sum`.

Frontend dependencies:

- None.
- There is no `package.json`, no React app, no build toolchain, and no separate frontend package.

### 2.4 API & Interface Design

Server endpoint inventory as actually implemented:

| Method(s) | Path | Handler | Notes |
|---|---|---|---|
| `GET` | `/` | `handleRoot` | Returns basic JSON metadata |
| any | `/ping` | `handlePing` | Plaintext `pong` |
| any | `/echo` | `handleEcho` | Echoes headers/body as JSON |
| any | `/download/:size` | `handleDownload` | Size parsed from tail |
| any | `/upload` | `handleUpload` | Reads full request body |
| any | `/delay/:ms` | `handleDelay` | Sleeps in request path |
| any | `/redirect/:n` | `handleRedirect` | 302 chain |
| any | `/streams/:n` | `handleStreams` | Simulated JSON only |
| any | `/headers/:n` | `handleHeaders` | Adds synthetic headers |
| any | `/status/:code` | `handleStatus` | Writes JSON with requested status |
| any | `/drip/:size/:delay` | `handleDrip` | Writes random bytes with per-byte sleep |
| any | `/tls-info` | `handleTLSInfo` | TLS info for TCP/TLS server only |
| any | `/quic-info` | `handleQUICInfo` | Always `supported: false` |
| any | `/migration-test` | `handleMigration` | Always `supported: false` |
| any | `/.well-known/triton` | `handleCapabilities` | Returns capabilities JSON |

Dashboard/API endpoints:

| Method | Path | Handler | Notes |
|---|---|---|---|
| `GET` | `/` | inline asset handler | Serves `index.html` |
| `GET` | `/assets/app.css` | inline asset handler | Embedded CSS |
| `GET` | `/assets/app.js` | inline asset handler | Embedded JS |
| `GET` | `/api/v1/status` | `handleStatus` | Always `{ "ok": true }` |
| `GET` | `/api/v1/probes` | `handleProbes` | File listing only |
| `GET` | `/api/v1/probes/:id` | `handleProbe` | File load only |
| `GET` | `/api/v1/benches` | `handleBenches` | File listing only |
| `GET` | `/api/v1/benches/:id` | `handleBench` | File load only |

API consistency assessment:

- JSON responses are mostly consistent within a package, but not globally standardized.
- Error handling is inconsistent: some endpoints use JSON responses, others use `http.Error`.
- Methods are not enforced on several routes even though the README/spec describe specific methods.
- No API versioning beyond dashboard endpoints.

Authentication/authorization:

- None.
- No auth on dashboard APIs.
- No auth on server endpoints.

Rate limiting, CORS, validation:

- Rate limiting is spec-only. `ServerConfig.RateLimit` exists in [internal/config/config.go:20-33](/d:/Codebox/TritonProbe/internal/config/config.go:20) but is unused.
- No CORS handling in dashboard or server.
- Validation is shallow and route-local; there is no shared validation layer.

## 3. Code Quality Assessment

### 3.1 Go Code Quality

Style consistency:

- The repo appears `gofmt`-clean.
- Naming is generally clear and idiomatic.
- The code is readable, compact, and intentionally low-abstraction.

Error handling:

- Mixed quality.
- Good: constructors and top-level flows usually return errors upward.
- Weak: many handlers ignore body read/write errors, for example [internal/appmux/mux.go:48-56](/d:/Codebox/TritonProbe/internal/appmux/mux.go:48) and [internal/appmux/mux.go:168-173](/d:/Codebox/TritonProbe/internal/appmux/mux.go:168).
- `internal/quic/transport/listener.go:104` discards `conn.HandleFrames` errors silently.

Context usage:

- Minimal.
- `server.Run()` uses `context.WithTimeout` only for shutdown.
- `probe.Run()` and `bench.Run()` do not propagate a caller context through the full stack.
- Loopback H3 helpers do not use context at all.

Logging:

- Basic `log.Printf` only in [internal/server/server.go:58-79](/d:/Codebox/TritonProbe/internal/server/server.go:58).
- No structured logs, no levels, no request IDs, no sensitive-data filtering policy.

Configuration management:

- Reasonably clean for the current size.
- Precedence is effectively defaults -> YAML -> env -> CLI, matching README claims.
- Validation misses many declared fields: `RateLimit`, `TraceDir`, `AccessLog`, `DefaultTests`, `DefaultFormat`, `DownloadSize`, and `UploadSize` are not validated or fully consumed.
- `ServerConfig.Listen` is defined but unused by the live server path; `server.New()` binds only `ListenTCP` in [internal/server/server.go:34](/d:/Codebox/TritonProbe/internal/server/server.go:34).

Magic numbers / hardcoded values:

- Hardcoded CIDs in [internal/quic/transport/dialer.go:52-53](/d:/Codebox/TritonProbe/internal/quic/transport/dialer.go:52).
- Hardcoded short-header DCID length `8` in [internal/quic/transport/listener.go:78](/d:/Codebox/TritonProbe/internal/quic/transport/listener.go:78).
- Hardcoded 4KB H3 read buffer in [internal/h3/loopback.go:85-91](/d:/Codebox/TritonProbe/internal/h3/loopback.go:85).
- Hardcoded default flow-control windows in `stream.New()` and `connection.New()`.

TODO/FIXME/HACK inventory:

- `rg -n "TODO|FIXME|HACK" -S .` returned no matches.
- Lack of TODOs is not a sign of completeness here; the gaps are structural, not merely annotated.

### 3.2 Frontend Code Quality

There is no React or TypeScript frontend. The embedded UI is static HTML/CSS/JS:

- [internal/dashboard/assets/index.html](/d:/Codebox/TritonProbe/internal/dashboard/assets/index.html)
- [internal/dashboard/assets/app.css](/d:/Codebox/TritonProbe/internal/dashboard/assets/app.css)
- [internal/dashboard/assets/app.js](/d:/Codebox/TritonProbe/internal/dashboard/assets/app.js)

Assessment:

- The UI is a scaffold, not a product dashboard.
- `app.js` does three `fetch()` calls and dumps JSON into `<pre>` tags.
- No bundling, routing, component model, or state management.
- CSS is intentionally lightweight and readable, but it does not match the extensive branding/design system in `.project/BRANDING.md`.
- Accessibility is basic but incomplete: no live regions, no keyboard-specific affordances, and no status semantics for async-loaded content.

### 3.3 Concurrency & Safety

Goroutine lifecycle:

- `server.Run()` manages startup goroutines and shuts down on SIGINT/SIGTERM.
- `transport.ListenQUIC()` launches an unmanaged goroutine in [internal/quic/transport/listener.go:41](/d:/Codebox/TritonProbe/internal/quic/transport/listener.go:41); closure relies on socket close side effects.
- `bench.Run()` spawns worker goroutines with a wall-clock stop condition, no context cancellation, and no back-pressure.

Mutex/channel patterns:

- `stream.Manager` and `connection.Connection` use coarse mutexes and buffered channels.
- `stream.Stream` has an unsafe lock discipline: `state` is read under `sendMu` in [internal/quic/stream/stream.go:68-72](/d:/Codebox/TritonProbe/internal/quic/stream/stream.go:68) but mutated under both `sendMu` and `recvMu` in [internal/quic/stream/stream.go:109-148](/d:/Codebox/TritonProbe/internal/quic/stream/stream.go:109) and [internal/quic/stream/stream.go:166-188](/d:/Codebox/TritonProbe/internal/quic/stream/stream.go:166).

Race condition risks:

- `stream.Stream.state` lock inconsistency as above.
- `Listener.acceptCh` is used both for public `Accept()` and `WaitForConnections()`, so consumers can steal each other's events in future multi-consumer use.
- `Listener` silently drops accepted remote streams when `acceptQueue` is full in [internal/quic/stream/manager.go:94-96](/d:/Codebox/TritonProbe/internal/quic/stream/manager.go:94).

Resource leak risks:

- `signal.Notify` is not paired with `signal.Stop` in [internal/server/server.go:74-75](/d:/Codebox/TritonProbe/internal/server/server.go:74), minor but real.
- Bench mode never calls `CloseIdleConnections()` on its transport.
- File storage closes files correctly.

Graceful shutdown:

- Present only for server/dashboard.
- No graceful cancellation model for in-flight probes or benches triggered externally.

### 3.4 Security Assessment

Input validation:

- Route-level integer parsing exists, but there are no size limits or normalization for request bodies.
- `/echo` and `/upload` read arbitrary body sizes into memory or discard without upper bounds.

Injection resistance:

- No SQL or shelling out, so SQL injection and command injection are not present.
- Path handling in dashboard asset serving uses `path.Clean`, which is good.

XSS/frontend:

- UI only renders JSON via `textContent`, so current XSS risk is low.
- No CSP, HSTS, or hardening headers.

Secrets management:

- `.gitignore` excludes `.env*` and `/triton-data/`, which is good in principle.
- But the repo currently contains committed runtime artifacts under `triton-data/`, including `triton-data/certs/triton-selfsigned-key.pem`. Even if that key is only for local dev, committing private keys is bad practice.

TLS/HTTPS configuration:

- Server uses TLS 1.2 minimum, not TLS 1.3 minimum, in [internal/server/server.go:39-42](/d:/Codebox/TritonProbe/internal/server/server.go:39).
- Bench mode disables certificate verification unconditionally in [internal/bench/bench.go:56-69](/d:/Codebox/TritonProbe/internal/bench/bench.go:56).
- Probe mode makes verification optional via `--insecure`, which is fine, but the code exposes the dangerous path through `InsecureSkipVerify` in [internal/probe/probe.go:53-61](/d:/Codebox/TritonProbe/internal/probe/probe.go:53).

CORS/auth:

- No auth.
- No CORS policy.
- Dashboard security posture relies mostly on binding to localhost by default.

Known vulnerability patterns:

- Unbounded `io.ReadAll` on request bodies.
- Intentional TLS verification bypass in bench mode.
- Committed private key material.
- No access control on persisted result APIs.

## 4. Testing Assessment

### 4.1 Test Coverage

Measured test results on 2026-04-11:

- `go test ./... -count=1`: pass
- `go build ./cmd/triton`: pass
- `go vet ./...`: pass
- `staticcheck ./...`: pass
- `go test -race ./...`: failed to run because the environment reports `-race requires cgo; enable cgo by setting CGO_ENABLED=1`

Packages with zero test files:

- `cmd/triton`
- `internal/appmux`
- `internal/bench`
- `internal/cli`
- `internal/dashboard`
- `internal/probe`

Test types present:

- Unit tests: yes
- Small integration/loopback tests: yes
- Benchmarks: no `Benchmark...` tests
- Fuzz tests: none despite specification
- End-to-end external integration tests: none

Coverage reality:

- Good coverage on parsing/helpers and the toy loopback transport.
- Weak or absent coverage on real user flows: CLI UX, dashboard APIs, external probe failure modes, external bench correctness, signal handling, config precedence edge cases.
- Estimated effective coverage: **~45% of meaningful behavior**, much lower on production-critical paths.

### 4.2 Test Infrastructure

- Test helpers are lightweight and local to packages.
- No test fixtures directory beyond temp dirs used in tests.
- No CI pipeline exists in `.github/workflows/`; the directory is absent.
- No frontend tests.

## 5. Specification vs Implementation Gap Analysis

### 5.1 Feature Completion Matrix

| Planned Feature | Spec Section | Implementation Status | Files/Packages | Notes |
|---|---|---|---|---|
| Single binary CLI with `server`, `probe`, `bench` | SPEC §4, §15 | PARTIAL | `cmd/triton`, `internal/cli` | Commands exist, but CLI is much smaller than planned |
| Custom UDP/QUIC transport | SPEC §3, §4, §15 | PARTIAL | `internal/quic/transport`, `internal/quic/packet`, `internal/quic/frame` | Parsers and toy listener/dialer exist; no RFC-complete transport |
| TLS 1.3 over QUIC, packet protection, 0-RTT | SPEC §3, §4.2, §12 | MISSING | none | No QUIC TLS handshake or packet protection |
| Production HTTP/3 server/client | SPEC §2, §7 | MISSING | `internal/h3` | Only in-process loopback helpers; no network-capable H3 stack |
| Full server test endpoint suite | SPEC §4.1 | PARTIAL | `internal/appmux`, `internal/server` | 15 endpoints present; `/push/:n` absent; many are simulated |
| HTTP/1.1 + HTTP/2 + HTTP/3 comparison | SPEC §4.3 | PARTIAL | `internal/bench`, `internal/server` | Only H1/H2 benching; server is TCP/TLS only |
| Deep probe analysis | SPEC §4.2 | MOSTLY MISSING | `internal/probe` | Only HTTPS timing/TLS metadata and loopback path |
| Analytics engine / qlog / timeline | SPEC §3, §6 | MISSING | none | No analytics package at all |
| Embedded real-time dashboard | SPEC §3, §7, §9 | PARTIAL | `internal/dashboard`, assets | Read-only listing UI; no SSE, WS, charts, controls |
| Config layering and validation | SPEC §10 | PARTIAL | `internal/config` | Basic defaults, YAML, env, CLI present |
| Filesystem result persistence | SPEC §12 | COMPLETE | `internal/storage` | Save/load/list/cleanup implemented |
| Build matrix, release automation, CI | SPEC §14 | MISSING | `Makefile`, `Dockerfile` only | No workflows, goreleaser, or cross-build matrix |

### 5.2 Architectural Deviations

- **Spec says custom QUIC + TLS + H3 engine; implementation ships a TCP/TLS HTTP server.**
  Files: [internal/server/server.go](/d:/Codebox/TritonProbe/internal/server/server.go:26), [internal/probe/probe.go](/d:/Codebox/TritonProbe/internal/probe/probe.go:53), [internal/bench/bench.go](/d:/Codebox/TritonProbe/internal/bench/bench.go:56)
  Assessment: regression relative to product promise, but a practical short-term simplification.

- **Spec says HTTP/3 server/client; implementation provides loopback-only request translation helpers.**
  Files: [internal/h3/server.go](/d:/Codebox/TritonProbe/internal/h3/server.go:16), [internal/h3/loopback.go](/d:/Codebox/TritonProbe/internal/h3/loopback.go:17)
  Assessment: useful for incremental development, but naming overstates maturity.

- **Spec says one handler tree under `internal/server`; implementation split handlers into `internal/appmux` while leaving duplicate code behind.**
  Files: [internal/appmux/mux.go](/d:/Codebox/TritonProbe/internal/appmux/mux.go:15), [internal/server/endpoints.go](/d:/Codebox/TritonProbe/internal/server/endpoints.go:15)
  Assessment: regression; this is maintenance debt, not an improvement.

- **Spec promises analytics, qlog, charts, SSE, WS, and inspector views; none exist.**
  Assessment: large scope reduction.

- **Spec promises zero external dependencies plus specific allowed crypto/sys packages; implementation uses only YAML and does not attempt the planned crypto/sys layers.**
  Assessment: simplification by omission.

### 5.3 Task Completion Assessment

`TASKS.md` defines 142 tasks across 10 phases. A strict audit shows only a minority are fully complete.

Estimated phase completion:

| Phase | Status | Notes |
|---|---|---|
| Phase 1 - Scaffold & UDP Transport | MOSTLY PARTIAL | Core scaffold exists; Makefile/Dockerfile are minimal |
| Phase 2 - QUIC Packet Layer | PARTIAL | Varints, packet numbers, header parsing exist; many packet types absent |
| Phase 3 - QUIC Frames & Crypto | PARTIAL | Several frame types implemented; TLS/key schedule absent |
| Phase 4 - QUIC Connection & Streams | PARTIAL | Toy connection/stream model exists |
| Phase 5 - Loss Detection & Congestion | MISSING | Not implemented |
| Phase 6 - Migration & 0-RTT | MISSING | Only placeholder-ish path challenge handling |
| Phase 7 - HTTP/3 Layer | PARTIAL | Minimal loopback H3 only |
| Phase 8 - Probe & Bench Modes | PARTIAL | Basic HTTPS probe and H1/H2 bench only |
| Phase 9 - Web Dashboard | PARTIAL | Very small dashboard scaffold |
| Phase 10 - Polish & Release | PARTIAL | README improved, release engineering absent |

Weighted completion estimate:

- Fully complete tasks: roughly **20-25**
- Partial tasks: roughly **25-30**
- Missing tasks: roughly **90+**
- Weighted task completion: **~26%**

Remaining effort for incomplete items:

- Minimum viable "real Triton" matching the current spec direction: **10-14+ weeks** of focused engineering for one strong engineer, likely longer for production quality.

### 5.4 Scope Creep Detection

Items in code not explicitly central in the spec:

- `triton://loopback/...` probe scheme and in-process H3 loopback path in [internal/probe/probe.go:38-40](/d:/Codebox/TritonProbe/internal/probe/probe.go:38) and [internal/h3](/d:/Codebox/TritonProbe/internal/h3/server.go:1).
  Assessment: valuable. This is the best "incremental architecture" choice in the repo.

- Duplicate handler package `internal/appmux` alongside `internal/server/endpoints.go`.
  Assessment: unnecessary complexity.

- Committed runtime output under `triton-data/` and checked-in `triton.exe`.
  Assessment: unnecessary repository clutter, not product scope.

### 5.5 Missing Critical Components

Most critical missing components, prioritized:

1. Real QUIC TLS handshake, packet protection, and connection lifecycle.
2. Network-capable HTTP/3 server and client.
3. Real H3 benchmarking path and protocol comparison including `h3`.
4. Probe features promised in the spec: 0-RTT, migration, streams, throughput, Alt-Svc, QPACK, congestion, loss.
5. Production security controls: auth, safe TLS defaults, input bounds, secret hygiene.
6. CI/CD, release tooling, and deployment documentation.

## 6. Performance & Scalability

### 6.1 Performance Patterns

Observed hot paths and bottlenecks:

- `bench.Run()` performs repeated `client.Get()` loops with coarse metrics only; no latency histograms, no connection setup separation, and likely connection reuse hides protocol differences.
- `/delay` and `/drip` intentionally block with `time.Sleep`, which is fine for test endpoints but dangerous if used heavily.
- `/echo` reads the full body into memory in [internal/appmux/mux.go:48-56](/d:/Codebox/TritonProbe/internal/appmux/mux.go:48).
- `storage.List()` does glob + stat on every request, which is acceptable for small local data sets but not scalable.
- `internal/h3/loopback.go:85-91` reads at most 4096 bytes from a stream in one shot, so larger payloads are truncated in loopback flows.

Memory allocation patterns:

- QUIC varint/header/frame parsers allocate copied slices frequently; acceptable at current scale.
- UDP transport uses a `sync.Pool`, which is good.
- The H3 header codec converts through strings and joins/splits aggressively; fine for scaffolding, not for a high-throughput H3 stack.

Caching/compression/static optimization:

- No application-level caching.
- Static assets are embedded but not compressed or cache-controlled.
- Dashboard responses do not set cache headers or ETags.

### 6.2 Scalability Assessment

- Horizontal scalability: not really relevant yet; the app is mostly local-tooling oriented.
- State model: local filesystem state under `triton-data/`; not multi-instance friendly.
- No queue/worker abstraction.
- No explicit resource limits, back-pressure, or admission control.
- No real connection pooling controls beyond default `net/http` behavior.

## 7. Developer Experience

### 7.1 Onboarding Assessment

- Clone/build/run is easy.
- README is clear and unusually honest about what is implemented today.
- Requirements are minimal: Go 1.24 and Docker if needed.
- No hot reload, dev containers, or local orchestration scripts.

### 7.2 Documentation Quality

- `README.md` is strong and current.
- Planning docs under `.project/` are detailed but aspirational.
- Missing docs: `LICENSE`, `CONTRIBUTING.md`, `CHANGELOG.md`, `ARCHITECTURE.md`, `API.md`.
- No ADRs.

### 7.3 Build & Deploy

- Build process is simple.
- `Makefile` is too small for the promised release process: only `build`, `test`, `fmt`.
- Docker image is functional but not optimized:
  - uses `debian:bookworm-slim`, not distroless/scratch
  - does not expose UDP 4433 even though the spec and example config mention it
  - no non-root user
  - copies the whole repo into the build context
- No CI/CD pipeline.

## 8. Technical Debt Inventory

### Critical

- `internal/bench/bench.go:56-69`: TLS verification disabled by default in benchmark mode. Suggested fix: add a safe default and explicit `--insecure` override. Effort: 1-2h.
- `internal/server/server.go:32-42`: live server ignores `Server.Listen` and provides no QUIC/H3 path. Suggested fix: either implement QUIC or rename/re-scope product claims. Effort: large.
- `internal/appmux/mux.go` and `internal/server/endpoints.go`: duplicate endpoint implementations. Suggested fix: delete dead copy and centralize tests on one mux. Effort: 1h.
- `triton-data/certs/triton-selfsigned-key.pem`: private key committed in repo. Suggested fix: remove tracked runtime secrets and rotate local certs. Effort: 0.5h plus git hygiene.

### Important

- `internal/quic/stream/stream.go:68-188`: inconsistent locking around `state`. Suggested fix: single mutex or explicit lock ordering. Effort: 2-4h.
- `internal/h3/headers.go:9-42`: naive newline/colon-delimited header serialization is not QPACK and is not robust. Suggested fix: rename as test codec or replace with real H3 header handling. Effort: medium to large.
- `internal/h3/loopback.go:85-91`: single-read 4KB cap on stream reads. Suggested fix: loop until EOF. Effort: 0.5h.
- `internal/dashboard/server.go:25-128`: no auth, no methods enforcement, no tests. Suggested fix: add auth/bind restrictions/tests or keep explicitly dev-only. Effort: 4-8h.
- `Makefile`, `Dockerfile`: far below promised build/release ergonomics. Effort: 4-8h.

### Minor

- `internal/cli/output.go`: "table" output is just indented JSON, and "markdown" is JSON-in-fence rather than a true Markdown report.
- `internal/server/server.go:74-75`: `signal.Stop` not called.
- `README.md` includes some characters with encoding artifacts from terminal output, and planning docs contain mojibake from Windows console rendering.

## 9. Metrics Summary Table

| Metric | Value |
|---|---:|
| Total Go Files | 52 |
| Total Go LOC | 4,792 |
| Total Frontend Files | 3 |
| Total Frontend LOC | 92 |
| Test Files | 20 |
| Test Coverage (estimated) | 45% |
| External Go Dependencies | 1 direct |
| External Frontend Dependencies | 0 |
| Open TODOs/FIXMEs | 0 |
| API Endpoints | 23 total (15 server + 8 dashboard/assets) |
| Spec Feature Completion | ~25% |
| Task Completion | ~26% weighted |
| Overall Health Score | 4/10 |
