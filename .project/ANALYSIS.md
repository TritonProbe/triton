# Project Analysis Report

> Auto-generated comprehensive analysis of TritonProbe
> Generated: 2026-04-14
> Analyzer: Codex - Full Codebase Audit

## 1. Executive Summary

TritonProbe is a single-binary Go diagnostics tool for standing up a local HTTPS/HTTP3 test server, probing remote HTTP and HTTP/3 targets, benchmarking H1/H2/H3 behavior, and reviewing stored results through an embedded dashboard. The important architectural reality is that the supported HTTP/3 path is not the in-repo QUIC implementation from the original vision; it is `quic-go` wired through `internal/realh3`, while the custom `internal/quic/*` and `internal/h3/*` stacks are explicitly isolated behind the `triton lab` flow and treated as transport research.

Key metrics from the audited tree:

| Metric | Value |
|---|---|
| Total files in repo working tree | 10,190 |
| Practical project files excluding cache noise | ~100 |
| Total Go files | 91 |
| Total Go LOC | 13,369 |
| Test files | 51 |
| Markdown docs | 13 |
| Frontend framework files (`.tsx/.ts/.jsx`) | 0 |
| Embedded dashboard asset LOC | 786 |
| Dashboard backend LOC | ~580 |
| Go packages | 20 |
| External Go dependencies | 8 |

Overall health assessment: **7.5/10**. The supported product path is materially better than the original target-state docs suggest: `go build ./cmd/triton`, `go test ./... -count=1`, and `go vet ./...` all pass, package coverage is consistently non-zero, configuration hardening is thoughtful, and the dashboard/server surfaces are reasonably disciplined. The score is held back by the large documentation-to-implementation delta, the existence of a substantial but non-production in-repo transport stack, the use of heuristic "advanced" probe features where the spec promises packet-level truth, and a dashboard/UI scope that is much smaller than the promised live workbench.

Top strengths:

- Strong supported-path discipline: real HTTP/3 uses `quic-go`, while the lab transport is explicitly opt-in and warned as experimental in startup output (`internal/server/server.go:276`, `internal/server/server.go:279`).
- Good defensive configuration gates: remote dashboard, insecure TLS, experimental H3, and mixed H3 planes all require explicit operator intent (`internal/config/config.go:111-181`).
- Healthy test posture for a small codebase: 51 test files, parser fuzz targets, and per-package coverage mostly in the 70-90% range.

Top concerns:

- The target specification still overstates reality by a wide margin: custom QUIC-TLS, packet protection, real QPACK, live migration, live 0-RTT, SSE/WebSocket dashboard, and many planned packages do not exist.
- Several "advanced" probe features are approximations, not packet-level instrumentation; the code now labels them explicitly in the CLI, dashboard, and machine-readable `fidelity_summary`, but the underlying implementation is still not packet-level (`internal/probe/probe.go`, `internal/cli/output.go`, `internal/dashboard/assets/app.js`).
- CI promises are still slightly ahead of local ergonomics: local `go test -race ./...` failed here because CGO is disabled. `staticcheck ./...` is now clean locally after removing the unused test helper that initially triggered the warning.
- `SUPPORTED.md` now gives the repo a single current-state reference for supported runtime, dashboard scope, endpoint inventory, and probe fidelity semantics.
- `API.md` now documents the dashboard route set, error format, list query semantics, and typed summary surfaces from the running code.

## 2. Architecture Analysis

### 2.1 High-Level Architecture

This is a **modular monolith** with one CLI binary and three major product modes:

- `server`: run HTTPS/TCP plus optional real HTTP/3 and optional dashboard
- `probe`: inspect a target and persist a structured result
- `bench`: compare H1/H2/H3 behavior and persist a structured result
- `lab`: intentionally isolate the in-repo experimental transport stack

Text data flow:

```text
CLI -> config.Load/defaults/env/flags -> config.Validate
   -> mode dispatch
      -> server.New -> appmux handler chain -> HTTPS/TCP and/or quic-go HTTP/3
      -> probe.Run -> standard net/http, quic-go HTTP/3, or experimental triton UDP H3
      -> bench.Run -> protocol runners -> stored bench summaries
      -> dashboard.New -> read-only status/config/probe/bench/trace APIs
   -> storage.FileStore -> gzip JSON result persistence
```

Important boundary:

- **Supported path**: `internal/server`, `internal/appmux`, `internal/probe`, `internal/bench`, `internal/dashboard`, `internal/realh3`
- **Experimental path**: `internal/quic/*`, `internal/h3/*`

Concurrency model:

- `server.Run()` launches separate goroutines for HTTPS, real HTTP/3, experimental UDP H3, and dashboard listeners (`internal/server/server.go`).
- Bench/probe concurrency is worker-oriented and bounded.
- The experimental stack uses goroutines per listener/session/stream flow; it is functional enough for loopback and lab transport tests but not complex enough to justify production confidence.
- Graceful shutdown exists with a 10 second timeout (`internal/server/server.go`).

### 2.2 Package Structure Assessment

Package inventory and responsibility:

| Package | Responsibility | Assessment |
|---|---|---|
| `cmd/triton` | binary entrypoint, exit code handling | clean |
| `internal/cli` | command parsing, command execution, output formatting | cohesive but a bit large |
| `internal/config` | defaults, YAML/env loading, validation | strong and cohesive |
| `internal/storage` | gzip JSON result persistence and retention | cohesive |
| `internal/appmux` | shared HTTP handler surface for server/lab/probe targets | cohesive |
| `internal/server` | listener orchestration, certificates, middleware, rate limiting | good separation |
| `internal/dashboard` | embedded dashboard assets and read-only APIs | cohesive; improved after modular split |
| `internal/observability` | request IDs, access logging, qlog helpers | cohesive |
| `internal/probe` | probing orchestration, analysis models, support summaries | too much in one file |
| `internal/bench` | benchmark orchestration and summarization | acceptable |
| `internal/realh3` | `quic-go` HTTP/3 client wrapper | small and clean |
| `internal/h3` | custom experimental HTTP/3-like protocol over custom transport | lab-only |
| `internal/h3/frame` | experimental H3 frame encoding | small and clear |
| `internal/quic/packet` | packet header/varint/PN parsing | simplified |
| `internal/quic/frame` | experimental frame parsing/serialization | simplified |
| `internal/quic/stream` | stream and manager | simplified but cohesive |
| `internal/quic/connection` | connection state and frame dispatch | simplified |
| `internal/quic/transport` | UDP transport, listener, dialer, sessions | simplified |
| `internal/quic/wire` | packet/frame assembly helpers | cohesive |
| `internal/testutil` | test servers and self-signed certs | good |

Package cohesion notes:

- Best-separated packages: `config`, `storage`, `observability`, `realh3`.
- Probe cohesion improved materially: orchestration remains in [`internal/probe/probe.go`](../internal/probe/probe.go), while models, support/fidelity logic, and analytics helpers now live in dedicated files under `internal/probe/`.
- Dashboard cohesion improved materially too: routing/orchestration remains in [`internal/dashboard/server.go`](../internal/dashboard/server.go), while models, summary/cache logic, and list/query helpers now live in dedicated files under `internal/dashboard/`.
- `internal/cli` is serviceable, but command parsing, option structs, and formatting could be split more clearly if the CLI expands.

Circular dependency risk:

- No active circular dependency issue was found.
- The codebase generally keeps infra packages below orchestration packages.

Internal vs `pkg` separation:

- Good. There is no premature public API surface.
- This repo is clearly application-first rather than library-first.

### 2.3 Dependency Analysis

#### Go dependencies

From `go.mod`:

| Dependency | Version | Purpose | Assessment |
|---|---|---|---|
| `github.com/quic-go/quic-go` | `v0.59.0` | supported HTTP/3 client/server | critical and justified |
| `gopkg.in/yaml.v3` | `v3.0.1` | config loading | justified |
| `github.com/quic-go/qpack` | `v0.6.0` | indirect via `quic-go` | expected |
| `golang.org/x/crypto` | `v0.41.0` | TLS/crypto helpers transitively | expected |
| `golang.org/x/net` | `v0.43.0` | HTTP/transport internals transitively | expected |
| `golang.org/x/sys` | `v0.35.0` | system calls transitively | expected |
| `golang.org/x/text` | `v0.28.0` | text handling transitively | expected |
| `github.com/kr/text` | `v0.2.0` | indirect dependency | harmless but low-value direct awareness |

Dependency hygiene:

- Excellent by count. Eight total dependencies is small for a tool of this scope.
- The original zero-dependency aspiration is no longer true, but the current dependency set is pragmatic and sane.
- The biggest strategic dependency is `quic-go`; this is a deliberate architectural pivot and, in production terms, the correct one.
- No obvious unused direct dependency was found.
- No dependency audit against live CVE feeds was performed in this offline code audit.

#### Frontend dependencies

- No `package.json`, `web/package.json`, `ui/package.json`, or `frontend/package.json` exists.
- The dashboard is plain embedded `index.html`, `app.css`, and `app.js`, backed by a small read-only API layer plus dedicated models and summary/list helper files.
- External frontend dependency count: **0**.

### 2.4 API & Interface Design

#### Test server endpoint inventory

Routes are registered in [`internal/appmux/mux.go`](../internal/appmux/mux.go):

| Method | Path | Registration |
|---|---|---|
| `GET` | `/` | `internal/appmux/mux.go:54` |
| `GET` | `/healthz` | `internal/appmux/mux.go:57` |
| `GET` | `/readyz` | `internal/appmux/mux.go:58` |
| `GET` | `/metrics` | `internal/appmux/mux.go:59` |
| `GET` | `/ping` | `internal/appmux/mux.go:60` |
| `GET,POST` | `/echo` | `internal/appmux/mux.go:61` |
| `GET` | `/download/:size` | `internal/appmux/mux.go:64` |
| `POST` | `/upload` | `internal/appmux/mux.go:65` |
| `GET` | `/delay/:ms` | `internal/appmux/mux.go:68` |
| `GET` | `/redirect/:n` | `internal/appmux/mux.go:69` |
| `GET` | `/streams/:n` | `internal/appmux/mux.go:70` |
| `GET` | `/headers/:n` | `internal/appmux/mux.go:71` |
| `GET` | `/status/:code` | `internal/appmux/mux.go:72` |
| `GET` | `/drip/:size/:delay` | `internal/appmux/mux.go:73` |
| `GET` | `/tls-info` | `internal/appmux/mux.go:74` |
| `GET` | `/quic-info` | `internal/appmux/mux.go:75` |
| `GET` | `/migration-test` | `internal/appmux/mux.go:76` |
| `GET` | `/.well-known/triton` | `internal/appmux/mux.go:77` |

#### Dashboard/API inventory

Routes are registered in [`internal/dashboard/server.go`](../internal/dashboard/server.go):

| Method | Path | Registration |
|---|---|---|
| `GET` | `/api/v1/status` | `internal/dashboard/server.go` |
| `GET` | `/api/v1/config` | `internal/dashboard/server.go` |
| `GET` | `/api/v1/probes` | `internal/dashboard/server.go` |
| `GET` | `/api/v1/benches` | `internal/dashboard/server.go` |
| `GET` | `/api/v1/probes/:id` | `internal/dashboard/server.go` |
| `GET` | `/api/v1/benches/:id` | `internal/dashboard/server.go` |
| `GET` | `/api/v1/traces` | `internal/dashboard/server.go` |
| `GET` | `/api/v1/traces/meta/:name` | `internal/dashboard/server.go` |
| `GET` | `/api/v1/traces/:name` | `internal/dashboard/server.go` |

API consistency assessment:

- Server endpoints are simple and intentionally ad hoc.
- Dashboard APIs return structured JSON and mostly sanitize error details well.
- List APIs support `q`, `sort`, `limit`, `offset`, and `view=summary`, which is a useful operator-focused step toward safer browsing at larger result volumes.
- There is no formal OpenAPI schema.

Authentication/authorization:

- Dashboard supports optional HTTP Basic Auth.
- Remote dashboard bind is blocked unless auth is configured and explicitly allowed (`internal/config/config.go:144-147`).
- Main test server endpoints are unauthenticated by design.

Rate limiting, CORS, validation:

- Main server has a simple IP-based rate limiter in middleware.
- Dashboard has no separate rate limiter.
- CORS is not opened broadly; this is safer than the original spec.
- Path and query validation is generally defensive on server/dashboard routes.

## 3. Code Quality Assessment

### 3.1 Go Code Quality

Style consistency:

- Source is broadly `gofmt`-clean.
- Naming is clear.
- Function size is reasonable across most packages after the recent `internal/probe` and `internal/dashboard` modular splits.

Error handling:

- Error returns are consistent and idiomatic.
- Validation errors are explicit and useful.
- Shutdown combines errors with `errors.Join`, which is a good production pattern.

Context usage:

- HTTP server shutdown uses context correctly.
- Request-scoped context propagation is present on standard HTTP client traces.
- The experimental transport stack is much lighter on context/cancellation discipline.

Logging:

- Structured JSON access logging is implemented in `internal/observability/logger.go`.
- Request IDs are injected consistently.
- Startup summary logging is informative and honest about experimental mode.

Configuration management:

- One of the strongest parts of the repo.
- Precedence is clean: defaults -> YAML -> env -> CLI.
- The safety gates in [`internal/config/config.go`](../internal/config/config.go) are production-minded and unusually explicit for a small tool.

Magic numbers and hardcoded values:

- A few internal constants are hardcoded, such as 10 second shutdown timeout in `internal/server/server.go`, fixed listener/request timeout values in the dashboard, and simplified experimental transport defaults.
- These are not egregious, but they are worth documenting or exposing if the product grows.

TODO/FIXME/HACK inventory:

- `rg -n "TODO|FIXME|HACK"` returned no matches.
- The repo relies more on roadmap docs than inline debt markers.

### 3.2 Frontend Code Quality

There is no React or TypeScript frontend. The actual UI is:

- `internal/dashboard/assets/index.html` - 97 LOC
- `internal/dashboard/assets/app.css` - 149 LOC
- `internal/dashboard/assets/app.js` - 540 LOC

Assessment:

- The dashboard is a plain JS embedded admin view, not the richer SPA promised by the spec.
- Good points:
  - zero build step
  - no frontend dependency attack surface
  - HTML escaping is used before inserting most values
  - responsive-enough card/grid layout
- Weak points:
  - no component model
  - no automated frontend test harness
  - plain string-template rendering makes long-term maintenance harder
  - no serious accessibility work beyond semantic basics

Accessibility:

- Inputs/selects are present but there is little explicit labeling, ARIA work, focus management, or keyboard UX refinement.
- Acceptable for an internal dashboard, below production-grade for a public UI.

### 3.3 Concurrency & Safety

Supported path:

- Reasonable.
- HTTP servers rely mostly on battle-tested stdlib and `quic-go`.
- Bench/probe concurrency is straightforward.

Experimental transport path:

- More fragile by design.
- The listener handshake is intentionally simplified, with a server reply built from `MaxDataFrame` and `HandshakeDoneFrame` rather than real QUIC-TLS negotiation (`internal/quic/transport/listener.go:137-138`).
- Auto-echo behavior and simplified session flow make this transport useful as a lab harness, not as a real QUIC engine.

Race condition risk:

- No race failure was reproduced locally because `go test -race ./...` could not run with `CGO_ENABLED=0`.
- The docs claim a dedicated race CI job exists, which is good, but local reproducibility is still limited in this environment.

Resource leak risk:

- Supported path looks acceptable.
- Experimental goroutine/session handling is test-covered, but still lower confidence than the stdlib/`quic-go` path.

Graceful shutdown:

- Implemented and sane in `internal/server/server.go`.
- Dashboard and HTTP/3 shutdown are included.

### 3.4 Security Assessment

Input validation:

- Stronger than average for this project size.
- Endpoint path parsing rejects invalid ranges and malformed shapes.
- Storage rejects category and ID traversal attempts.

Injection risks:

- No SQL layer exists.
- No shelling-out from request paths exists.
- Dashboard rendering escapes HTML before insertion.

Secrets management:

- No hardcoded credentials found.
- TLS/dashboard credentials are config-driven.
- The repo now includes an MIT `LICENSE` file, which resolves the legal/documentation gap noted during the initial audit pass.

TLS/HTTPS:

- HTTPS/TCP uses TLS 1.2 minimum.
- Real HTTP/3 uses TLS 1.3 minimum.
- Probe/bench insecure TLS usage is explicitly gated (`internal/config/config.go:172`, `internal/config/config.go:181`).

CORS/security headers:

- Server security headers include `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `Cache-Control`, and a restrictive CSP (`internal/server/server.go:369`).
- Dashboard inherits similar middleware patterns.
- No permissive wildcard CORS setup was found.

Known vulnerability patterns:

- Biggest security concern is operator confusion, not exploit code: the repo still contains a substantial pseudo-QUIC stack that could be mistaken for production if the docs are skimmed carelessly.

## 4. Testing Assessment

### 4.1 Test Coverage

Counts:

- Source Go files: 91
- Test files: 51
- Test/source file ratio: 56%

Runtime verification:

- `go build ./cmd/triton` -> passed
- `go test ./... -count=1` -> passed
- `go vet ./...` -> passed
- `go test ./... -cover` -> passed for every package
- `go test -race ./...` -> failed locally because CGO is disabled
- `staticcheck ./...` -> passed

Package coverage from `go test ./... -cover`:

| Package | Coverage |
|---|---|
| `cmd/triton` | 85.7% |
| `internal/appmux` | 91.6% |
| `internal/bench` | 88.9% |
| `internal/cli` | 61.2% |
| `internal/config` | 83.9% |
| `internal/dashboard` | 70.8% |
| `internal/h3` | 78.0% |
| `internal/h3/frame` | 73.7% |
| `internal/observability` | 69.4% |
| `internal/probe` | 74.8% |
| `internal/quic/connection` | 60.2% |
| `internal/quic/frame` | 72.4% |
| `internal/quic/packet` | 79.0% |
| `internal/quic/stream` | 88.6% |
| `internal/quic/transport` | 79.6% |
| `internal/quic/wire` | 72.4% |
| `internal/realh3` | 100.0% |
| `internal/server` | 68.8% |
| `internal/storage` | 83.1% |
| `internal/testutil` | 81.1% |

Packages with zero test coverage:

- None.

Test types present:

- Unit tests
- Integration tests
- Fuzz tests for packet/frame parsing
- Real HTTP/3 integration tests

Missing test types:

- No benchmarks in the Go testing sense (`BenchmarkXxx`)
- No browser/E2E UI tests
- No load tests beyond application-level bench mode

### 4.2 Test Infrastructure

Strengths:

- Self-signed cert helpers
- HTTP/3 test server helper
- Good use of table-free but readable tests
- Coverage across the custom transport lab stack is unexpectedly broad

Weak spots:

- No dedicated fixture directory for richer end-to-end scenarios
- No frontend test harness
- Staticcheck failure in tests means the quality gate is not perfectly aligned with local code

CI pipeline maturity:

- Present in `.github/workflows/ci.yml` and `.github/workflows/release.yml`.
- Docs and workflow naming suggest CI covers tests, vet, staticcheck, gosec, smoke, and release automation.
- This is good maturity for the current repo size.

## 5. Specification vs Implementation Gap Analysis

This is the most important section. The short version: **the codebase is materially more useful than the old target-state backlog suggests, but materially less complete than the target-state architecture documents claim.**

### 5.1 Feature Completion Matrix

| Planned Feature | Spec Reference | Implementation Status | Files/Packages | Notes |
|---|---|---|---|---|
| CLI with `server`, `probe`, `bench`, `version` | Spec §4, Tasks Phase 1 | ✅ Complete | `internal/cli`, `cmd/triton` | Also includes `lab`, which is not a major original spec command |
| Config precedence defaults/YAML/env/CLI | Spec §11 | ✅ Complete | `internal/config` | One of the cleanest parts of the repo |
| Gzip JSON result persistence | Spec §12 | ✅ Complete | `internal/storage` | Includes retention and max-results cleanup |
| HTTPS/TCP test server | Spec §4.1 | ✅ Complete | `internal/server`, `internal/appmux` | Supports health/readiness/metrics and diagnostics routes |
| Real HTTP/3 support | Current baseline addendum | ✅ Complete | `internal/realh3`, `internal/server`, `internal/probe`, `internal/bench` | Delivered through `quic-go`, not custom QUIC |
| Experimental custom UDP H3 | Spec custom-engine track | ⚠️ Partial | `internal/quic`, `internal/h3` | Functional for loopback/lab, not RFC-complete |
| Dashboard status/config/probe/bench/trace APIs | Spec §9 baseline subset | ✅ Complete | `internal/dashboard` | Read-only operator API, not full control plane |
| Dashboard compare/trend/filter/sort UX | Spec §9 partial | ✅ Complete | `internal/dashboard/assets/app.js` | Useful but much smaller than promised live workbench |
| Probe handshake/TLS/latency/throughput/streams | Spec §8 | ✅ Complete | `internal/probe` | Supported on standard and real H3 targets |
| Probe 0-RTT | Spec §8.2 | ⚠️ Partial | `internal/probe/probe.go` | Resumption timing check, not true early data; code says so |
| Probe migration | Spec §8.3 | ⚠️ Partial | `internal/probe/probe.go` | Endpoint capability check, not live path rebinding |
| Probe QPACK analysis | Spec §8 | ⚠️ Partial | `internal/probe/probe.go` | Header-block estimate only |
| Probe loss/congestion/version/retry/ECN/spin-bit telemetry | Spec §8 | ⚠️ Partial | `internal/probe/probe.go` | Heuristic approximations, not packet-level |
| H1/H2/H3 benchmark comparison | Spec §5 | ✅ Complete | `internal/bench` | Includes summaries and protocol health rollups |
| Percentiles/phase timings/error summaries | Spec §5 | ✅ Complete | `internal/bench`, `internal/cli/output.go` | Better than early docs implied |
| SSE live updates | Spec §9 | ❌ Missing | none | No SSE hub exists |
| WebSocket dashboard control plane | Spec §9 | ❌ Missing | none | No WebSocket endpoint exists |
| Packet inspector/timeline/charts dashboard | Spec §9 | ❌ Missing | none | Current UI is static-card operator view |
| Custom QUIC-TLS handshake and packet protection | Spec §1, §12 | ❌ Missing | none | Custom transport does not implement real QUIC-TLS |
| Real QPACK codec | Spec §2.1 | ❌ Missing | none | Experimental header encoder is newline-delimited, not QPACK |
| ACME automation | Spec §3.3 | ❌ Missing | none | Server uses self-signed or configured certs only |
| Server push endpoint `/push/:n` | Spec §4.1 | ❌ Missing | none | Route absent |
| Compare/inspect/certs CLI subcommands | Spec §8 CLI vision | ❌ Missing | none | Current CLI is smaller |

### 5.2 Architectural Deviations

Major deviations:

1. **Custom QUIC/H3 replaced by `quic-go` for supported HTTP/3.**
   - Improvement for production readiness.
   - Regression only relative to the original "build everything ourselves" ambition.
   - Implemented intentionally and documented in README/ARCHITECTURE/ENGINE_STRATEGY.

2. **The custom transport remains in-repo but is sandboxed behind `triton lab`.**
   - Improvement over deleting it or pretending it is stable.
   - This makes the architecture dual-track instead of pure custom-stack.

3. **Advanced probe features are surfaced with explicit support and fidelity summaries rather than omitted.**
   - Mixed outcome.
   - Better UX than pretending the features do not exist.
   - The recent labeling work materially reduces misread risk, but the implementation is still heuristic for several fields.

4. **Dashboard is an operator overview, not a live control-plane workbench.**
   - Sensible scope reduction.
   - Large deviation from the specification.

### 5.3 Task Completion Assessment

`TASKS.md` still contains the original 142-task target backlog and explicitly says unchecked items are roadmap targets, not proof of emptiness.

My assessment against that backlog:

| Status | Estimated Count | Notes |
|---|---|---|
| Complete | ~52 | CLI, config, storage, supported server/probe/bench, dashboard read APIs, CI, fuzzing, real H3 path |
| Partial | ~20 | experimental QUIC/H3, advanced probe features, dashboard richness, release hardening |
| Missing | ~70 | custom QUIC-TLS, real QPACK, migration engine, SSE/WS dashboard, ACME, many target-state packages |

Estimated task completion: **~37% complete, ~14% partial, ~49% missing**.

### 5.4 Scope Creep Detection

Features present that were not primary original scope items:

- `triton lab` as a dedicated experimental transport boundary
- explicit mixed-plane safety gating
- richer support-summary UX for advanced probe tests
- compare/trend dashboard panel on top of stored summaries
- strong operator safety validation around remote dashboard and insecure probe/bench

Assessment:

- This is mostly **valuable scope creep**.
- The project added safety and observability improvements, not random complexity.

### 5.5 Missing Critical Components

Most critical missing pieces relative to the specification:

1. Real custom QUIC-TLS, packet protection, and RFC-level transport correctness
2. Real QPACK implementation
3. Live migration and true 0-RTT packet-level support
4. Full dashboard workbench with SSE/WebSocket/live charts
5. ACME and broader production certificate automation
6. Formal API/docs completeness

If the project keeps the current strategic direction, items 1-3 may no longer be v1 blockers for the product track, but they are still blockers for the original spec.

## 6. Performance & Scalability

### 6.1 Performance Patterns

Supported path:

- Good use of stdlib HTTP and `quic-go`.
- Gzip result storage is fine for current scale.
- Bench/probe summaries avoid over-complicated data pipelines.

Potential bottlenecks:

- [`internal/probe/probe.go`](../internal/probe/probe.go) does a lot of serial orchestration and repeated request sampling in one package; maintainability is a bigger issue than raw speed.
- Dashboard list and trace views are still filesystem-backed, but the current implementation is healthier than the initial audit pass: list APIs now support offset pagination, probe/bench summary views can omit heavier raw payload fields, probe/bench runs now persist compact summary sidecars plus category manifest indexes for restart-friendly first-list performance, dashboard summary generation is cached in-process to reduce repeated gzip/decode work, repeated query/filter/sort/view combinations are cached in-process at the dashboard layer, the storage layer caches category listings behind directory metadata checks to cut repeat `glob/stat` work, and the backend no longer concentrates all handler/models/helper logic in one file.
- The frontend pulls JSON endpoints directly and renders large strings in one pass; acceptable for current size.

### 6.2 Scalability Assessment

Horizontal scaling:

- The test server itself is mostly stateless aside from local storage.
- Dashboard and result persistence are filesystem-local, so scaling the whole product horizontally would need shared storage or externalization.

State model:

- Local filesystem state for probes/benches/traces/certs.
- No database, queue, or distributed coordination layer.

Back-pressure:

- Simple IP rate limit on main server.
- No sophisticated connection shedding or queue management.

Verdict:

- Fine for operator tooling and local/small deployment.
- Not architected for large distributed fleet service.

## 7. Developer Experience

### 7.1 Onboarding Assessment

Good:

- README is substantial.
- `make` targets and Dockerfile exist.
- Example config exists.
- One binary, low dependency footprint.

Less good:

- The documentation stack is large and inconsistent.
- A newcomer can easily confuse target-state docs with implemented reality.
- The repo contains previous generated analysis docs, which is useful historically but adds noise.

### 7.2 Documentation Quality

Strengths:

- There is a lot of documentation.
- README and ARCHITECTURE are reasonably aligned with current strategy.
- The current-state addenda in spec/task docs are doing important repair work.

Weaknesses:

- The root `LICENSE` file now exists and matches the MIT claims in project docs.
- Some docs still describe packages and subsystems that do not exist.
- There is no formal API schema.

### 7.3 Build & Deploy

Build process:

- Simple and workable.
- Dockerfile and `.goreleaser.yml` exist.
- CI/release workflow presence is a good sign.

Cross-compilation:

- Supported via Makefile and release config.

Container readiness:

- Present, but this is still operator-tool quality rather than "internet-facing SaaS" quality.

## 8. Technical Debt Inventory

### 🔴 Critical

- `Specification drift across core architecture docs`
  - Location: `.project/SPECIFICATION.md`, `.project/IMPLEMENTATION.md`, `.project/TASKS.md`
  - Problem: target-state docs still describe many non-existent packages and capabilities.
  - Fix: split "supported product architecture" and "research roadmap" into separate authoritative docs.
  - Effort: 6-10h

- `Advanced probe implementation still below advertised transport fidelity`
  - Location: `internal/probe/probe.go`, `internal/cli/output.go`, `internal/dashboard/assets/app.js`
  - Problem: 0-RTT, migration, QPACK, retry, ECN, spin-bit, loss, and congestion are still estimators or contract checks even though the UX now labels them honestly.
  - Fix: either keep narrowing claims or replace heuristics with direct transport telemetry over time.
  - Effort: 12-40h depending on how much packet-level work is attempted


### 🟡 Important

- `Probe orchestration is still central even after modular split`
  - Location: `internal/probe/probe.go`
  - Problem: the package is much healthier now, but orchestration for standard/H3/loopback/remote probe paths is still concentrated in one file.
  - Fix: if the package grows again, split transport runners from orchestration entrypoints.
  - Effort: 4-8h

- `Experimental transport may be mistaken for production`
  - Location: `internal/quic/*`, `internal/h3/*`, CLI/docs
  - Problem: in-repo presence plus extensive tests can create false confidence.
  - Fix: stronger naming or package-level lab banners in README/CLI output/docs.
  - Effort: 2-4h

- `No formal frontend tests`
  - Location: dashboard assets
  - Problem: operator UI has no regression harness.
  - Fix: add minimal API-contract and HTML rendering smoke coverage.
  - Effort: 4-8h


### 🟢 Minor

- `Dashboard accessibility is basic`
  - Location: `internal/dashboard/assets/*`
  - Fix: labels, focus handling, better semantics.

- `Some runtime constants are fixed in code`
  - Location: `internal/server/server.go`, `internal/dashboard/server.go`
  - Fix: document or expose them via config.

- `Coverage profile file checked into root adds noise`
  - Location: `coverage`
  - Fix: move generated artifacts to ignored paths.

## 9. Metrics Summary Table

| Metric | Value |
|---|---|
| Total Go Files | 91 |
| Total Go LOC | 13,369 |
| Total Frontend Framework Files | 0 |
| Embedded Dashboard Asset LOC | 786 |
| Test Files | 51 |
| Test Coverage (estimated) | ~77% average package coverage |
| External Go Dependencies | 8 |
| External Frontend Dependencies | 0 |
| Open TODOs/FIXMEs | 0 |
| Test Server Endpoints | 18 |
| Dashboard JSON Endpoints | 9 |
| Spec Feature Completion | ~44% against target-state spec |
| Task Completion | ~37% complete |
| Overall Health Score | 7.5/10 |
