# Triton Architecture

This document describes Triton's current architecture as implemented in this repository.
It is intentionally current-state oriented. Future RFC-complete QUIC and HTTP/3 goals are useful context, but deployment and support decisions should follow the supported runtime boundary below.

Current-state authority order:

1. [SUPPORTED.md](SUPPORTED.md)
2. This file
3. [CONFIG.md](CONFIG.md), [API.md](API.md), and [OPERATIONS.md](OPERATIONS.md)
4. Future-looking project notes under `.project/`

## Executive View

Triton is a single Go binary for HTTP/3 and QUIC-oriented diagnostics. The binary exposes four operator-facing workflows plus one lab workflow:

- `triton server`: HTTPS/TCP test server, optional real HTTP/3, optional dashboard
- `triton probe`: target inspection and structured result generation
- `triton bench`: protocol benchmarking across H1, H2, and H3
- `triton check`: reusable probe plus bench verification for profiles and CI
- `triton lab`: isolated experimental in-repo UDP H3 runtime

The supported HTTP/3 implementation uses `quic-go` through `internal/realh3`.
The in-repo `internal/quic` and `internal/h3` packages are lab-only protocol-building blocks.

```mermaid
flowchart LR
    User[Operator or CI] --> CLI[cmd/triton]
    CLI --> App[internal/cli]
    App --> Config[internal/config]
    Config --> Mode{Command}

    Mode -->|server| Server[internal/server]
    Mode -->|probe| Probe[internal/probe]
    Mode -->|bench| Bench[internal/bench]
    Mode -->|check| Check[Probe + Bench verifier]
    Mode -->|lab| Lab[Experimental UDP H3 only]

    Server --> AppMux[internal/appmux]
    Server --> Dashboard[internal/dashboard]
    Probe --> Store[internal/storage]
    Bench --> Store
    Check --> Store
    Dashboard --> Store

    Server --> RealH3[internal/realh3 via quic-go]
    Probe --> RealH3
    Bench --> RealH3

    Lab --> CustomH3[internal/h3]
    CustomH3 --> CustomQUIC[internal/quic]
```

## Product Boundary

Triton has three transport planes in the repository, but only two are part of the supported production-like runtime.

```mermaid
flowchart TB
    subgraph Supported["Supported Runtime"]
        HTTPS["HTTPS/TCP server<br/>server.listen_tcp<br/>http/1.1 + h2"]
        QGO["Real HTTP/3<br/>server.listen_h3<br/>quic-go"]
        Dash["Embedded dashboard<br/>loopback by default"]
    end

    subgraph LabOnly["Lab-Only Runtime"]
        UDPH3["Experimental Triton UDP H3<br/>server.listen"]
        LabCmd["triton lab"]
        TritonURL["triton:// targets"]
        InRepo["internal/quic + internal/h3"]
    end

    HTTPS --> AppMux["Shared appmux endpoints"]
    QGO --> AppMux
    UDPH3 --> AppMux
    Dash --> Store["Filesystem result store"]

    LabCmd --> UDPH3
    TritonURL --> UDPH3
    UDPH3 --> InRepo
```

| Plane | Entry | Implementation | Status | Intended Use |
|---|---|---|---|---|
| HTTPS/TCP | `server.listen_tcp` | Go `net/http` with TLS | Supported | Normal server runtime, H1/H2 tests |
| Real HTTP/3 | `server.listen_h3`, `h3://`, H3 bench | `quic-go` through `internal/realh3` | Supported | Real HTTP/3 diagnostics |
| Dashboard | `server.dashboard` | Embedded static UI plus JSON API | Supported operator surface | Local or authenticated remote inspection |
| Triton UDP H3 | `server.listen`, `triton lab`, `triton://` | In-repo QUIC/H3 scaffold | Lab-only | Protocol research and loopback experiments |

## Repository Map

```text
.
|-- cmd/triton              CLI entrypoint and build-info wiring
|-- internal/cli            Command parsing, mode orchestration, output, reports
|-- internal/config         Defaults, YAML/env/flag layering, validation, profiles
|-- internal/server         Runtime listeners, TLS, dashboard startup, shutdown
|-- internal/appmux         Shared HTTP test endpoints, metrics, health/readiness
|-- internal/realh3         quic-go HTTP/3 client wiring
|-- internal/probe          Probe execution, fidelity metadata, analytics
|-- internal/bench          Benchmark workers, phase timing, protocol summaries
|-- internal/dashboard      Embedded UI, dashboard API, list/detail caches
|-- internal/storage        Filesystem gzip JSON persistence and summary indexes
|-- internal/observability  Request IDs, access logging, qlog trace integration
|-- internal/runid          Collision-safe probe and bench IDs
|-- internal/h3             Experimental minimal H3 frame/request dispatch
|-- internal/quic           Experimental QUIC packet/frame/stream/transport blocks
|-- scripts                 Local and CI helper scripts
|-- triton.yaml.example     Baseline configuration example
```

## Runtime Modes

```mermaid
flowchart TD
    Args["os.Args[1:]"] --> Main["cmd/triton/main.go"]
    Main --> BuildInfo["internal/buildinfo.Set(version, buildTime)"]
    BuildInfo --> CLI["cli.NewApp(...).Run(args)"]

    CLI --> Version["version"]
    CLI --> Server["server"]
    CLI --> Lab["lab"]
    CLI --> Probe["probe"]
    CLI --> Bench["bench"]
    CLI --> Check["check"]

    Server --> LoadServer["load config + storage"]
    LoadServer --> ValidateServer["cfg.Validate"]
    ValidateServer --> ServerNew["server.New"]
    ServerNew --> ServerRun["Server.Run"]

    Probe --> LoadProbe["load config + profile + flags"]
    LoadProbe --> ProbeRun["probe.Run"]
    ProbeRun --> SaveProbe["SaveProbe + SaveProbeSummary"]

    Bench --> LoadBench["load config + profile + flags"]
    LoadBench --> BenchRun["bench.Run"]
    BenchRun --> SaveBench["SaveBench + SaveBenchSummary"]

    Check --> ResolveProfiles["resolve probe/bench profiles"]
    ResolveProfiles --> RunBoth["run selected probe and/or bench"]
    RunBoth --> Verdict["combined pass/fail + optional reports"]
```

| Command | Primary Packages | Persistent Output | Notes |
|---|---|---|---|
| `server` | `internal/server`, `internal/appmux`, `internal/dashboard` | Runtime certs, traces, result reads | Long-running listener process |
| `lab` | `internal/server`, `internal/h3`, `internal/quic` | Optional traces/results through shared paths | Forces experimental listener, disables supported dashboard/TCP/H3 |
| `probe` | `internal/probe`, `internal/storage`, `internal/dashboard` summaries | Probe gzip JSON and summary JSON | Supports `https://`, `h3://`, and lab-only `triton://` |
| `bench` | `internal/bench`, `internal/storage`, `internal/dashboard` summaries | Bench gzip JSON and summary JSON | Supports H1/H2/H3 comparisons |
| `check` | `internal/cli`, `internal/probe`, `internal/bench` | Probe/bench results plus optional reports | Profile-based CI gate |

## Configuration Architecture

Configuration is merged in a strict, fail-fast order:

```mermaid
flowchart LR
    Defaults["Built-in defaults<br/>config.Default"] --> YAML["YAML config<br/>KnownFields(true)"]
    YAML --> Env["Environment variables<br/>typed parsing"]
    Env --> Flags["CLI flags"]
    Flags --> Profiles["Optional named profiles"]
    Profiles --> Validate["Config.Validate"]
    Validate --> Runtime["Server / Probe / Bench / Check"]
```

Important properties:

- Unknown YAML fields are rejected.
- Invalid typed environment variables fail startup.
- `probe.insecure` and `bench.insecure` require explicit `allow_insecure_tls`.
- `server.listen` is experimental and requires `allow_experimental_h3`.
- Non-loopback experimental binds require `allow_remote_experimental_h3`.
- Mixed real HTTP/3 plus experimental UDP H3 requires `allow_mixed_h3_planes`.
- Remote dashboard access requires `allow_remote_dashboard`, credentials, and explicit TLS files.

```mermaid
stateDiagram-v2
    [*] --> ConfigLoaded
    ConfigLoaded --> Invalid: unknown YAML / bad env / bad flag
    ConfigLoaded --> ValidateListeners
    ValidateListeners --> Invalid: no listener
    ValidateListeners --> Invalid: unsafe experimental bind
    ValidateListeners --> Invalid: remote dashboard without auth/TLS
    ValidateListeners --> ValidateTLS
    ValidateTLS --> Invalid: missing or invalid cert/key pair
    ValidateTLS --> Ready
    Invalid --> [*]
    Ready --> Runtime
```

## Server Runtime

`internal/server.Server` owns process-level listener setup and shutdown. It builds one shared handler stack and attaches it to the active transport planes.

```mermaid
flowchart TB
    ServerNew["server.New(cfg, dataDir, store)"] --> Cert["ensureCertificate"]
    ServerNew --> Logger["observability.NewLogger"]
    ServerNew --> Handler["buildHandler"]

    Handler --> AppMux["appmux.NewWithOptions"]
    Handler --> RateLimit["rate limiter"]
    Handler --> Security["security headers"]
    Handler --> AccessLog["access log middleware"]
    Handler --> RequestID["request ID middleware"]

    ServerNew --> HTTPS["http.Server<br/>ListenAndServeTLS"]
    ServerNew --> RealH3["http3.Server<br/>quic-go ListenAndServeTLS"]
    ServerNew --> UDP["transport.Listener<br/>experimental"]
    ServerNew --> Dash["dashboard.Server"]

    AppMux --> Endpoints["Test endpoints<br/>healthz readyz metrics ping echo download upload etc."]
```

### Listener Matrix

| Config Field | Package Path | Protocol | Default | Safety Gate |
|---|---|---|---|---|
| `server.listen_tcp` | `net/http` in `internal/server` | HTTPS over TCP, H1/H2 | `:8443` | TLS material is generated or configured |
| `server.listen_h3` | `quic-go/http3` in `internal/server` | HTTP/3 over QUIC | empty | Supported, explicit listener opt-in |
| `server.listen` | `internal/quic/transport` + `internal/h3` | Experimental Triton UDP H3 | empty | Requires experimental opt-in |
| `server.dashboard_listen` | `internal/dashboard` | HTTP loopback or HTTPS remote | `127.0.0.1:9090` | Remote requires auth and explicit TLS |

### Shared Endpoint Layer

`internal/appmux` exposes a protocol-neutral `http.Handler`. The same handler can run behind HTTPS/TCP, real HTTP/3, and experimental UDP H3.

```mermaid
flowchart LR
    Client["Client"] --> Plane{Transport Plane}
    Plane -->|HTTP/1.1 or H2| TCP["HTTPS/TCP"]
    Plane -->|H3| H3["quic-go HTTP/3"]
    Plane -->|Lab H3| LabH3["in-repo UDP H3 adapter"]
    TCP --> Handler["appmux handler"]
    H3 --> Handler
    LabH3 --> Handler
    Handler --> Routes["/ping /echo /download /upload /delay /streams /headers /drip /status /tls-info /quic-info /migration-test /.well-known/triton"]
    Handler --> Health["/healthz /readyz"]
    Handler --> Metrics["/metrics"]
```

The appmux layer also provides:

- Route-level method checks
- Request body size limits
- Synthetic endpoint behavior for latency, throughput, redirects, headers, and stream-style sampling
- Prometheus-style counters from `/metrics`
- Capability discovery through `/.well-known/triton`
- Health/readiness hooks provided by `internal/server`

## Probe Architecture

`internal/probe` produces a structured `probe.Result` with timing, TLS metadata, headers, analysis sections, trace files, support metadata, and fidelity metadata.

```mermaid
flowchart TD
    ProbeCmd["triton probe"] --> ProbeRun["probe.Run(target, cfg)"]
    ProbeRun --> Parse["Parse target URL"]
    Parse --> Scheme{Scheme}

    Scheme -->|https or empty| HTTPSProbe["Standard HTTPS probe<br/>net/http + httptrace"]
    Scheme -->|h3| RealH3Probe["Real HTTP/3 probe<br/>internal/realh3 + quic-go"]
    Scheme -->|triton://loopback| LoopbackProbe["In-process lab H3 loopback"]
    Scheme -->|triton://host| RemoteLabProbe["Remote experimental Triton UDP H3"]

    HTTPSProbe --> Analyze["Analysis enrichers"]
    RealH3Probe --> Analyze
    LoopbackProbe --> Analyze
    RemoteLabProbe --> Analyze

    Analyze --> Fidelity["support + support_summary + fidelity_summary"]
    Fidelity --> Result["probe.Result"]
    Result --> Store["storage.SaveProbe"]
    Result --> Summary["dashboard.BuildProbeSummary"]
```

### Probe Fidelity Model

Probe output intentionally distinguishes direct diagnostics from approximations.

```mermaid
flowchart LR
    Requested["Requested tests"] --> Plan["test_plan"]
    Plan --> Full["full<br/>direct current-path diagnostics"]
    Plan --> Observed["observed<br/>client/protocol-layer observation"]
    Plan --> Partial["partial<br/>heuristic or capability check"]
    Plan --> Unavailable["unavailable<br/>requested but not available"]

    Full --> Support["support"]
    Observed --> Support
    Partial --> Support
    Unavailable --> Support
    Support --> Rollup["support_summary"]
    Support --> Fidelity["fidelity_summary"]
```

| Fidelity | Tests | Meaning |
|---|---|---|
| `full` | `handshake`, `tls`, `latency`, `throughput`, `streams`, `alt-svc` | Directly implemented current-path diagnostics |
| `observed` | `version`, `retry`, `ecn` | Derived from visible client/protocol metadata, not packet capture |
| `partial` | `0rtt`, `migration`, `qpack`, `loss`, `congestion`, `spin-bit` | Heuristic, estimate-based, or endpoint-contract checks |

### Probe Result Shape

```mermaid
flowchart TB
    ProbeResult["probe.Result"]
    ProbeResult --> Identity["id, target, timestamp, duration"]
    ProbeResult --> Protocol["status, proto, headers, TLS metadata"]
    ProbeResult --> Timing["timings_ms<br/>dns connect tls first_byte total"]
    ProbeResult --> Trace["trace_files"]
    ProbeResult --> Analysis["analysis"]

    Analysis --> Response["response<br/>body bytes + throughput"]
    Analysis --> Latency["latency<br/>samples + p50/p95/p99"]
    Analysis --> Streams["streams<br/>concurrency success rate"]
    Analysis --> Advanced["advanced fields<br/>0rtt migration qpack loss congestion version retry ecn spin-bit"]
    Analysis --> FidelityFields["test_plan<br/>support<br/>support_summary<br/>fidelity_summary"]
```

## Benchmark Architecture

`internal/bench` runs worker loops for configured protocols and returns per-protocol `Stats` plus an aggregate `Summary`.

```mermaid
flowchart TD
    BenchCmd["triton bench"] --> BenchRun["bench.Run(target, cfg)"]
    BenchRun --> BeforeTrace["List qlog files before run"]
    BeforeTrace --> ProtocolLoop["For each configured protocol"]
    ProtocolLoop --> Warmup["Optional warmup"]
    Warmup --> RunProtocol["runProtocol"]

    RunProtocol --> H1["h1: net/http, HTTP/1.1 only"]
    RunProtocol --> H2["h2: net/http, HTTP/2 enabled"]
    RunProtocol --> H3["h3: quic-go HTTP/3 or triton:// lab path"]

    H1 --> Collector["benchmarkCollector"]
    H2 --> Collector
    H3 --> Collector
    Collector --> Stats["requests errors latency phases bytes"]
    Stats --> Summary["healthy/degraded/failed best/riskiest"]
    Summary --> Result["bench.Result"]
    Result --> Store["storage.SaveBench"]
    Result --> DashSummary["dashboard.BuildBenchSummary"]
```

### Benchmark Worker Model

```mermaid
sequenceDiagram
    participant B as bench.Run
    participant P as runProtocol
    participant W as Worker goroutines
    participant C as benchmarkCollector
    participant T as Target

    B->>P: protocol, duration, concurrency
    P->>W: start N workers until deadline
    loop until duration expires
        W->>T: GET request
        T-->>W: response or error
        W->>C: recordSuccess or recordError
    end
    P->>C: finalize(duration)
    C-->>B: Stats
```

| Stat | Source |
|---|---|
| `requests`, `errors`, `error_rate` | Atomic worker counters |
| `avg_ms` | Successful request total duration |
| `req_per_sec` | Successful requests divided by configured duration |
| `latency_ms.p50/p95/p99` | Bounded sample set |
| `phases_ms.connect/tls/first_byte/transfer` | `httptrace` where available |
| `error_summary` | Categorized request failures |
| `summary.best_protocol` | Highest requests per second |
| `summary.riskiest_protocol` | Highest error rate |

## Check Architecture

`triton check` is a profile-oriented orchestration mode. It does not introduce a new measurement engine; it reuses probe and bench.

```mermaid
flowchart LR
    Check["triton check"] --> Resolve["Resolve --profile, --probe-profile, --bench-profile"]
    Resolve --> ProbeProfile["Apply probe profile"]
    Resolve --> BenchProfile["Apply bench profile"]
    ProbeProfile --> ProbeRun["probe.Run"]
    BenchProfile --> BenchRun["bench.Run"]
    ProbeRun --> ProbeThresholds["evaluateProbeThresholds"]
    BenchRun --> BenchThresholds["evaluateBenchThresholds"]
    ProbeThresholds --> Verdict["Combined CheckResult"]
    BenchThresholds --> Verdict
    Verdict --> Outputs["table/json/yaml/markdown<br/>optional report, summary, JUnit"]
```

## Storage Architecture

`internal/storage.FileStore` is a local filesystem store for probe and bench results.

```mermaid
flowchart TB
    Result["Probe or bench result"] --> Save["FileStore.save"]
    Save --> Validate["Validate category and ID"]
    Validate --> Gzip["Write gzip JSON<br/>exclusive create"]
    Gzip --> Cleanup["Retention and max-results cleanup"]
    Result --> Summary["SaveProbeSummary / SaveBenchSummary"]
    Summary --> SummaryFile["summary JSON<br/>exclusive create"]
    Summary --> Index["summary index.json<br/>file lock + temp replace"]

    Dashboard["Dashboard list/detail API"] --> List["FileStore.List"]
    List --> Cache["List cache keyed by directory metadata"]
    Dashboard --> Load["Load gzip result or summary JSON"]
    Load --> Index
```

On disk, the default layout is:

```text
triton-data/
|-- probes/
|   `-- pr_*.json.gz
|-- benches/
|   `-- bn_*.json.gz
|-- probe_summaries/
|   |-- pr_*.json
|   `-- index.json
|-- bench_summaries/
|   |-- bn_*.json
|   `-- index.json
`-- certs/
    |-- server.crt
    `-- server.key
```

Storage design characteristics:

- Result IDs are validated before path construction.
- Result writes use exclusive creation and do not overwrite existing files.
- Full results are gzip JSON.
- Dashboard summaries are plain JSON for faster list rendering.
- Summary indexes are protected by an in-process write mutex plus lock files.
- Retention cleanup removes old or excess result files and matching summaries.

## Dashboard Architecture

The dashboard is an embedded operator UI plus a read-only JSON API.

```mermaid
flowchart TD
    Browser["Browser"] --> DashboardHTTP["dashboard.Server"]
    DashboardHTTP --> Static["Embedded assets<br/>index.html app.css app.js"]
    DashboardHTTP --> API["/api/v1/*"]

    API --> Auth["Optional Basic Auth"]
    API --> Headers["Security headers"]
    API --> Store["FileStore"]
    API --> Cache["Probe/bench/trace list caches"]

    Store --> Probes["Probe summaries/details"]
    Store --> Benches["Bench summaries/details"]
    API --> Traces["qlog .sqlog metadata/download"]
```

API surface:

| Route | Purpose |
|---|---|
| `GET /api/v1/status` | Dashboard uptime and storage counts |
| `GET /api/v1/config` | Sanitized dashboard-visible config snapshot |
| `GET /api/v1/probes` | Recent probe summaries with filter/sort/page support |
| `GET /api/v1/probes/:id` | Full stored probe result |
| `GET /api/v1/benches` | Recent bench summaries with filter/sort/page support |
| `GET /api/v1/benches/:id` | Full stored bench result |
| `GET /api/v1/traces` | qlog trace list |
| `GET /api/v1/traces/meta/:name` | qlog trace metadata and preview |
| `GET /api/v1/traces/:name` | qlog trace download |

Dashboard security posture:

- GET-only API routes
- Optional Basic Auth
- Remote dashboard mode requires auth and explicit TLS files
- Restrictive browser security headers
- API errors avoid exposing internal error detail
- Trace downloads validate `.sqlog` basenames and reject symlinks

## Observability Architecture

```mermaid
flowchart LR
    Request["HTTP request"] --> ReqID["WithRequestID"]
    ReqID --> Handler["Server or dashboard handler"]
    Handler --> Access["WithAccessLog"]
    Access --> Logger["slog JSON/text destination"]

    QUIC["quic-go connection"] --> QLOG["NewQLOGTracer"]
    QLOG --> TraceFile["*.sqlog trace file"]
    TraceFile --> Dashboard["Dashboard trace API"]
```

Observability components:

- Request IDs through `X-Request-Id`
- Access logs with component, method, path, status, bytes, duration, remote address, and user agent
- Startup logs that identify stable and experimental listener planes
- Prometheus-style appmux metrics from `/metrics`
- qlog trace generation for `quic-go` client/server paths when trace directories are configured
- Dashboard trace listing, preview, and download

## Real HTTP/3 Path

The supported HTTP/3 path is deliberately narrow and outsourced to `quic-go`.

```mermaid
flowchart LR
    subgraph ServerSide["Server Side"]
        H3Listen["server.listen_h3"] --> QGOServer["quic-go http3.Server"]
        QGOServer --> SharedHandler["appmux handler"]
    end

    subgraph ClientSide["Client Side"]
        H3Target["h3:// or h3 bench"] --> RealH3Client["internal/realh3.NewClient"]
        RealH3Client --> QGOTransport["quic-go http3.Transport"]
    end

    QGOTransport --> Network["QUIC over UDP"]
    Network --> QGOServer
```

`internal/realh3` is intentionally small:

- It builds a `quic-go/http3.Transport`.
- It enforces TLS 1.3 for HTTP/3 clients.
- It attaches qlog tracing when configured.
- It supports a TLS session cache for resumption checks.

## Experimental QUIC/H3 Path

The in-repo transport path is useful for learning, tests, and lab experiments. It is not the production HTTP/3 engine.

```mermaid
flowchart TD
    TritonURL["triton:// target or triton lab"] --> Dialer["internal/quic/transport.Dialer"]
    Dialer --> Initial["BuildInitialPacket<br/>client hello placeholder"]
    Initial --> Listener["transport.Listener"]
    Listener --> Conn["connection.Connection"]
    Conn --> Streams["stream.Manager"]
    Streams --> H3Frames["internal/h3/frame<br/>HEADERS + DATA"]
    H3Frames --> Adapter["h3 request adapter"]
    Adapter --> AppMux["appmux handler"]
    AppMux --> Response["H3 response frames"]
    Response --> ShortPacket["BuildShortPacket"]
    ShortPacket --> Client["experimental client session"]
```

Implemented building blocks include:

- QUIC varint helpers
- Packet number helpers
- Long and short packet parsing/building
- Selected QUIC frame parsing/serialization
- UDP transport wrapper with pooled buffers
- Listener/dialer session scaffold
- Connection state transitions and frame dispatch
- Stream manager and reassembly behavior
- Minimal H3 `HEADERS` and `DATA` frames
- Handler dispatch from H3 requests into `http.Handler`

Current limitations:

- No RFC-complete TLS 1.3 QUIC handshake
- No packet-level production telemetry
- No production congestion control or recovery model
- No complete QPACK implementation
- No production-grade migration, Retry, ECN, or spin-bit validation

## Security Model

```mermaid
flowchart TD
    Config["Configuration"] --> Validation["Config.Validate"]
    Validation --> ListenerSafety["Listener safety gates"]
    Validation --> TLSCheck["TLS material validation"]
    Validation --> InsecureGate["Insecure TLS opt-in gates"]

    Runtime["Runtime HTTP stack"] --> SecurityHeaders["Security headers"]
    Runtime --> RateLimit["Optional rate limit"]
    Runtime --> BodyLimit["Max body bytes"]
    Runtime --> Auth["Dashboard Basic Auth when configured"]

    Storage["Storage"] --> PathValidation["Category and ID validation"]
    Storage --> ExclusiveWrites["Exclusive result writes"]
    Storage --> Locking["Summary index locks"]
```

Key safety decisions:

- Experimental listeners are off by default.
- Lab-only remote exposure requires explicit opt-in.
- Remote dashboard mode requires auth and explicit certificate/key files.
- Runtime-generated certificates are not accepted for remote dashboard mode.
- Insecure client TLS must be acknowledged through `allow_insecure_tls`.
- Storage paths are constrained to known categories and validated IDs.
- Dashboard trace file access is limited to plain `.sqlog` basenames.

## Health, Readiness, and Metrics

```mermaid
flowchart LR
    Health["GET /healthz"] --> HealthCheck["runtimeHealthCheck"]
    Ready["GET /readyz"] --> ReadyCheck["runtimeReadyCheck"]
    Metrics["GET /metrics"] --> AppMetrics["appmux Metrics"]

    HealthCheck --> TLS["TLS material accessible and loadable"]
    HealthCheck --> StorageRead["Storage directory accessible"]
    HealthCheck --> TraceRead["Trace directory accessible"]

    ReadyCheck --> StorageWrite["Storage directory writable"]
    ReadyCheck --> TraceWrite["Trace directory writable"]

    AppMetrics --> Counters["request totals by route/status"]
    AppMetrics --> Uptime["uptime seconds"]
```

## Data Lifecycle

```mermaid
sequenceDiagram
    participant CLI as CLI command
    participant Engine as Probe/Bench engine
    participant Store as FileStore
    participant Summary as Dashboard summary builder
    participant Dash as Dashboard API
    participant UI as Browser

    CLI->>Engine: run target with merged config
    Engine-->>CLI: Result with timings, stats, analysis
    CLI->>Store: Save full gzip JSON
    CLI->>Summary: Build compact summary
    Summary-->>CLI: Summary JSON shape
    CLI->>Store: Save summary + update index
    UI->>Dash: GET /api/v1/probes or /benches
    Dash->>Store: List + load summaries
    Store-->>Dash: Summary list
    Dash-->>UI: JSON cards/tables
    UI->>Dash: GET detail by ID
    Dash->>Store: Load full gzip JSON
    Store-->>Dash: Full result
    Dash-->>UI: Detail JSON
```

## Dependency Direction

Triton's internal packages mostly follow a simple dependency shape: CLI orchestrates, engines measure, storage persists, dashboard reads, and shared transport/application primitives sit below them.

```mermaid
flowchart BT
    QUIC["internal/quic"] --> H3["internal/h3"]
    AppMux["internal/appmux"] --> Server["internal/server"]
    AppMux --> H3
    RealH3["internal/realh3"] --> Server
    RealH3 --> Probe["internal/probe"]
    RealH3 --> Bench["internal/bench"]
    Storage["internal/storage"] --> CLI["internal/cli"]
    Storage --> Dashboard["internal/dashboard"]
    Probe --> CLI
    Bench --> CLI
    Dashboard --> Server
    Config["internal/config"] --> CLI
    Config --> Server
    Observability["internal/observability"] --> Server
    Observability --> Dashboard
    Observability --> RealH3
    RunID["internal/runid"] --> Probe
    RunID --> Bench
```

Read the arrows as "is used by" from lower-level packages to higher-level packages.

## Testing and Quality Gates

The repository includes tests across:

- CLI command behavior and output
- Config loading, validation, profiles, and strict parsing
- Server endpoints, rate limiting, and runtime safety
- Probe analytics and remote paths
- Benchmark summaries and statistics
- Storage persistence, duplicate protection, and concurrent summary writes
- Dashboard API list/detail behavior
- QUIC packet, frame, wire, connection, stream, and transport helpers
- H3 frame parsing and loopback behavior

```mermaid
flowchart LR
    Unit["go test ./..."] --> Packages["internal packages"]
    Race["go test -race ./... in CI"] --> Packages
    Vet["go vet ./..."] --> Static["static correctness"]
    Staticcheck["staticcheck ./..."] --> Static
    Gosec["gosec ./..."] --> Security["security scan"]
    Smoke["scripts/ci-smoke"] --> CLI["binary smoke paths"]
    BenchGuard["scripts/ci-bench-guard"] --> Perf["benchmark guardrails"]
```

## Operational Profiles

### Local Development

```mermaid
flowchart LR
    Dev["Developer"] --> Server["triton server"]
    Server --> HTTPS["https://localhost:8443"]
    Server --> Dashboard["http://127.0.0.1:9090"]
    Probe["triton probe --target https://localhost:8443/ping --insecure --allow-insecure-tls"] --> HTTPS
```

Typical properties:

- Runtime-generated certificate material is acceptable for disposable local use.
- Dashboard stays on loopback.
- Storage defaults to `./triton-data`.
- Experimental transport remains disabled unless explicitly tested.

### Supported Production-Like Runtime

```mermaid
flowchart LR
    Operator["Operator"] --> Config["Explicit config"]
    Config --> Certs["server.cert + server.key"]
    Config --> HTTPS["server.listen_tcp"]
    Config --> H3["optional server.listen_h3"]
    Config --> Dash["dashboard loopback or authenticated HTTPS remote"]
    Config --> Store["persistent storage.results_dir"]
    HTTPS --> Health["/healthz /readyz /metrics"]
    H3 --> Health
    Dash --> API["/api/v1/status"]
```

Recommended posture:

- Use supported listeners only.
- Provide explicit TLS files for shared or remote environments.
- Keep dashboard loopback unless remote access is intentional.
- Mount persistent storage.
- Set retention and max-result limits.
- Monitor health, readiness, metrics, and dashboard status.
- Keep experimental flags unset.

### Lab Runtime

```mermaid
flowchart LR
    Research["Researcher"] --> Lab["triton lab"]
    Lab --> UDP["127.0.0.1:4433 by default"]
    UDP --> InRepo["internal/quic + internal/h3"]
    Probe["triton probe --target triton://loopback/ping"] --> UDP
```

Lab posture:

- Use for protocol experiments and educational inspection.
- Prefer loopback.
- Do not describe results as production HTTP/3 truth.
- Separate lab observations from supported-path operational claims.

## Known Limits

- Advanced probe fields are not all packet-level telemetry.
- The dashboard is a lightweight operator surface, not a full live protocol workbench.
- Persistence is filesystem-backed and single-node oriented.
- High-scale multi-tenant operation is not claimed.
- The in-repo QUIC/H3 implementation is research code.
- The supported HTTP/3 path depends on `quic-go` rather than the custom engine.

## Architecture Decision Summary

| Decision | Current Choice | Why |
|---|---|---|
| Product shape | Single binary | Simple local/CI/operator workflow |
| Supported H3 engine | `quic-go` | Real, maintained HTTP/3 behavior today |
| Custom QUIC/H3 | Lab-only in-repo scaffold | Useful for research without overstating support |
| Persistence | Filesystem gzip JSON plus summary indexes | Simple, inspectable, portable single-node storage |
| Dashboard | Embedded static assets plus read-only JSON API | No external frontend deployment required |
| Config | Defaults + YAML + env + flags | Works for local, container, and CI usage |
| Safety | Explicit opt-ins for risky modes | Avoid accidental remote/lab exposure |
| Fidelity | `full` / `observed` / `partial` metadata | Honest interpretation of probe output |
