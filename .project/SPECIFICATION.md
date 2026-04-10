# SPECIFICATION.md — Triton

## HTTP/3 (QUIC) Test Server & Benchmarking Platform

**Version:** 1.0.0
**Status:** SPECIFICATION COMPLETE
**Repository:** github.com/tritonprobe/triton
**Website:** tritonprobe.com
**License:** MIT
**Tagline:** "Three Prongs. One Binary. Every Packet."

---

## 1. EXECUTIVE SUMMARY

Triton is a pure Go, zero-dependency, single-binary HTTP/3 (QUIC) test server and benchmarking platform. It provides comprehensive QUIC protocol testing, 0-RTT measurement, connection migration analysis, congestion control profiling, and an embedded web dashboard for real-time visualization. Triton operates in three modes: **Server** (HTTP/3 test endpoint), **Probe** (test external servers), and **Bench** (comparative benchmarking across HTTP/1.1, HTTP/2, HTTP/3).

Named after the Greek sea god who carries a trident — three prongs representing HTTP/3's three revolutionary pillars: QUIC transport, 0-RTT resumption, and stream multiplexing without head-of-line blocking.

---

## 2. PHILOSOPHY & CONSTRAINTS

### 2.1 #NOFORKANYMORE

- **Zero external dependencies** — only `golang.org/x/crypto`, `golang.org/x/sys`, `gopkg.in/yaml.v3` permitted
- **Single binary** — all assets (Web UI HTML/CSS/JS, TLS test certs, default config) embedded via `embed.FS`
- **Multi-platform** — Linux (amd64/arm64), macOS (amd64/arm64/universal), Windows (amd64/arm64)
- **Documentation-first** — SPECIFICATION → IMPLEMENTATION → TASKS → BRANDING → CODE

### 2.2 Core Principles

1. **Protocol Correctness** — Strict RFC 9000 (QUIC), RFC 9001 (QUIC-TLS), RFC 9114 (HTTP/3), RFC 9204 (QPACK) compliance
2. **Observable by Default** — Every packet, frame, stream event is traceable and measurable
3. **Comparison-Driven** — Always show HTTP/3 vs HTTP/2 vs HTTP/1.1 side-by-side
4. **Scriptable** — CLI-first with JSON output, Web UI is a bonus layer
5. **Educational** — Explain what's happening at every protocol layer

---

## 3. ARCHITECTURE OVERVIEW

```
┌─────────────────────────────────────────────────────────┐
│                     TRITON BINARY                       │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │  PRONG ONE  │  │  PRONG TWO   │  │  PRONG THREE  │  │
│  │   Server    │  │    Probe     │  │    Bench      │  │
│  │   Mode      │  │    Mode      │  │    Mode       │  │
│  └──────┬──────┘  └──────┬───────┘  └───────┬───────┘  │
│         │                │                   │          │
│  ┌──────┴────────────────┴───────────────────┴───────┐  │
│  │              QUIC ENGINE (Custom)                  │  │
│  │  ┌────────┐ ┌────────┐ ┌─────────┐ ┌──────────┐  │  │
│  │  │ Packet │ │ Stream │ │  Flow   │ │Congestion│  │  │
│  │  │ Parser │ │  Mux   │ │ Control │ │ Control  │  │  │
│  │  └────────┘ └────────┘ └─────────┘ └──────────┘  │  │
│  │  ┌────────┐ ┌────────┐ ┌─────────┐ ┌──────────┐  │  │
│  │  │  Loss  │ │  Conn  │ │  0-RTT  │ │  QPACK   │  │  │
│  │  │Detect  │ │Migration│ │ Resume │ │ Codec    │  │  │
│  │  └────────┘ └────────┘ └─────────┘ └──────────┘  │  │
│  └───────────────────────┬───────────────────────────┘  │
│                          │                              │
│  ┌───────────────────────┴───────────────────────────┐  │
│  │            TLS 1.3 ENGINE (crypto/tls)            │  │
│  └───────────────────────┬───────────────────────────┘  │
│                          │                              │
│  ┌───────────────────────┴───────────────────────────┐  │
│  │            UDP SOCKET LAYER (net.UDPConn)         │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │              HTTP/3 FRAME LAYER                   │  │
│  │  ┌─────────┐ ┌──────────┐ ┌────────────────────┐ │  │
│  │  │ DATA    │ │ HEADERS  │ │ SETTINGS/GOAWAY/   │ │  │
│  │  │ Frames  │ │ Frames   │ │ PUSH_PROMISE/etc.  │ │  │
│  │  └─────────┘ └──────────┘ └────────────────────┘ │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │                ANALYTICS ENGINE                   │  │
│  │  ┌──────────┐ ┌──────────┐ ┌────────────────┐   │  │
│  │  │ Metrics  │ │ Timeline │ │   Comparison   │   │  │
│  │  │Collector │ │ Recorder │ │   Calculator   │   │  │
│  │  └──────────┘ └──────────┘ └────────────────┘   │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │               WEB DASHBOARD (Embedded)            │  │
│  │  Vanilla JS + CSS │ SSE Live Updates │ Charts     │  │
│  └───────────────────────────────────────────────────┘  │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │                REST API + CLI                     │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

---

## 4. THREE OPERATING MODES

### 4.1 Prong One — Server Mode

Runs an HTTP/3 test server with configurable test endpoints.

```bash
triton server --listen :4433 --cert cert.pem --key key.pem
```

**Test Endpoints:**

| Endpoint | Purpose |
|---|---|
| `GET /` | Server info + capabilities |
| `GET /ping` | Minimal latency test (1 byte response) |
| `GET /echo` | Echo request headers/body back |
| `GET /download/:size` | Download test (1KB, 10KB, 100KB, 1MB, 10MB, 100MB, 1GB) |
| `POST /upload` | Upload test with throughput measurement |
| `GET /delay/:ms` | Artificial delay response |
| `GET /streams/:n` | Open N concurrent streams with staggered responses |
| `GET /headers/:n` | Response with N custom headers (QPACK stress) |
| `GET /push/:n` | Server push N resources (HTTP/3 push) |
| `GET /redirect/:n` | Chain of N redirects |
| `GET /status/:code` | Return specific HTTP status code |
| `GET /drip/:size/:delay` | Drip-feed response bytes with delay |
| `GET /tls-info` | TLS 1.3 connection details |
| `GET /quic-info` | QUIC connection parameters |
| `GET /migration-test` | Endpoint for migration testing |
| `GET /.well-known/triton` | Machine-readable server capabilities |

**Server Features:**
- Auto-generates self-signed TLS certificates if none provided
- ACME/Let's Encrypt integration for production certs
- Configurable QUIC transport parameters (max streams, flow control windows, idle timeout)
- HTTP/1.1 + HTTP/2 fallback on same port (TCP) for comparison
- Access logging with nanosecond timestamps
- Rate limiting per client IP

### 4.2 Prong Two — Probe Mode

Tests external HTTP/3 servers with deep protocol analysis.

```bash
triton probe https://example.com --full
triton probe https://example.com --0rtt --migration --streams 50
```

**Probe Tests:**

| Test | Description |
|---|---|
| `handshake` | Full QUIC handshake timing (UDP→Initial→Handshake→1-RTT) |
| `0rtt` | 0-RTT resumption test (requires two connections) |
| `migration` | Connection migration (rebind to new UDP port) |
| `streams` | Concurrent stream stress test |
| `throughput` | Upload/download bandwidth measurement |
| `latency` | RTT measurement with percentiles (p50, p95, p99) |
| `mtu` | Path MTU discovery test |
| `ecn` | ECN (Explicit Congestion Notification) support |
| `retry` | Retry token handling |
| `version` | QUIC version negotiation |
| `tls` | TLS 1.3 cipher suites, ALPN, certificate chain |
| `qpack` | Dynamic table usage, header compression ratio |
| `congestion` | Congestion window growth profile |
| `loss` | Packet loss recovery behavior |
| `spin-bit` | Spin bit RTT estimation validation |
| `grease` | GREASE frame/transport parameter handling |
| `h3-settings` | HTTP/3 SETTINGS frame analysis |
| `alt-svc` | Alt-Svc header detection from HTTP/1.1 or HTTP/2 |
| `prioritization` | HTTP/3 priority signal analysis |

**Probe Output Formats:** `--format json|csv|table|yaml|markdown`

### 4.3 Prong Three — Bench Mode

Comparative benchmarking across protocol versions.

```bash
triton bench https://example.com --protocols h1,h2,h3 --concurrency 100 --duration 30s
triton bench --targets targets.yaml --report html
```

**Benchmark Dimensions:**

| Metric | H1.1 | H2 | H3 |
|---|---|---|---|
| Time to First Byte (TTFB) | ✓ | ✓ | ✓ |
| Connection setup time | ✓ | ✓ | ✓ |
| Request throughput (req/s) | ✓ | ✓ | ✓ |
| Download bandwidth | ✓ | ✓ | ✓ |
| Upload bandwidth | ✓ | ✓ | ✓ |
| Concurrent stream perf | - | ✓ | ✓ |
| HoL blocking impact | ✓ | ✓ | ✓ |
| Connection resumption | - | - | ✓ |
| Migration resilience | - | - | ✓ |
| Packet loss resilience | ✓ | ✓ | ✓ |
| Memory footprint | ✓ | ✓ | ✓ |

**Network Simulation (Built-in):**
- Artificial latency injection (`--latency 50ms`)
- Packet loss simulation (`--loss 2%`)
- Bandwidth throttling (`--bandwidth 10mbps`)
- Jitter simulation (`--jitter 10ms`)
- Reordering simulation (`--reorder 1%`)

---

## 5. QUIC ENGINE — CUSTOM IMPLEMENTATION

### 5.1 Packet Layer (RFC 9000)

**Packet Types:**
- Initial (0x00): Connection establishment, contains CRYPTO frames
- 0-RTT (0x01): Early data before handshake completes
- Handshake (0x02): Handshake completion
- Retry (0xF0): Stateless retry for address validation
- Version Negotiation: Protocol version mismatch
- 1-RTT (Short Header): Post-handshake application data

**Packet Number Encoding:**
- Variable-length encoding (1-4 bytes)
- Packet number protection (AES-ECB or ChaCha20 mask)
- Largest acknowledged tracking for decoding

**Header Protection:**
- AES-128-ECB for AES-based cipher suites
- ChaCha20 for ChaCha20-Poly1305 cipher suites
- Sample-based mask generation

### 5.2 Connection Management

**Connection ID:**
- Server-chosen connection IDs (configurable length 4-20 bytes)
- Multiple active connection IDs for migration
- NEW_CONNECTION_ID / RETIRE_CONNECTION_ID frame handling

**Address Validation:**
- Token-based validation (Retry packets)
- NEW_TOKEN frame for future connections
- HMAC-based token generation with expiry

**Connection States:**
```
IDLE → INITIAL_SENT → HANDSHAKE → CONNECTED → DRAINING → CLOSED
                                      ↓
                                  MIGRATING → CONNECTED
```

### 5.3 Stream Multiplexing

**Stream Types:**
- Client-initiated bidirectional (0, 4, 8, ...)
- Server-initiated bidirectional (1, 5, 9, ...)
- Client-initiated unidirectional (2, 6, 10, ...)
- Server-initiated unidirectional (3, 7, 11, ...)

**Stream States:**
```
IDLE → OPEN → HALF-CLOSED(local) → CLOSED
         ↓
    HALF-CLOSED(remote) → CLOSED
         ↓
       RESET
```

**Stream Prioritization:**
- Urgency (0-7) + incremental flag (RFC 9218)
- Extensible Priority Scheme

### 5.4 Flow Control

- **Connection-level**: MAX_DATA / DATA_BLOCKED frames
- **Stream-level**: MAX_STREAM_DATA / STREAM_DATA_BLOCKED frames
- **Stream count**: MAX_STREAMS / STREAMS_BLOCKED frames
- Auto-tuning based on RTT and bandwidth estimation

### 5.5 Loss Detection & Recovery (RFC 9002)

**Mechanisms:**
- Packet threshold (kPacketThreshold = 3)
- Time threshold (9/8 × max(smoothed_rtt, latest_rtt))
- Probe Timeout (PTO) with exponential backoff
- Persistent congestion detection

**RTT Estimation:**
- smoothed_rtt = 7/8 × smoothed_rtt + 1/8 × latest_rtt
- rttvar = 3/4 × rttvar + 1/4 × |smoothed_rtt - latest_rtt|
- min_rtt tracking (no smoothing, minimum observed)

### 5.6 Congestion Control

**Algorithms (selectable):**

| Algorithm | Description |
|---|---|
| New Reno | RFC 9002 default, AIMD |
| Cubic | TCP Cubic adaptation for QUIC |
| BBR v2 | Bottleneck Bandwidth and RTT |

**States:** Slow Start → Congestion Avoidance → Recovery
**ECN Support:** CE, ECT(0), ECT(1) codepoint handling

### 5.7 Connection Migration

**Migration Scenarios:**
- NAT rebinding (same IP, new port)
- Network switch (WiFi → cellular)
- Deliberate migration (client-initiated)

**Path Validation:**
- PATH_CHALLENGE / PATH_RESPONSE frame exchange
- Anti-amplification: 3× limit until validated
- Congestion state reset on new path

### 5.8 0-RTT Resumption

**Session Ticket Management:**
- TLS 1.3 session tickets via crypto/tls
- Ticket storage (in-memory + optional file persistence)
- Early data acceptance/rejection handling
- Anti-replay: client_hello hash dedup window (10 seconds)

**Measurement Points:**
- First connection: full handshake timing
- Resumed connection: 0-RTT data timing
- Time saved = full_handshake_time - zero_rtt_time
- Early data bytes sent before handshake completes

---

## 6. HTTP/3 FRAME LAYER (RFC 9114)

### 6.1 Frame Types

| Frame | Type ID | Stream | Description |
|---|---|---|---|
| DATA | 0x00 | Request | Payload body |
| HEADERS | 0x01 | Request | Header block (QPACK-encoded) |
| CANCEL_PUSH | 0x03 | Control | Cancel server push |
| SETTINGS | 0x04 | Control | Connection parameters |
| PUSH_PROMISE | 0x05 | Request | Server push |
| GOAWAY | 0x07 | Control | Graceful shutdown |

### 6.2 QPACK Header Compression (RFC 9204)

- Static table (99 entries, RFC 9204 Appendix A)
- Dynamic table with absolute/relative indexing
- Encoder stream (unidirectional, type 0x02)
- Decoder stream (unidirectional, type 0x03)
- Required Insert Count for ordered delivery
- Blocked stream limit tracking

### 6.3 Control Streams

- Client control stream (type 0x00): SETTINGS, GOAWAY
- Server control stream (type 0x00): SETTINGS, GOAWAY
- QPACK encoder stream (type 0x02)
- QPACK decoder stream (type 0x03)

---

## 7. ANALYTICS ENGINE

### 7.1 Metrics Collection

Every connection produces a `ConnectionTrace` object:

```
ConnectionTrace {
    id:                  string          // Connection ID (hex)
    server_addr:         string          // Server address
    client_addr:         string          // Client address
    quic_version:        uint32          // Negotiated version
    tls_cipher:          string          // TLS cipher suite
    tls_version:         string          // Always "1.3"
    alpn:                string          // "h3", "h3-29", etc.
    
    // Timing (nanosecond precision)
    dns_start:           time            // DNS resolution start
    dns_end:             time            // DNS resolution end
    udp_connect:         time            // UDP socket bind
    initial_sent:        time            // First Initial packet
    initial_received:    time            // First server Initial
    handshake_complete:  time            // TLS handshake done
    first_stream_open:   time            // First request stream
    first_byte_received: time            // TTFB
    connection_close:    time            // Connection closed
    
    // 0-RTT
    zero_rtt_attempted:  bool
    zero_rtt_accepted:   bool
    zero_rtt_bytes:      uint64          // Early data bytes
    zero_rtt_time:       duration        // Time to first 0-RTT byte
    
    // Migration
    migration_count:     int
    migrations:          []MigrationEvent
    
    // Transport
    rtt_samples:         []RTTSample     // All RTT measurements
    smoothed_rtt:        duration
    min_rtt:             duration
    rttvar:              duration
    cwnd_samples:        []CWNDSample    // Congestion window over time
    bytes_sent:          uint64
    bytes_received:      uint64
    packets_sent:        uint64
    packets_received:    uint64
    packets_lost:        uint64
    packets_retransmit:  uint64
    
    // Streams
    streams_opened:      uint64
    streams_completed:   uint64
    streams_reset:       uint64
    max_concurrent:      uint64
    
    // QPACK
    qpack_dynamic_size:  uint64
    qpack_blocked_count: uint64
    header_compress_ratio: float64
    
    // Frames
    frame_counts:        map[FrameType]uint64
    
    // Errors
    errors:              []ConnectionError
}
```

### 7.2 Timeline Recorder

Records every protocol event with nanosecond timestamps for visualization:

```
Event Types:
- packet_sent, packet_received, packet_lost, packet_retransmitted
- frame_sent, frame_received
- stream_opened, stream_closed, stream_reset
- handshake_start, handshake_complete
- zero_rtt_attempt, zero_rtt_accepted, zero_rtt_rejected
- migration_start, path_validated, migration_complete
- cwnd_update, rtt_sample
- flow_control_update
- connection_close
```

**qlog Compatibility:**
Output follows draft-ietf-quic-qlog format for interoperability with existing QUIC analysis tools.

### 7.3 Comparison Calculator

Produces side-by-side comparison reports:

```
ComparisonReport {
    target:         string
    timestamp:      time
    protocols:      []ProtocolResult   // H1, H2, H3
    
    ProtocolResult {
        protocol:       string         // "h1.1", "h2", "h3"
        ttfb:           PercentileSet  // p50, p95, p99, mean, stddev
        throughput:     PercentileSet
        req_per_sec:    PercentileSet
        connect_time:   PercentileSet
        errors:         uint64
        
        // H3-specific
        zero_rtt_gain:  duration       // Only for H3
        migration_ok:   bool           // Only for H3
        hol_impact:     float64        // Measured HoL blocking impact
    }
    
    winner:         map[string]string  // metric → protocol
    summary:        string             // Human-readable summary
}
```

---

## 8. WEB DASHBOARD

### 8.1 Technology Stack

- **HTML5** — Semantic markup, embedded via `embed.FS`
- **CSS3** — Custom properties (dark/light theme), no frameworks
- **Vanilla JavaScript** — ES2020+, no frameworks, no build step
- **SSE** — Server-Sent Events for real-time updates
- **Canvas API** — High-performance charting (no chart library)
- **WebSocket** — Bidirectional control channel

### 8.2 Dashboard Pages

#### 8.2.1 Overview Dashboard (`/`)
- Active connections counter
- Real-time request throughput graph
- Latest probe results summary
- Quick-launch buttons for common tests
- System resource usage (CPU, memory, goroutines)

#### 8.2.2 Server Monitor (`/dashboard/server`)
- Active connections list with details
- Per-connection stream visualization
- Bandwidth usage graph (in/out)
- QUIC transport parameter display
- TLS session cache status
- 0-RTT acceptance rate

#### 8.2.3 Probe Console (`/dashboard/probe`)
- Target URL input + test selection checkboxes
- Real-time test progress with SSE
- Connection timeline waterfall (à la Chrome DevTools)
- Packet-level trace viewer
- QPACK header analysis table
- Export results button

#### 8.2.4 Benchmark Dashboard (`/dashboard/bench`)
- Multi-target configuration
- Protocol comparison bar/line charts
- TTFB distribution histograms
- Throughput over time graphs
- HoL blocking impact visualization
- Network condition simulator controls

#### 8.2.5 Connection Inspector (`/dashboard/inspect/:id`)
- Full connection lifecycle timeline
- Packet-by-packet trace table
- Stream state diagram
- Congestion window graph over time
- RTT measurement graph
- Flow control waterfall
- QPACK dynamic table state
- Frame-level breakdown

#### 8.2.6 Comparison View (`/dashboard/compare`)
- Side-by-side protocol comparison
- Radar chart (multi-metric)
- Winner highlighting per metric
- Historical comparison trends
- Export comparison report (JSON/CSV/PDF)

#### 8.2.7 Settings (`/dashboard/settings`)
- Server configuration
- Default probe parameters
- Theme toggle (dark/light)
- Certificate management
- Export/import configuration

### 8.3 Real-Time Visualizations (Canvas-Based)

| Chart Type | Usage |
|---|---|
| Timeline waterfall | Connection setup phases |
| Line chart | RTT, CWND, throughput over time |
| Bar chart | Protocol comparison metrics |
| Histogram | Latency distribution |
| Radar chart | Multi-metric comparison |
| Flame graph | Packet-level timing |
| Stream diagram | Concurrent stream visualization |
| Gauge | Live throughput / connection count |

---

## 9. REST API

### 9.1 Endpoints

```
# Server Control
GET    /api/v1/status                    # Server status + version
GET    /api/v1/config                    # Current configuration
PUT    /api/v1/config                    # Update configuration (hot-reload)

# Probe Operations
POST   /api/v1/probe                     # Start a probe test
GET    /api/v1/probe/:id                 # Get probe result
GET    /api/v1/probe/:id/events          # SSE stream of probe events
GET    /api/v1/probes                    # List all probe results
DELETE /api/v1/probe/:id                 # Delete probe result

# Benchmark Operations
POST   /api/v1/bench                     # Start a benchmark
GET    /api/v1/bench/:id                 # Get benchmark result
GET    /api/v1/bench/:id/events          # SSE stream of bench events
GET    /api/v1/benches                   # List all benchmark results
DELETE /api/v1/bench/:id                 # Delete benchmark result

# Connection Inspection (Server Mode)
GET    /api/v1/connections               # List active connections
GET    /api/v1/connections/:id           # Connection details
GET    /api/v1/connections/:id/trace     # Connection trace (qlog format)
GET    /api/v1/connections/:id/streams   # Stream listing

# Comparison
POST   /api/v1/compare                   # Run comparison benchmark
GET    /api/v1/compare/:id               # Get comparison result

# Export
GET    /api/v1/export/:type/:id          # Export result (json/csv/markdown/qlog)

# Certificate Management
POST   /api/v1/certs/generate            # Generate self-signed cert
POST   /api/v1/certs/acme               # Request ACME certificate
GET    /api/v1/certs                     # List certificates
```

### 9.2 Response Format

```json
{
    "status": "ok",
    "data": { ... },
    "meta": {
        "request_id": "tr-abc123",
        "duration_ms": 42,
        "timestamp": "2025-01-15T10:30:00Z"
    }
}
```

---

## 10. CLI INTERFACE

### 10.1 Command Structure

```
triton [command] [subcommand] [flags]

Commands:
  server     Start HTTP/3 test server
  probe      Probe an HTTP/3 server
  bench      Run comparative benchmark
  compare    Compare protocols side-by-side
  inspect    Inspect saved connection trace
  certs      Certificate management
  version    Print version info
  help       Help about any command

Global Flags:
  --config string      Config file path (default: triton.yaml)
  --log-level string   Log level: debug|info|warn|error (default: info)
  --format string      Output format: json|table|csv|yaml|markdown (default: table)
  --no-color           Disable colored output
  --quiet              Suppress non-essential output
```

### 10.2 Server Command

```
triton server [flags]

Flags:
  --listen string            Listen address (default: ":4433")
  --listen-tcp string        TCP listen address for H1/H2 fallback (default: ":8443")
  --cert string              TLS certificate file
  --key string               TLS private key file
  --acme-domain string       Domain for ACME certificate
  --acme-email string        ACME account email
  --dashboard                Enable web dashboard (default: true)
  --dashboard-listen string  Dashboard listen address (default: ":9090")
  --max-connections int      Max concurrent connections (default: 10000)
  --idle-timeout duration    Connection idle timeout (default: 30s)
  --max-streams int          Max concurrent streams per connection (default: 100)
  --initial-window int       Initial flow control window bytes (default: 65535)
  --max-window int           Max flow control window bytes (default: 16777216)
  --congestion string        Congestion algorithm: newreno|cubic|bbr (default: cubic)
  --enable-0rtt              Enable 0-RTT (default: true)
  --access-log string        Access log file path
  --trace-dir string         Directory for connection traces
  --rate-limit int           Requests per second per client (default: 0 = unlimited)
```

### 10.3 Probe Command

```
triton probe <url> [flags]

Flags:
  --tests string          Comma-separated tests: all,handshake,0rtt,migration,...
  --full                  Run all tests
  --count int             Number of probe iterations (default: 1)
  --timeout duration      Per-test timeout (default: 10s)
  --0rtt                  Test 0-RTT resumption
  --migration             Test connection migration
  --streams int           Concurrent stream test count (default: 10)
  --download-size string  Download test size (default: 1MB)
  --upload-size string    Upload test size (default: 1MB)
  --save string           Save results to file
  --qlog                  Output qlog format
  --trace                 Enable packet-level tracing
  --insecure              Skip TLS certificate verification
  --sni string            Override TLS SNI
  --alpn string           Override ALPN (default: h3)
```

### 10.4 Bench Command

```
triton bench <url> [flags]

Flags:
  --protocols string      Protocols to test: h1,h2,h3 (default: h1,h2,h3)
  --concurrency int       Concurrent connections (default: 10)
  --streams int           Streams per connection (default: 1)
  --duration duration     Benchmark duration (default: 10s)
  --requests int          Total requests (alternative to duration)
  --method string         HTTP method (default: GET)
  --body string           Request body
  --header strings        Custom headers (repeatable)
  --targets string        Targets file (YAML) for multi-target bench
  --warmup duration       Warmup duration (default: 2s)
  --latency duration      Simulated network latency
  --loss float            Simulated packet loss percentage
  --bandwidth string      Simulated bandwidth limit
  --jitter duration       Simulated jitter
  --report string         Generate report: html|json|csv|markdown
  --compare-with string   Compare with saved benchmark result
```

---

## 11. CONFIGURATION

### 11.1 Configuration File (triton.yaml)

```yaml
server:
  listen: ":4433"
  listen_tcp: ":8443"
  tls:
    cert: ""
    key: ""
    acme:
      enabled: false
      domain: ""
      email: ""
  dashboard:
    enabled: true
    listen: ":9090"
  quic:
    max_connections: 10000
    idle_timeout: "30s"
    max_streams: 100
    initial_window: 65535
    max_window: 16777216
    congestion: "cubic"
    enable_0rtt: true
    max_0rtt_size: 16384
  rate_limit:
    enabled: false
    requests_per_second: 100
  logging:
    access_log: ""
    trace_dir: ""
    qlog: false

probe:
  timeout: "10s"
  insecure: false
  default_tests:
    - handshake
    - tls
    - 0rtt
    - streams
    - throughput
    - latency

bench:
  warmup: "2s"
  default_duration: "10s"
  default_concurrency: 10
  default_protocols:
    - h1
    - h2
    - h3

storage:
  results_dir: "./triton-data"
  max_results: 1000
  retention: "30d"
```

### 11.2 Environment Variables

All config options available as `TRITON_` prefixed environment variables:
```
TRITON_SERVER_LISTEN=:4433
TRITON_SERVER_TLS_CERT=/path/to/cert.pem
TRITON_DASHBOARD_ENABLED=true
...
```

**Priority:** CLI flags > Environment > Config file > Defaults

---

## 12. STORAGE & PERSISTENCE

### 12.1 Results Storage

- **Format:** JSON files with gzip compression
- **Location:** `./triton-data/` (configurable)
- **Structure:**
  ```
  triton-data/
  ├── probes/
  │   ├── pr-20250115-103000-abc.json.gz
  │   └── ...
  ├── benches/
  │   ├── bn-20250115-110000-def.json.gz
  │   └── ...
  ├── traces/
  │   ├── tr-20250115-103000-abc.qlog.gz
  │   └── ...
  └── certs/
      ├── triton-selfsigned.pem
      └── triton-selfsigned-key.pem
  ```

### 12.2 Session Ticket Store (0-RTT)

- **In-memory LRU cache** with configurable size (default: 1000 entries)
- **Optional file persistence** for cross-restart 0-RTT support
- **TTL:** Configurable, default 24 hours
- **Key derivation:** SHA-256(server_name + server_addr)

---

## 13. SECURITY CONSIDERATIONS

### 13.1 Server Mode
- Amplification attack mitigation (Retry tokens, 3× limit)
- Connection ID length randomization
- Token-based address validation with HMAC
- Rate limiting per source IP
- Max connection limits
- Idle connection cleanup

### 13.2 Probe/Bench Mode
- Certificate verification (default on, `--insecure` to disable)
- No credential storage
- Timeout enforcement on all operations
- Resource limits (max connections, memory)

### 13.3 Dashboard
- Bind to localhost by default
- Optional basic auth
- CORS disabled by default
- CSP headers

---

## 14. BUILD & DISTRIBUTION

### 14.1 Build Matrix

| OS | Arch | Binary Name |
|---|---|---|
| Linux | amd64 | triton-linux-amd64 |
| Linux | arm64 | triton-linux-arm64 |
| macOS | amd64 | triton-darwin-amd64 |
| macOS | arm64 | triton-darwin-arm64 |
| macOS | universal | triton-darwin-universal |
| Windows | amd64 | triton-windows-amd64.exe |
| Windows | arm64 | triton-windows-arm64.exe |

### 14.2 Build Commands

```bash
# Development
go build -o triton ./cmd/triton

# Release (with version, stripped, embedded assets)
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=1.0.0" -o triton ./cmd/triton

# Cross-compile
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ...
```

### 14.3 Docker

```dockerfile
FROM scratch
COPY triton /triton
EXPOSE 4433/udp 8443/tcp 9090/tcp
ENTRYPOINT ["/triton"]
```

### 14.4 Installation Methods

```bash
# Binary download
curl -fsSL https://tritonprobe.com/install.sh | sh

# Go install
go install github.com/tritonprobe/triton/cmd/triton@latest

# Docker
docker run -p 4433:4433/udp -p 9090:9090 tritonprobe/triton server

# Homebrew (macOS/Linux)
brew install tritonprobe/tap/triton
```

---

## 15. PROJECT STRUCTURE

```
triton/
├── cmd/
│   └── triton/
│       └── main.go                 # Entry point, command routing
├── internal/
│   ├── quic/                       # QUIC Engine
│   │   ├── packet/                 # Packet parsing/serialization
│   │   │   ├── header.go           # Long/Short header
│   │   │   ├── initial.go          # Initial packet
│   │   │   ├── handshake.go        # Handshake packet
│   │   │   ├── onertt.go           # 1-RTT packet
│   │   │   ├── zerortt.go          # 0-RTT packet
│   │   │   ├── retry.go            # Retry packet
│   │   │   ├── version_neg.go      # Version Negotiation
│   │   │   ├── protection.go       # Header protection
│   │   │   └── number.go           # Packet number encoding
│   │   ├── frame/                  # QUIC frames
│   │   │   ├── types.go            # Frame type definitions
│   │   │   ├── ack.go              # ACK frame
│   │   │   ├── stream.go           # STREAM frame
│   │   │   ├── crypto.go           # CRYPTO frame
│   │   │   ├── flow.go             # MAX_DATA, MAX_STREAM_DATA, etc.
│   │   │   ├── connection.go       # CONNECTION_CLOSE, NEW_CONNECTION_ID
│   │   │   ├── path.go             # PATH_CHALLENGE, PATH_RESPONSE
│   │   │   └── misc.go             # PING, PADDING, NEW_TOKEN, etc.
│   │   ├── stream/                 # Stream multiplexing
│   │   │   ├── manager.go          # Stream lifecycle
│   │   │   ├── stream.go           # Individual stream
│   │   │   ├── flow_control.go     # Per-stream flow control
│   │   │   └── priority.go         # Stream prioritization
│   │   ├── connection/             # Connection management
│   │   │   ├── conn.go             # Connection state machine
│   │   │   ├── id_manager.go       # Connection ID management
│   │   │   ├── migration.go        # Connection migration
│   │   │   └── close.go            # Connection close/drain
│   │   ├── transport/              # Transport layer
│   │   │   ├── udp.go              # UDP socket management
│   │   │   ├── listener.go         # Server-side listener
│   │   │   ├── dialer.go           # Client-side dialer
│   │   │   └── mtu.go              # Path MTU discovery
│   │   ├── tls/                    # TLS 1.3 integration
│   │   │   ├── handshake.go        # QUIC-TLS handshake
│   │   │   ├── keyschedule.go      # Key derivation
│   │   │   ├── zerortt.go          # 0-RTT handling
│   │   │   └── session.go          # Session ticket management
│   │   ├── recovery/               # Loss detection & recovery
│   │   │   ├── detector.go         # Loss detection
│   │   │   ├── rtt.go              # RTT estimation
│   │   │   ├── pto.go              # Probe timeout
│   │   │   └── sent_tracker.go     # Sent packet tracking
│   │   ├── congestion/             # Congestion control
│   │   │   ├── controller.go       # Interface + factory
│   │   │   ├── newreno.go          # New Reno
│   │   │   ├── cubic.go            # Cubic
│   │   │   └── bbr.go              # BBR v2
│   │   ├── token/                  # Token management
│   │   │   ├── retry.go            # Retry tokens
│   │   │   └── new_token.go        # NEW_TOKEN tokens
│   │   └── version.go              # Version negotiation
│   ├── h3/                         # HTTP/3 Layer
│   │   ├── frame/                  # HTTP/3 frames
│   │   │   ├── types.go            # Frame type definitions
│   │   │   ├── data.go             # DATA frame
│   │   │   ├── headers.go          # HEADERS frame
│   │   │   ├── settings.go         # SETTINGS frame
│   │   │   ├── goaway.go           # GOAWAY frame
│   │   │   └── push.go             # PUSH_PROMISE, CANCEL_PUSH
│   │   ├── qpack/                  # QPACK codec
│   │   │   ├── encoder.go          # QPACK encoder
│   │   │   ├── decoder.go          # QPACK decoder
│   │   │   ├── static_table.go     # Static table (99 entries)
│   │   │   ├── dynamic_table.go    # Dynamic table
│   │   │   ├── huffman.go          # Huffman coding
│   │   │   └── instructions.go     # Encoder/decoder instructions
│   │   ├── server.go               # HTTP/3 server
│   │   ├── client.go               # HTTP/3 client
│   │   ├── handler.go              # Request handler interface
│   │   └── transport.go            # H3 ↔ QUIC binding
│   ├── server/                     # Prong One: Test Server
│   │   ├── server.go               # Server orchestrator
│   │   ├── endpoints.go            # Test endpoint handlers
│   │   ├── fallback.go             # HTTP/1.1 + HTTP/2 fallback
│   │   └── acme.go                 # ACME client
│   ├── probe/                      # Prong Two: Probe
│   │   ├── probe.go                # Probe orchestrator
│   │   ├── tests.go                # Individual test implementations
│   │   ├── handshake_test.go       # Handshake analysis
│   │   ├── zerortt_test.go         # 0-RTT testing
│   │   ├── migration_test.go       # Migration testing
│   │   ├── throughput_test.go      # Throughput measurement
│   │   └── report.go              # Result formatting
│   ├── bench/                      # Prong Three: Benchmark
│   │   ├── bench.go                # Benchmark orchestrator
│   │   ├── runner.go               # Protocol-specific runners
│   │   ├── h1_runner.go            # HTTP/1.1 benchmark
│   │   ├── h2_runner.go            # HTTP/2 benchmark
│   │   ├── h3_runner.go            # HTTP/3 benchmark
│   │   ├── simulator.go            # Network condition simulator
│   │   ├── comparison.go           # Cross-protocol comparison
│   │   └── report.go               # Benchmark report generator
│   ├── analytics/                  # Analytics Engine
│   │   ├── collector.go            # Metrics collector
│   │   ├── timeline.go             # Timeline recorder
│   │   ├── comparison.go           # Comparison calculator
│   │   ├── qlog.go                 # qlog format writer
│   │   └── percentile.go           # Percentile calculator
│   ├── dashboard/                  # Web Dashboard
│   │   ├── server.go               # Dashboard HTTP server
│   │   ├── api.go                  # REST API handlers
│   │   ├── sse.go                  # SSE event broadcaster
│   │   ├── websocket.go            # WebSocket handler
│   │   └── assets/                 # Embedded static files
│   │       ├── index.html
│   │       ├── css/
│   │       │   └── triton.css
│   │       └── js/
│   │           ├── app.js
│   │           ├── charts.js       # Canvas-based charts
│   │           ├── timeline.js     # Connection timeline
│   │           ├── comparison.js   # Comparison view
│   │           └── inspector.js    # Connection inspector
│   ├── config/                     # Configuration
│   │   ├── config.go               # Config struct + defaults
│   │   ├── loader.go               # YAML + env + flag merging
│   │   └── validate.go             # Config validation
│   ├── storage/                    # Results Persistence
│   │   ├── store.go                # Storage interface
│   │   ├── filesystem.go           # Filesystem backend
│   │   └── cleanup.go              # Retention/cleanup
│   └── cli/                        # CLI Interface
│       ├── root.go                 # Root command
│       ├── server.go               # Server subcommand
│       ├── probe.go                # Probe subcommand
│       ├── bench.go                # Bench subcommand
│       ├── compare.go              # Compare subcommand
│       ├── inspect.go              # Inspect subcommand
│       ├── certs.go                # Certs subcommand
│       ├── flags.go                # Flag definitions
│       └── output.go               # Output formatting (table, json, etc.)
├── web/                            # Web UI source (pre-build)
│   ├── index.html
│   ├── css/
│   │   └── triton.css
│   └── js/
│       ├── app.js
│       ├── charts.js
│       ├── timeline.js
│       ├── comparison.js
│       └── inspector.js
├── testdata/                       # Test fixtures
│   ├── certs/
│   ├── captures/
│   └── configs/
├── triton.yaml.example             # Example configuration
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
├── README.md
├── SPECIFICATION.md
├── IMPLEMENTATION.md
├── TASKS.md
├── BRANDING.md
├── LICENSE
└── .github/
    └── workflows/
        ├── build.yml
        ├── test.yml
        └── release.yml
```

---

## 16. TESTING STRATEGY

### 16.1 Unit Tests
- Packet encoding/decoding round-trip
- Frame serialization/deserialization
- QPACK encode/decode with various table states
- Flow control arithmetic
- RTT estimation with known inputs
- Congestion window transitions
- Connection state machine transitions

### 16.2 Integration Tests
- Full handshake between Triton server & client
- 0-RTT resumption end-to-end
- Connection migration simulation
- Multi-stream concurrent transfer
- Protocol comparison with known endpoints

### 16.3 Interop Tests
- Test against known HTTP/3 servers (Google, Cloudflare, Facebook)
- Verify qlog output compatibility
- Cross-validate RTT measurements

### 16.4 Fuzz Tests
- Packet parser fuzzing
- Frame parser fuzzing
- QPACK decoder fuzzing
- Config parser fuzzing

---

## 17. PERFORMANCE TARGETS

| Metric | Target |
|---|---|
| Binary size | < 15 MB |
| Memory (idle server) | < 20 MB |
| Memory (10K connections) | < 500 MB |
| Handshake latency (loopback) | < 1 ms |
| Probe report generation | < 100 ms |
| Dashboard page load | < 200 ms |
| Max concurrent connections | 10,000+ |
| Benchmark throughput | > 100K req/s (loopback) |
| Startup time | < 500 ms |

---

## 18. FUTURE ROADMAP (Post v1.0)

- **WebTransport** support (RFC 9220)
- **Multipath QUIC** (draft-ietf-quic-multipath)
- **MASQUE** proxy protocol testing
- **QUIC v2** (RFC 9369) support
- **Distributed benchmarking** (multi-node)
- **Grafana/Prometheus** metrics export
- **CI/CD integration** (GitHub Actions / GitLab CI plugins)
- **Browser extension** for in-browser QUIC analysis
- **MCP Server** for LLM-driven protocol testing

---

*Three Prongs. One Binary. Every Packet.*
