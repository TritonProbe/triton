# BRANDING.md — Triton

> Current-state note (2026-04-14): this document is brand-direction guidance, not a guarantee that every phrase below reflects shipped behavior today. The supported product path is real HTTP diagnostics plus real HTTP/3 via `quic-go`; the in-repo custom QUIC/H3 engine remains lab-only, and several advanced probe metrics are still `observed` or `partial` rather than packet-level telemetry.

## HTTP/3 (QUIC) Test Server & Benchmarking Platform

---

## 1. NAME & IDENTITY

### 1.1 Name

**Triton** `/ˈtraɪ.tən/`

Named after the Greek god of the sea, son of Poseidon, who carries the legendary **trident** — a three-pronged weapon. The three prongs represent HTTP/3's three revolutionary pillars:

1. **QUIC Transport** — UDP-based, encrypted by default
2. **0-RTT Resumption** — Instant reconnection
3. **Stream Multiplexing** — No head-of-line blocking

The sea metaphor extends naturally: networks are oceans of packets, protocols are currents, and Triton navigates them all.

### 1.2 Tagline

**Primary:** "Three Prongs. One Binary. Every Packet."

Use with care:
- Treat "Every Packet" as aspirational brand language, not a literal promise of packet-level visibility in the current product.

**Alternatives:**
- "Master the Third Wave of HTTP."
- "Probe. Bench. Conquer."
- "Where Packets Meet Their Match."
- "The Trident of Protocol Testing."

### 1.3 Domains

- **Primary:** tritonprobe.com
- **Alternative:** tritontest.com, triton-quic.com
- **GitHub:** github.com/tritonprobe/triton

---

## 2. COLOR PALETTE

### 2.1 Primary Colors

| Name | Hex | RGB | Usage |
|---|---|---|---|
| **Trident Blue** | `#0066FF` | 0, 102, 255 | Primary brand, buttons, links, active states |
| **Deep Ocean** | `#001A3D` | 0, 26, 61 | Dark backgrounds, headers, code blocks |
| **Abyss Black** | `#0A0E17` | 10, 14, 23 | Darkest background (dark theme) |

### 2.2 Secondary Colors

| Name | Hex | RGB | Usage |
|---|---|---|---|
| **Wave Teal** | `#00D4AA` | 0, 212, 170 | Success states, 0-RTT highlights, positive metrics |
| **Coral Warning** | `#FF6B4A` | 255, 107, 74 | Warnings, packet loss, latency spikes |
| **Storm Yellow** | `#FFB800` | 255, 184, 0 | Caution, migration events, medium priority |

### 2.3 Protocol Colors (Comparison Charts)

| Protocol | Color | Hex | Rationale |
|---|---|---|---|
| **HTTP/3** | Trident Blue | `#0066FF` | Primary — the protocol Triton is built for |
| **HTTP/2** | Muted Slate | `#6B7B8D` | Secondary — capable but older |
| **HTTP/1.1** | Faded Bronze | `#8B7355` | Tertiary — legacy |

### 2.4 Dashboard Theme Colors

**Dark Theme (Default):**
```css
--bg-primary:    #0A0E17;    /* Abyss Black */
--bg-secondary:  #111827;    /* Panel background */
--bg-tertiary:   #1F2937;    /* Card background */
--text-primary:  #F9FAFB;    /* High contrast text */
--text-secondary:#9CA3AF;    /* Muted text */
--border:        #374151;    /* Subtle borders */
--accent:        #0066FF;    /* Trident Blue */
--success:       #00D4AA;    /* Wave Teal */
--warning:       #FFB800;    /* Storm Yellow */
--error:         #FF6B4A;    /* Coral Warning */
```

**Light Theme:**
```css
--bg-primary:    #FFFFFF;
--bg-secondary:  #F3F4F6;
--bg-tertiary:   #E5E7EB;
--text-primary:  #111827;
--text-secondary:#6B7280;
--border:        #D1D5DB;
--accent:        #0052CC;    /* Darker Trident Blue */
--success:       #059669;
--warning:       #D97706;
--error:         #DC2626;
```

---

## 3. TYPOGRAPHY

### 3.1 Font Stack

| Usage | Font | Fallback |
|---|---|---|
| **Headings** | JetBrains Mono Bold | monospace |
| **Body** | Inter | system-ui, -apple-system, sans-serif |
| **Code/Data** | JetBrains Mono | Fira Code, Consolas, monospace |
| **Dashboard Metrics** | JetBrains Mono Medium | monospace |

### 3.2 Type Scale

| Element | Size | Weight | Tracking |
|---|---|---|---|
| H1 (Page title) | 32px / 2rem | 700 | -0.02em |
| H2 (Section) | 24px / 1.5rem | 600 | -0.01em |
| H3 (Card title) | 18px / 1.125rem | 600 | 0 |
| Body | 16px / 1rem | 400 | 0 |
| Small/Caption | 14px / 0.875rem | 400 | 0.01em |
| Metric value | 28px / 1.75rem | 500 | -0.02em |
| Code | 14px / 0.875rem | 400 | 0 |

---

## 4. LOGO CONCEPT

### 4.1 Primary Mark

A **stylized trident** formed from three vertical prongs converging into a single base (representing single binary). The prongs have subtle wave curves suggesting data flow / network packets. The center prong is slightly taller, representing HTTP/3 as the primary focus.

**Variations:**
- **Full Logo:** Trident mark + "TRITON" wordmark
- **Compact:** Trident mark only (for favicon, social avatars)
- **Wordmark:** "TRITON" in JetBrains Mono Bold with the "T" slightly stylized

### 4.2 Logo Colors

- **Primary on Dark:** Trident Blue (#0066FF) on Abyss Black (#0A0E17)
- **Primary on Light:** Deep Ocean (#001A3D) on White (#FFFFFF)
- **Monochrome:** White on dark, black on light
- **Accent variation:** Wave Teal (#00D4AA) glow effect on prong tips

### 4.3 Favicon

Simplified trident in 32×32 and 16×16: three prongs only, no base detail. Trident Blue on transparent.

---

## 5. ICONOGRAPHY

### 5.1 Mode Icons

| Mode | Icon Concept | Description |
|---|---|---|
| **Server** | Trident pointing up + signal waves | Active server, broadcasting |
| **Probe** | Trident pointing right + target | Testing/probing external servers |
| **Bench** | Trident + bar chart | Benchmarking/comparison |

### 5.2 Metric Icons (Dashboard)

| Metric | Symbol |
|---|---|
| Latency/RTT | Stopwatch |
| Throughput | Speed gauge |
| Packet loss | Broken chain link |
| Connections | Network nodes |
| Streams | Parallel lines |
| 0-RTT | Lightning bolt |
| Migration | Arrows crossing |
| TLS | Lock/shield |
| QPACK | Compress arrows |
| Error | Triangle warning |

---

## 6. VISUAL LANGUAGE

### 6.1 Dashboard Design Principles

1. **Data-Dense:** Show maximum useful information without clutter
2. **Real-Time First:** Live updates as default, historical on demand
3. **Protocol-Colored:** Consistent H1(bronze)/H2(slate)/H3(blue) everywhere
4. **Dark by Default:** Reduce eye strain for long analysis sessions
5. **Monospaced Metrics:** All numerical data in JetBrains Mono

### 6.2 Chart Style

- **Line charts:** Smooth curves, subtle gradient fills, point markers on hover
- **Bar charts:** Rounded corners (2px radius), 60% width, grouped for comparison
- **Histograms:** Filled bars, no gaps, with p50/p95/p99 marker lines
- **Radar charts:** Filled polygon with semi-transparent background
- **Waterfall:** Horizontal bars with time axis, color-coded by phase
- **Gauges:** Circular arc, gradient fill from success→warning→error

### 6.3 Animation

- **Chart transitions:** 300ms ease-out for data updates
- **Connection events:** Pulse animation on new connections
- **Packet flow:** Subtle left-to-right flow animation in timeline view
- **Metric counters:** Number rolling animation on value change

---

## 7. CLI BRANDING

### 7.1 Banner (--version)

```
  ▄▄▄▄▄▄▄  ▄▄▄▄▄▄   ▄▄▄ ▄▄▄▄▄▄▄  ▄▄▄▄▄▄  ▄▄    ▄
 █  ▄▄▄▄█ █   ▄  █ █   █  ▄▄▄▄█ █      █ █  █  █ █
 █ █▄▄▄▄  █  █▀█ █ █   █ █▄▄▄▄  █  ▄   █ █   █▄█ █
 █▄▄▄▄  █ █   ▄▄▀█ █   █▄▄▄▄  █ █ █ █  █ █  ▄   ██
  ▄▄▄▄█ █ █  █  █  █   █▄▄▄▄█ █ █ █▄█  █ █ █ █   █
 █▄▄▄▄▄▄█ █▄▄█  █▄▄█   █▄▄▄▄▄▄█ █▄▄▄▄▄▄█ █▄█  █▄▄█

  Triton v1.0.0 — HTTP/3 (QUIC) Test Server
  Three Prongs. One Binary. Every Packet.
```

### 7.2 CLI Color Scheme

| Element | Color | ANSI |
|---|---|---|
| Success/pass | Green | `\033[32m` |
| Error/fail | Red | `\033[31m` |
| Warning | Yellow | `\033[33m` |
| Info/metric | Cyan | `\033[36m` |
| Header/title | Bold White | `\033[1;37m` |
| Dimmed/secondary | Gray | `\033[90m` |
| HTTP/3 | Blue | `\033[34m` |
| HTTP/2 | Default | (no color) |
| HTTP/1.1 | Dark | `\033[90m` |

### 7.3 Probe Output Style

```
$ triton probe https://example.com --full

  🔱 Triton Probe — https://example.com

  ┌─────────────────────────────────────────────┐
  │ HANDSHAKE                                   │
  ├─────────────────────────────────────────────┤
  │ DNS Resolution      2.34 ms                 │
  │ UDP Connect         0.12 ms                 │
  │ QUIC Initial        15.67 ms                │
  │ TLS Handshake       28.45 ms                │
  │ Total               46.58 ms                │
  ├─────────────────────────────────────────────┤
  │ TLS                                         │
  ├─────────────────────────────────────────────┤
  │ Version             TLS 1.3                 │
  │ Cipher              TLS_AES_128_GCM_SHA256  │
  │ ALPN                h3                      │
  │ Certificate         *.example.com (valid)   │
  ├─────────────────────────────────────────────┤
  │ 0-RTT                                       │
  ├─────────────────────────────────────────────┤
  │ Supported           ✓ YES                   │
  │ Full Handshake      46.58 ms                │
  │ 0-RTT Resume        12.34 ms                │
  │ Time Saved          34.24 ms (73.5%)        │
  ├─────────────────────────────────────────────┤
  │ THROUGHPUT                                  │
  ├─────────────────────────────────────────────┤
  │ Download (1MB)      245.6 Mbps              │
  │ Upload (1MB)        198.3 Mbps              │
  │ TTFB (p50)          18.2 ms                 │
  │ TTFB (p99)          42.7 ms                 │
  └─────────────────────────────────────────────┘
```

---

## 8. SOCIAL MEDIA & MARKETING

### 8.1 Key Messages

- "The only HTTP/3 test tool you'll ever need — in a single Go binary."
- "Stop guessing if your server supports QUIC. Triton knows."
- "See the 73% handshake time savings 0-RTT gives you."
- "HTTP/3 vs HTTP/2 vs HTTP/1.1 — side by side, no opinions, just data."
- "Connection migration: because Wi-Fi→cellular shouldn't break your session."

### 8.2 Target Audience

1. **Backend Engineers** — Deploying HTTP/3, need to validate configuration
2. **SRE/DevOps** — Performance benchmarking, protocol comparison
3. **Security Engineers** — TLS 1.3 analysis, certificate chain inspection
4. **Network Engineers** — QUIC protocol analysis, congestion profiling
5. **Protocol Researchers** — qlog output, packet-level inspection

### 8.3 Hashtags

`#HTTP3` `#QUIC` `#GoLang` `#OpenSource` `#NetworkTesting` `#WebPerformance` `#ZeroDependency` `#SingleBinary` `#Triton`

---

## 9. COMPARISON POSITIONING

### 9.1 vs curl

| Feature | curl | Triton |
|---|---|---|
| HTTP/3 support | Via nghttp3+ngtcp2 | Built-in pure Go |
| 0-RTT testing | Manual | Automated with timing |
| Migration test | No | Built-in |
| Comparison bench | No | H1/H2/H3 side-by-side |
| Web dashboard | No | Built-in |
| Protocol analysis | Basic timing | Full packet/frame/stream |
| Single binary | External deps | Yes, always |

### 9.2 vs h2load / wrk

| Feature | h2load/wrk | Triton |
|---|---|---|
| HTTP/3 | h2load: yes, wrk: no | Yes |
| Protocol comparison | Single protocol | All three |
| 0-RTT measurement | No | Yes |
| Network simulation | No | Built-in |
| Dashboard | No | Built-in |
| qlog output | No | Yes |
| QUIC analysis | No | Full stack |

---

## 10. INFOGRAPHIC PROMPT (Nano Banana 2)

### 10.1 Logo/Icon Generation

```
Prompt for Triton logo:
"Minimalist tech logo, stylized trident symbol with three vertical prongs,
center prong slightly taller, prongs have subtle wave curves suggesting data
flow, converging into single base point, electric blue (#0066FF) on dark navy
(#0A0E17) background, clean geometric lines, modern tech aesthetic, suitable
for favicon and social media avatar"
```

### 10.2 Feature Infographic

```
Prompt for Triton feature overview:
"Modern tech infographic, dark background (#0A0E17), title 'TRITON - HTTP/3
Test Server', three-column layout representing three modes: Server (signal
tower icon), Probe (target/crosshair icon), Bench (chart icon), each column
lists 4-5 key features in clean monospaced font, bottom section shows protocol
comparison bars (HTTP/3 blue, HTTP/2 gray, HTTP/1.1 bronze), minimal design,
developer-focused, includes trident logo mark at top"
```

### 10.3 Protocol Comparison Infographic

```
Prompt for H1/H2/H3 comparison:
"Side-by-side protocol comparison infographic, three columns: HTTP/1.1
(bronze/brown), HTTP/2 (slate gray), HTTP/3 (electric blue), rows comparing:
Connection Setup, Multiplexing, Head-of-Line Blocking, Migration, Encryption,
each cell has icon + short description, HTTP/3 column highlighted with subtle
glow, dark tech background, clean typography, developer-friendly design"
```

---

*Three Prongs. One Binary. Every Packet.*
