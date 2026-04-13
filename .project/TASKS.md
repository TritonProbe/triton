# TASKS.md — Triton

## HTTP/3 (QUIC) Test Server & Benchmarking Platform

**Total Tasks:** 142
**Estimated Duration:** 14-18 weeks
**Phases:** 10

> Current-state note (2026-04-12): this task list is still the original target-state backlog. A substantial subset is already complete in the current repository, including the working CLI, config system, storage, HTTPS test server, real HTTP/3 support via `quic-go`, the isolated `triton lab` flow for the experimental transport, richer probe/bench summaries, dashboard status/config/probe/bench/trace APIs, dashboard hardening, CI automation, `gosec` integration, and initial fuzz coverage for parser surfaces. Treat unchecked items below as roadmap targets, not proof that the repository is empty or non-functional.

## Current Status Snapshot

### Completed Foundations

- Working commands: `server`, `lab`, `probe`, `bench`, `version`
- Config precedence: defaults -> YAML -> env -> CLI
- Filesystem result persistence with gzip JSON storage
- HTTPS test server with health, readiness, metrics, and benchmark endpoints
- Real HTTP/3 client/server support via `quic-go`
- Experimental in-repo UDP H3 kept behind explicit opt-in and `triton lab`
- Probe support summaries and partial advanced analysis coverage
- Bench percentile/phase/error summaries and per-run health rollups
- Embedded dashboard with status/config/probes/benches/traces APIs and overview cards
- CI running format, tests, vet, staticcheck, build, smoke, and `gosec`
- Parser fuzz targets for QUIC packet/frame and H3 frame surfaces

### Still Strategic / Incomplete

- Custom QUIC-TLS handshake and packet protection
- Real QPACK implementation
- Packet-level 0-RTT, migration, congestion, loss, retry, ECN, and spin-bit telemetry
- Full live dashboard workbench with SSE/charts/inspector
- Race-tested confidence for the entire transport stack in CI

---

## PHASE 1 — PROJECT SCAFFOLD & UDP TRANSPORT (Week 1)

> Foundation: Go module, project structure, UDP socket layer, build system.

| # | Task | Description | Depends |
|---|---|---|---|
| 1 | Go module init | `go mod init github.com/tritonprobe/triton`, create directory skeleton per SPECIFICATION §15 | — |
| 2 | Config struct + defaults | `internal/config/config.go` — Config struct with all fields, default values, Validate() method | 1 |
| 3 | YAML config loader | `internal/config/loader.go` — Load from triton.yaml via gopkg.in/yaml.v3, env var override (TRITON_*), merge priority | 2 |
| 4 | CLI command router | `internal/cli/root.go` — Zero-dep command parser: Command struct, Flag parsing (--flag=val, -f val), help generation, subcommand routing | 1 |
| 5 | CLI output formatters | `internal/cli/output.go` — TableFormatter, JSONFormatter, CSVFormatter, YAMLFormatter, MarkdownFormatter, terminal color detection | 4 |
| 6 | Server command skeleton | `internal/cli/server.go` — server subcommand with all flags from SPEC §10.2, wiring to config | 4 |
| 7 | Probe command skeleton | `internal/cli/probe.go` — probe subcommand with all flags from SPEC §10.3 | 4 |
| 8 | Bench command skeleton | `internal/cli/bench.go` — bench subcommand with all flags from SPEC §10.4 | 4 |
| 9 | UDP transport | `internal/quic/transport/udp.go` — UDPTransport struct: ReadPacket, WritePacket, WriteBatch, SetReadDeadline, Close. Buffer pool with sync.Pool | 1 |
| 10 | Platform-specific UDP opts | Linux: SO_REUSEPORT/UDP_GRO/UDP_GSO via x/sys. macOS: SO_REUSEPORT. Windows: standard. Build tags | 9 |
| 11 | Varint encoding | Variable-length integer encode/decode per RFC 9000 §16: ReadVarInt, WriteVarInt, VarIntLen | 1 |
| 12 | Entry point + version | `cmd/triton/main.go` — main(), version flag, command dispatch | 4,6,7,8 |
| 13 | Makefile | Build targets: build, build-all (cross-compile), test, test-fuzz, lint, clean, dev, docker | 12 |
| 14 | Dockerfile | Multi-stage: Go builder → scratch, EXPOSE 4433/udp 8443/tcp 9090/tcp | 13 |

**Phase 1 Deliverable:** `triton version` works, `triton server --help` shows flags, UDP socket reads/writes packets.

---

## PHASE 2 — QUIC PACKET LAYER (Weeks 2-3)

> Packet parsing, serialization, header protection, packet number handling.

| # | Task | Description | Depends |
|---|---|---|---|
| 15 | Long header parser | `internal/quic/packet/header.go` — Parse long header: form bit, type, version, DCID/SCID lengths+values, type-specific fields (token for Initial, length) | 11 |
| 16 | Short header parser | Parse short header: form bit, spin bit, key phase, packet number (after unprotection) | 11 |
| 17 | Initial packet | `internal/quic/packet/initial.go` — Parse/serialize Initial packets: token extraction, CRYPTO frame payload, padding | 15 |
| 18 | Handshake packet | `internal/quic/packet/handshake.go` — Parse/serialize Handshake packets | 15 |
| 19 | 1-RTT packet | `internal/quic/packet/onertt.go` — Parse/serialize short header 1-RTT packets | 16 |
| 20 | 0-RTT packet | `internal/quic/packet/zerortt.go` — Parse/serialize 0-RTT long header packets | 15 |
| 21 | Retry packet | `internal/quic/packet/retry.go` — Parse/serialize Retry packets with Retry Integrity Tag (AES-128-GCM) | 15 |
| 22 | Version Negotiation | `internal/quic/packet/version_neg.go` — Parse/serialize Version Negotiation packets (no encryption) | 15 |
| 23 | Packet number encoding | `internal/quic/packet/number.go` — EncodePacketNumber, DecodePacketNumber per RFC 9000 §A.3 | 11 |
| 24 | AES header protection | `internal/quic/packet/protection.go` — AESHeaderProtector: Protect/Unprotect using AES-128-ECB mask from sample | 23 |
| 25 | ChaCha20 header protection | ChaChaHeaderProtector: Protect/Unprotect using ChaCha20 mask from sample | 23 |
| 26 | Packet round-trip tests | Unit tests: parse → serialize → parse for every packet type, known test vectors from RFC | 17-25 |
| 27 | Packet parser fuzz tests | `go test -fuzz=FuzzPacketParser` — fuzz Initial, Handshake, 1-RTT, Retry parsing | 26 |

**Phase 2 Deliverable:** All QUIC packet types can be parsed and serialized. Header protection works with known test vectors.

---

## PHASE 3 — QUIC FRAMES & CRYPTO (Weeks 3-4)

> Frame parsing, TLS 1.3 integration, initial key derivation.

| # | Task | Description | Depends |
|---|---|---|---|
| 28 | Frame type definitions | `internal/quic/frame/types.go` — Frame interface, FrameType constants, ParseFrames() dispatcher | 11 |
| 29 | ACK frame | `internal/quic/frame/ack.go` — ACKFrame parse/serialize, ACKRange iteration, ForEachRange, Contains, ECN counts | 28 |
| 30 | STREAM frame | `internal/quic/frame/stream.go` — StreamFrame parse/serialize with OFF/LEN/FIN bit handling | 28 |
| 31 | CRYPTO frame | `internal/quic/frame/crypto.go` — CryptoFrame parse/serialize (offset + data for TLS handshake) | 28 |
| 32 | Flow control frames | `internal/quic/frame/flow.go` — MAX_DATA, MAX_STREAM_DATA, MAX_STREAMS_BIDI/UNI, DATA_BLOCKED, STREAM_DATA_BLOCKED, STREAMS_BLOCKED | 28 |
| 33 | Connection frames | `internal/quic/frame/connection.go` — CONNECTION_CLOSE (transport + app), NEW_CONNECTION_ID, RETIRE_CONNECTION_ID, HANDSHAKE_DONE | 28 |
| 34 | Path frames | `internal/quic/frame/path.go` — PATH_CHALLENGE, PATH_RESPONSE (8-byte data) | 28 |
| 35 | Misc frames | `internal/quic/frame/misc.go` — PING, PADDING, NEW_TOKEN, STOP_SENDING, RESET_STREAM | 28 |
| 36 | Frame fuzz tests | Fuzz ParseFrames with random payloads | 29-35 |
| 37 | Initial key derivation | `internal/quic/tls/keyschedule.go` — Derive initial keys from DCID using HKDF-Extract + HKDF-Expand-Label (x/crypto/hkdf) with QUIC v1 salt | 11 |
| 38 | QUIC-TLS handshake wrapper | `internal/quic/tls/handshake.go` — QUICTLSConn wrapping crypto/tls.Conn, CRYPTO stream adapter, key callback hooks for each encryption level | 37 |
| 39 | Packet protection (encrypt/decrypt) | AES-128-GCM and ChaCha20-Poly1305 AEAD using crypto/aes + crypto/cipher, nonce = iv XOR pn | 37 |
| 40 | 0-RTT key handling | `internal/quic/tls/zerortt.go` — Session ticket storage, early data key derivation, 0-RTT acceptance/rejection | 38 |
| 41 | Session ticket store | `internal/quic/tls/session.go` — In-memory LRU cache (configurable size), optional file persistence, TTL management | 40 |
| 42 | Transport parameters | `internal/quic/connection/` — TransportParameters struct, Serialize/Deserialize as TLS extension (0x39) | 38 |

**Phase 3 Deliverable:** All QUIC frames parsed/serialized. TLS 1.3 handshake produces encryption keys at all levels.

---

## PHASE 4 — QUIC CONNECTION & STREAMS (Weeks 5-6)

> Connection state machine, stream multiplexing, flow control.

| # | Task | Description | Depends |
|---|---|---|---|
| 43 | Connection ID manager | `internal/quic/connection/id_manager.go` — Generate/store/retire connection IDs, sequence number tracking, stateless reset token | 11 |
| 44 | Connection state machine | `internal/quic/connection/conn.go` — Connection struct with full state machine: Idle→InitialSent→Handshake→Connected→Draining→Closed. Main Run() loop | 42,43 |
| 45 | Packet dispatch | Connection.handlePacket(): decrypt, parse frames, dispatch to appropriate handler per frame type | 39,44 |
| 46 | QUIC listener (server) | `internal/quic/transport/listener.go` — Accept incoming connections, demux by DCID, create Connection for new clients, handle Initial packets | 9,44 |
| 47 | QUIC dialer (client) | `internal/quic/transport/dialer.go` — Dial server: generate DCID/SCID, send Initial, complete handshake, return Connection | 9,44 |
| 48 | Stream struct | `internal/quic/stream/stream.go` — Stream with send/recv buffers, io.ReadWriteCloser, CloseWrite (FIN), Reset (RESET_STREAM) | 30 |
| 49 | Receive buffer (reassembly) | `internal/quic/stream/stream.go` — Gap-based reassembly buffer: Insert(offset, data, fin), Read(), Readable() for out-of-order data | 48 |
| 50 | Stream manager | `internal/quic/stream/manager.go` — OpenStream, AcceptStream, GetStream, CloseStream. Stream ID allocation (client/server, bidi/uni parity) | 48 |
| 51 | Per-stream flow control | `internal/quic/stream/flow_control.go` — MAX_STREAM_DATA tracking, STREAM_DATA_BLOCKED detection, auto window update | 32,48 |
| 52 | Connection-level flow control | Connection: MAX_DATA tracking, DATA_BLOCKED, aggregate stream data accounting | 32,44 |
| 53 | Stream count limits | MAX_STREAMS enforcement, STREAMS_BLOCKED signaling | 32,50 |
| 54 | Stream prioritization | `internal/quic/stream/priority.go` — Urgency (0-7) + incremental flag per RFC 9218 | 50 |
| 55 | Connection close/drain | `internal/quic/connection/close.go` — Send CONNECTION_CLOSE, enter draining state, drain timer (3× PTO), immediate close | 44 |
| 56 | Token management | `internal/quic/token/` — Retry token generation/validation (HMAC), NEW_TOKEN generation for future connections | 35,44 |
| 57 | Version negotiation | `internal/quic/version.go` — Version negotiation handling, supported versions list (0x00000001 for QUIC v1) | 22,44 |
| 58 | Integration test: handshake | Full client↔server handshake on loopback, verify keys, exchange data on stream | 46,47,50 |

**Phase 4 Deliverable:** Full QUIC connection lifecycle works: connect, open streams, transfer data, close. Loopback integration test passes.

---

## PHASE 5 — LOSS DETECTION & CONGESTION CONTROL (Week 7)

> Recovery, RTT estimation, congestion algorithms.

| # | Task | Description | Depends |
|---|---|---|---|
| 59 | Sent packet tracker | `internal/quic/recovery/sent_tracker.go` — Per-space sent packet map, ack-eliciting count, in-flight bytes | 29 |
| 60 | RTT estimator | `internal/quic/recovery/rtt.go` — smoothed_rtt, rttvar, min_rtt, latest_rtt calculation per RFC 9002 | 59 |
| 61 | Loss detector | `internal/quic/recovery/detector.go` — Packet/time threshold detection, OnAckReceived, DetectLostPackets | 59,60 |
| 62 | PTO timer | `internal/quic/recovery/pto.go` — Probe Timeout calculation with exponential backoff, PTO probe sending | 60,61 |
| 63 | Persistent congestion detection | Detect persistent congestion (consecutive PTO spans), reset CWND to minimum | 61,62 |
| 64 | New Reno congestion control | `internal/quic/congestion/newreno.go` — CongestionController implementation: slow start, congestion avoidance, recovery phase | 61 |
| 65 | Cubic congestion control | `internal/quic/congestion/cubic.go` — W(t)=C*(t-K)³+W_max, K=∛(W_max*β/C), β=0.7, C=0.4 | 61 |
| 66 | BBR v2 congestion control | `internal/quic/congestion/bbr.go` — Startup→Drain→ProbeBW→ProbeRTT state machine, pacing rate, BDP estimation | 61 |
| 67 | ECN handling | ECN codepoint setting on sent packets, ECN count tracking from ACK_ECN frames, CE detection → congestion event | 29,64 |
| 68 | Congestion controller factory | `internal/quic/congestion/controller.go` — Interface + NewCongestionController(algorithm string) factory | 64,65,66 |
| 69 | Recovery integration | Wire loss detector + congestion controller into Connection.handlePacket() and send path | 44,61,68 |
| 70 | Loss recovery tests | Unit tests with known packet sequences, verify correct loss detection and CWND transitions | 69 |

**Phase 5 Deliverable:** Loss detection, RTT estimation, and all 3 congestion control algorithms working. Data transfer resilient to packet loss.

---

## PHASE 6 — CONNECTION MIGRATION & 0-RTT (Week 8)

> Migration path validation, 0-RTT end-to-end, MTU discovery.

| # | Task | Description | Depends |
|---|---|---|---|
| 71 | Path validator | `internal/quic/connection/migration.go` — PathValidator: PATH_CHALLENGE/PATH_RESPONSE exchange, validation timeout, anti-amplification (3× limit) | 34,44 |
| 72 | Migration handler | OnPeerAddressChange: detect address change, switch path, reset congestion state, initiate validation | 71,68 |
| 73 | NAT rebinding | Handle NAT rebinding (same IP, new port) without full migration — skip path validation for minor changes | 72 |
| 74 | Active migration (client) | Client-side: rebind to new UDP socket, update Connection.localAddr, send on new path, validate | 72 |
| 75 | Connection ID rotation | Rotate to new connection ID on migration, retire old CIDs via RETIRE_CONNECTION_ID | 43,72 |
| 76 | 0-RTT end-to-end (server) | Server: accept early data, replay protection (client_hello hash dedup window 10s), signal acceptance/rejection | 40,41,44 |
| 77 | 0-RTT end-to-end (client) | Client: send early data with saved session ticket, handle acceptance/rejection, fall back to 1-RTT | 40,41,47 |
| 78 | MTU discovery | `internal/quic/transport/mtu.go` — DPLPMTUD: probe with PING+PADDING, binary search between 1200 and path MTU, timer-based probing | 9,35 |
| 79 | Migration integration test | Client connects → transfers data → rebinds socket → validates path → continues transfer → verify no data loss | 74,75 |
| 80 | 0-RTT integration test | Client connects (full handshake) → gets ticket → reconnects with 0-RTT → verify early data delivered | 76,77 |

**Phase 6 Deliverable:** Connection migration (NAT rebinding + active) works. 0-RTT resumption end-to-end. MTU discovery probing.

---

## PHASE 7 — HTTP/3 LAYER (Weeks 9-10)

> QPACK, HTTP/3 frames, H3 server/client, test endpoints.

| # | Task | Description | Depends |
|---|---|---|---|
| 81 | QPACK static table | `internal/h3/qpack/static_table.go` — 99-entry static table from RFC 9204 Appendix A | — |
| 82 | Huffman codec | `internal/h3/qpack/huffman.go` — Huffman encode/decode per RFC 7541 Appendix B table | — |
| 83 | QPACK dynamic table | `internal/h3/qpack/dynamic_table.go` — Insert, Lookup, Evict, capacity management, insert count tracking | 81 |
| 84 | QPACK encoder | `internal/h3/qpack/encoder.go` — EncodeHeaders: static ref, dynamic ref, literal with/without name ref, Huffman, encoder instructions | 81,82,83 |
| 85 | QPACK decoder | `internal/h3/qpack/decoder.go` — DecodeHeaders: Required Insert Count, section prefix, all instruction types | 81,82,83 |
| 86 | QPACK fuzz tests | Fuzz decoder with random inputs, round-trip encode→decode verification | 84,85 |
| 87 | HTTP/3 frame types | `internal/h3/frame/types.go` — Frame type definitions, generic Parse/Serialize | 11 |
| 88 | DATA frame | `internal/h3/frame/data.go` — Type 0x00, payload pass-through | 87 |
| 89 | HEADERS frame | `internal/h3/frame/headers.go` — Type 0x01, QPACK-encoded header block | 87 |
| 90 | SETTINGS frame | `internal/h3/frame/settings.go` — Type 0x04, key-value transport settings | 87 |
| 91 | GOAWAY + misc H3 frames | `internal/h3/frame/goaway.go` — GOAWAY (0x07), CANCEL_PUSH (0x03), PUSH_PROMISE (0x05) | 87 |
| 92 | H3 server | `internal/h3/server.go` — Accept QUIC conn, open control+QPACK streams, accept request streams, QPACK decode headers, call http.Handler, QPACK encode response, send DATA frames | 50,84,85,88-91 |
| 93 | H3 client | `internal/h3/client.go` — RoundTrip: open stream, encode request, write HEADERS+DATA, read response HEADERS+DATA, return http.Response | 47,84,85,88-91 |
| 94 | H3 transport binding | `internal/h3/transport.go` — Bind H3 to QUIC connection, control stream management, SETTINGS exchange | 92,93 |
| 95 | Test endpoint handlers | `internal/server/endpoints.go` — All 16 test endpoints from SPEC §4.1: /ping, /echo, /download/:size, /upload, /delay/:ms, /streams/:n, /headers/:n, /redirect/:n, /status/:code, /drip/:size/:delay, /tls-info, /quic-info, /migration-test, /.well-known/triton | 92 |
| 96 | HTTP/1.1+HTTP/2 fallback server | `internal/server/fallback.go` — Standard http.Server with TLS, same handlers, ALPN h2/http/1.1 | 95 |
| 97 | ACME client | `internal/server/acme.go` — HTTP-01 + TLS-ALPN-01 challenge, account management, cert renewal | 92 |
| 98 | Self-signed cert generator | Auto-generate self-signed TLS cert if no cert/key provided, store in triton-data/certs/ | 92 |
| 99 | Server mode integration | Wire everything: CLI server command → config → QUIC listener → H3 server → test endpoints + fallback + dashboard. `triton server` fully operational | 6,92,95,96,98 |
| 100 | H3 integration test | Client sends requests to all test endpoints, verifies correct responses over HTTP/3 | 93,95 |

**Phase 7 Deliverable:** Full HTTP/3 server with all test endpoints. HTTP/1.1+H2 fallback. ACME support. `triton server` works end-to-end.

---

## PHASE 8 — PROBE & BENCH MODES (Weeks 11-12)

> Protocol analysis, benchmarking, network simulation.

| # | Task | Description | Depends |
|---|---|---|---|
| 101 | Connection tracer | `internal/analytics/collector.go` — ConnectionTracer with all hook methods: OnPacketSent/Received/Lost, OnRTTSample, OnCWNDUpdate, OnStreamOpened/Closed, OnHandshake, OnZeroRTT, OnMigration | 44 |
| 102 | Timeline recorder | `internal/analytics/timeline.go` — Nanosecond-precision event recording for all protocol events | 101 |
| 103 | qlog writer | `internal/analytics/qlog.go` — Output draft-ietf-quic-qlog format JSON | 102 |
| 104 | Percentile calculator | `internal/analytics/percentile.go` — p50, p95, p99, mean, stddev, min, max from sample arrays | — |
| 105 | Probe orchestrator | `internal/probe/probe.go` — Probe struct, Run(): DNS timing, iterate selected tests, collect results, format output | 93,101 |
| 106 | Handshake probe test | `internal/probe/tests.go` — Full handshake timing breakdown: DNS, UDP connect, Initial sent, Initial received, Handshake complete, first byte | 105 |
| 107 | TLS probe test | TLS cipher suite, ALPN, certificate chain analysis, key exchange details | 105 |
| 108 | 0-RTT probe test | Two-phase test: full handshake → ticket save → 0-RTT resume, measure time saved | 77,105 |
| 109 | Migration probe test | Connect → send data → rebind socket → PATH_CHALLENGE → verify continued transfer | 74,105 |
| 110 | Throughput probe test | Download/upload speed measurement with configurable sizes | 105 |
| 111 | Latency probe test | RTT measurement with percentiles (p50, p95, p99) over N iterations | 104,105 |
| 112 | Stream concurrency probe | Open N concurrent streams, measure multiplexing behavior vs HoL blocking | 105 |
| 113 | QPACK analysis probe | Dynamic table usage, header compression ratio, blocked stream count | 84,85,105 |
| 114 | Congestion profiling probe | CWND growth over time, loss recovery behavior, algorithm detection | 101,105 |
| 115 | Alt-Svc detection probe | HTTP/1.1 or H2 request to check Alt-Svc header for H3 advertisement | 105 |
| 116 | Spin bit validation probe | Verify spin bit RTT estimation matches actual RTT | 105 |
| 117 | GREASE probe | Test server handling of unknown frame types and transport params | 105 |
| 118 | Probe result formatting | Format ProbeResult as table/json/csv/yaml/markdown per --format flag | 5,105 |
| 119 | Bench orchestrator | `internal/bench/bench.go` — BenchRunner: warmup, spawn workers per protocol, collect results, calculate percentiles | 104 |
| 120 | H1 benchmark runner | `internal/bench/h1_runner.go` — HTTP/1.1 benchmark using net/http | 119 |
| 121 | H2 benchmark runner | `internal/bench/h2_runner.go` — HTTP/2 benchmark using net/http with ForceAttemptHTTP2 | 119 |
| 122 | H3 benchmark runner | `internal/bench/h3_runner.go` — HTTP/3 benchmark using Triton's H3 client | 93,119 |
| 123 | Network condition simulator | `internal/bench/simulator.go` — WrapConn: latency injection, packet loss, bandwidth throttling, jitter, reordering via token bucket | 119 |
| 124 | Comparison calculator | `internal/analytics/comparison.go` — Cross-protocol comparison: winner per metric, percentage differences, summary | 104,119 |
| 125 | Bench report generator | HTML/JSON/CSV/Markdown report output with charts (HTML uses inline SVG) | 119,124 |
| 126 | Probe CLI integration | Wire probe command → Probe orchestrator → output | 7,105,118 |
| 127 | Bench CLI integration | Wire bench command → BenchRunner → output | 8,119,125 |

**Phase 8 Deliverable:** `triton probe` and `triton bench` fully operational with all test types and output formats.

---

## PHASE 9 — WEB DASHBOARD (Weeks 13-14)

> Embedded web UI, real-time updates, interactive charts.

| # | Task | Description | Depends |
|---|---|---|---|
| 128 | Dashboard HTTP server | `internal/dashboard/server.go` — http.Server + ServeMux, embed.FS for static assets, route registration | 99 |
| 129 | REST API handlers | `internal/dashboard/api.go` — All endpoints from SPEC §9.1: status, config, probe CRUD, bench CRUD, connections, compare, export | 128 |
| 130 | SSE hub | `internal/dashboard/sse.go` — Client registration, broadcast, event types (probe_progress, bench_update, connection_event), auto-reconnect support | 128 |
| 131 | WebSocket handler | `internal/dashboard/websocket.go` — Bidirectional control channel for probe/bench start/stop commands (custom WS implementation, no gorilla) | 128 |
| 132 | Storage backend | `internal/storage/filesystem.go` — FileStore: Save/Load/List/Cleanup with gzip JSON, retention policy | 2 |
| 133 | HTML/CSS structure | `web/` — index.html (SPA shell), triton.css (CSS custom properties, dark/light theme, responsive layout, grid-based dashboard) | — |
| 134 | Canvas chart library | `web/js/charts.js` — TritonChart: LineChart, BarChart, Histogram, RadarChart, WaterfallChart, GaugeChart. Dark/light theme, responsive, tooltips, animation | 133 |
| 135 | Connection timeline view | `web/js/timeline.js` — Waterfall visualization: DNS→UDP→Initial→Handshake→1-RTT→Streams, color-coded phases, zoom | 134 |
| 136 | Packet inspector view | `web/js/inspector.js` — Table of packets with expandable frame details, filter by type/stream, loss highlighting | 134 |
| 137 | Dashboard app JS | `web/js/app.js` — SPA router, SSE connection, page initialization, probe/bench launch forms, settings management | 134,135,136 |
| 138 | Comparison view | `web/js/comparison.js` — Side-by-side H1/H2/H3 bar charts, radar chart, winner indicators | 134 |
| 139 | Embed assets | `embed.FS` directives in dashboard/server.go, compress-on-build (gzip pre-compressed for web serving) | 133-138 |
| 140 | Dashboard integration | Wire dashboard server into main server command, `/dashboard/*` routes, live SSE from probe/bench operations | 128-139 |

**Phase 9 Deliverable:** Full web dashboard with real-time charts, probe console, benchmark view, connection inspector.

---

## PHASE 10 — POLISH, TESTING & RELEASE (Weeks 15-16)

> Interop testing, documentation, release engineering.

| # | Task | Description | Depends |
|---|---|---|---|
| 141 | Interop test suite | Test against Google (google.com), Cloudflare (cloudflare-quic.com), Facebook (facebook.com) HTTP/3 servers. Verify handshake, 0-RTT, streams | 126 |
| 142 | README.md | Comprehensive README: badges, quick start, features, screenshots, installation, usage examples, configuration, API docs links, contributing guide | ALL |

**Phase 10 Deliverable:** Production-ready v1.0.0 release. Cross-platform binaries, Docker image, comprehensive docs.

---

## DEPENDENCY GRAPH SUMMARY

```
Phase 1 (Scaffold)
  └→ Phase 2 (Packets)
      └→ Phase 3 (Frames + Crypto)
          └→ Phase 4 (Connection + Streams)
              ├→ Phase 5 (Recovery + Congestion)
              │   └→ Phase 6 (Migration + 0-RTT)
              │       └→ Phase 8 (Probe + Bench)
              │           └→ Phase 10 (Polish)
              └→ Phase 7 (HTTP/3)
                  ├→ Phase 8 (Probe + Bench)
                  └→ Phase 9 (Dashboard)
                      └→ Phase 10 (Polish)
```

---

*Three Prongs. One Binary. Every Packet.*
