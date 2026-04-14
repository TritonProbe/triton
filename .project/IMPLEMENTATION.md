# IMPLEMENTATION.md — Triton

## HTTP/3 (QUIC) Test Server & Benchmarking Platform

---

## CURRENT IMPLEMENTATION SCOPE (2026-04-14)

This document contains target-state implementation notes, but the active repository currently follows a dual-track model:

- Supported track:
- HTTP server/runtime in `internal/server`, `internal/appmux`, `internal/probe`, `internal/bench`, `internal/dashboard`.
- Real HTTP/3 behavior implemented through `internal/realh3` (`quic-go`) for `h3://` probe/bench and optional server listener.
- Experimental track:
- `internal/quic/*` and `internal/h3/*` are intentionally lab-grade transport research and not RFC-complete production QUIC/H3.
- Current hardening already in place:
- Explicit opt-in guards for experimental transport, remote dashboard, insecure TLS, and mixed H3 planes.
- CI includes `go test`, `go vet`, `staticcheck`, `gosec`, smoke flow, and a dedicated CGO race job.
- Dashboard now includes list filtering/sorting/limits and compare/trend summaries from stored probe/bench rollups.
- Product/lab boundary and conditional custom-engine vNext milestones are documented in [ENGINE_STRATEGY.md](./ENGINE_STRATEGY.md).

Use this section as the implementation truth boundary when the rest of this file describes longer-term target internals.

Do not treat the detailed sections below as proof that every named package or subsystem exists in the repository today. Large portions of this file still document the intended end-state architecture.

### Current implementation truth summary

- Supported HTTP/3 behavior is implemented through `internal/realh3` and `quic-go`.
- `internal/quic/*` and `internal/h3/*` are experimental lab-only transport research.
- Current advanced probe fields such as `0rtt`, `migration`, `qpack`, `loss`, `congestion`, `retry`, `version`, `ecn`, and `spin-bit` are not all packet-level implementations; several are explicitly heuristic or capability-based approximations in the shipped code.

---

## 1. QUIC ENGINE IMPLEMENTATION

### 1.1 UDP Socket Layer

```
internal/quic/transport/udp.go

UDPTransport struct:
    conn         *net.UDPConn
    readBuffer   []byte        // 65535 bytes, reused
    writeBuffer  []byte        // 65535 bytes, reused
    readTimeout  time.Duration
    writeTimeout time.Duration
    mtu          int           // Discovered MTU (default: 1200, min for QUIC)
    ecn          bool          // ECN support flag
    gso          bool          // GSO support (Linux only, x/sys)

Methods:
    ReadPacket()  → ([]byte, *net.UDPAddr, error)
    WritePacket(data []byte, addr *net.UDPAddr) → error
    WriteBatch(packets [][]byte, addr *net.UDPAddr) → error  // GSO path
    SetReadDeadline(t time.Time)
    Close()

Platform Optimizations:
    Linux:   SO_REUSEPORT, UDP_GRO, UDP_GSO via x/sys
    macOS:   SO_REUSEPORT
    Windows: Standard net.UDPConn
```

**Buffer Pool:**
```
var packetPool = sync.Pool{
    New: func() interface{} {
        buf := make([]byte, 1452)  // 1500 - 40 (IPv6) - 8 (UDP)
        return &buf
    },
}
```

### 1.2 Packet Parser

```
internal/quic/packet/header.go

ParseHeader(data []byte) → (Header, []byte, error)

Header interface:
    IsLongHeader() bool
    PacketType() PacketType
    Version() uint32
    DestConnectionID() []byte
    SrcConnectionID() []byte  // Long header only

LongHeader struct:
    form          byte         // 1 = long
    fixedBit      byte         // 1
    packetType    PacketType   // 2 bits: Initial(0), 0-RTT(1), Handshake(2), Retry(3)
    reserved      byte         // 2 bits
    pnLength      byte         // 2 bits (encoded as length-1)
    version       uint32
    dcidLen       byte
    dcid          []byte
    scidLen       byte
    scid          []byte
    // Type-specific
    tokenLength   uint64       // Initial only (varint)
    token         []byte       // Initial only
    length        uint64       // Payload length (varint)
    packetNumber  uint64       // After header protection removal

ShortHeader struct:
    form          byte         // 0 = short
    fixedBit      byte         // 1
    spinBit       byte         // 1 bit (latency measurement)
    reserved      byte         // 2 bits
    keyPhase      byte         // 1 bit
    pnLength      byte         // 2 bits (encoded as length-1)
    dcid          []byte       // Length known from connection state
    packetNumber  uint64       // After header protection removal

Encoding:
    Variable-Length Integer (RFC 9000 §16):
    - 1 byte:  6-bit value (0-63), prefix 00
    - 2 bytes: 14-bit value (0-16383), prefix 01
    - 4 bytes: 30-bit value (0-1073741823), prefix 10
    - 8 bytes: 62-bit value (0-4611686018427387903), prefix 11

    ReadVarInt(r io.Reader) → (uint64, int, error)
    WriteVarInt(w io.Writer, v uint64) → (int, error)
    VarIntLen(v uint64) → int
```

### 1.3 Header Protection

```
internal/quic/packet/protection.go

HeaderProtector interface:
    Protect(header []byte, payload []byte, pnOffset int)
    Unprotect(header []byte, payload []byte, pnOffset int)

AESHeaderProtector struct:
    block cipher.Block   // AES-128-ECB from crypto/aes

ChaChaHeaderProtector struct:
    key [32]byte         // ChaCha20 key

Algorithm:
    1. Sample = payload[4:20] (16 bytes starting at pnOffset+4)
    2. For AES: mask = AES-ECB-Encrypt(key, sample)
    3. For ChaCha20: counter = sample[0:4], nonce = sample[4:16], mask = ChaCha20(key, counter, nonce, 5 zero bytes)
    4. Long header: header[0] ^= (mask[0] & 0x0f)
       Short header: header[0] ^= (mask[0] & 0x1f)
    5. pnBytes[i] ^= mask[1+i] for each packet number byte
```

### 1.4 Packet Number Handling

```
internal/quic/packet/number.go

DecodePacketNumber(largest_pn uint64, truncated_pn uint64, pn_nbits int) → uint64
    // RFC 9000 §A.3
    expected := largest_pn + 1
    pn_win := 1 << pn_nbits
    pn_hwin := pn_win / 2
    pn_mask := pn_win - 1
    candidate := (expected & ^uint64(pn_mask)) | truncated_pn
    if candidate <= expected-pn_hwin && candidate < (1<<62)-pn_win {
        return candidate + pn_win
    }
    if candidate > expected+pn_hwin && candidate >= pn_win {
        return candidate - pn_win
    }
    return candidate

EncodePacketNumber(full_pn uint64, largest_acked uint64) → (truncated uint64, nbits int)
    // Minimum bytes to unambiguously represent
    num_unacked := full_pn - largest_acked
    if num_unacked < 0x80 { return full_pn & 0xFF, 8 }
    if num_unacked < 0x8000 { return full_pn & 0xFFFF, 16 }
    if num_unacked < 0x800000 { return full_pn & 0xFFFFFF, 24 }
    return full_pn & 0xFFFFFFFF, 32
```

### 1.5 Frame Parser

```
internal/quic/frame/types.go

Frame interface:
    Type() FrameType
    Serialize(w io.Writer) error
    Length() int

FrameType constants:
    PADDING           = 0x00
    PING              = 0x01
    ACK               = 0x02
    ACK_ECN           = 0x03
    RESET_STREAM      = 0x04
    STOP_SENDING      = 0x05
    CRYPTO            = 0x06
    NEW_TOKEN         = 0x07
    STREAM            = 0x08-0x0f  // 3 low bits: FIN|LEN|OFF
    MAX_DATA          = 0x10
    MAX_STREAM_DATA   = 0x11
    MAX_STREAMS_BIDI  = 0x12
    MAX_STREAMS_UNI   = 0x13
    DATA_BLOCKED      = 0x14
    STREAM_DATA_BLOCKED = 0x15
    STREAMS_BLOCKED_BIDI = 0x16
    STREAMS_BLOCKED_UNI  = 0x17
    NEW_CONNECTION_ID = 0x18
    RETIRE_CONNECTION_ID = 0x19
    PATH_CHALLENGE    = 0x1a
    PATH_RESPONSE     = 0x1b
    CONNECTION_CLOSE  = 0x1c
    CONNECTION_CLOSE_APP = 0x1d
    HANDSHAKE_DONE    = 0x1e

ParseFrames(data []byte) → ([]Frame, error)
    // Iteratively parse frames from decrypted packet payload
```

**ACK Frame:**
```
internal/quic/frame/ack.go

ACKFrame struct:
    LargestAcked   uint64
    ACKDelay       uint64      // In microseconds (scaled by ack_delay_exponent)
    ACKRangeCount  uint64
    FirstACKRange  uint64      // From LargestAcked
    ACKRanges      []ACKRange  // Additional ranges
    ECNCounts      *ECNCounts  // Only for ACK_ECN (0x03)

ACKRange struct:
    Gap   uint64
    Range uint64

ECNCounts struct:
    ECT0  uint64
    ECT1  uint64
    CE    uint64

Iteration:
    ForEachRange(fn func(start, end uint64)) → iterate all acked ranges
    Contains(pn uint64) → bool
```

**STREAM Frame:**
```
internal/quic/frame/stream.go

StreamFrame struct:
    StreamID   uint64
    Offset     uint64    // Present if OFF bit set
    Length     uint64    // Present if LEN bit set
    Fin        bool      // FIN bit
    Data       []byte

Flags (from frame type byte):
    OFF = 0x04  // Offset field present
    LEN = 0x02  // Length field present
    FIN = 0x01  // Final data on stream
```

### 1.6 Connection State Machine

```
internal/quic/connection/conn.go

Connection struct:
    state              ConnectionState
    role               Role              // Client or Server
    version            uint32            // Negotiated QUIC version
    
    // Connection IDs
    localCIDs          *CIDManager
    remoteCIDs         *CIDManager
    originalDCID       []byte            // For transport param validation
    
    // TLS
    tlsConn            *QUICTLSConn      // Wrapper around crypto/tls
    handshakeComplete  chan struct{}
    
    // Streams
    streamManager      *StreamManager
    
    // Flow control
    localMaxData       uint64
    remoteMaxData      uint64
    dataSent           uint64
    dataReceived       uint64
    
    // Recovery
    lossDetector       *LossDetector
    congestionCtrl     CongestionController
    
    // Addresses
    localAddr          *net.UDPAddr
    remoteAddr         *net.UDPAddr
    
    // 0-RTT
    zeroRTTEnabled     bool
    zeroRTTAccepted    bool
    zeroRTTData        []byte
    
    // Migration
    pathValidator      *PathValidator
    
    // Transport parameters
    localParams        TransportParameters
    remoteParams       TransportParameters
    
    // Tracing
    tracer             *ConnectionTracer
    
    // Crypto
    initialKeys        *CryptoKeys
    handshakeKeys      *CryptoKeys
    oneRTTKeys         *CryptoKeys
    zeroRTTKeys        *CryptoKeys
    
    // Timers
    idleTimer          *time.Timer
    drainingTimer      *time.Timer
    
    // Channels
    incoming           chan *ReceivedPacket
    outgoing           chan *OutgoingPacket
    closeSignal        chan error

ConnectionState enum:
    StateIdle
    StateInitialSent
    StateInitialReceived
    StateHandshake
    StateConnected
    StateDraining
    StateClosed

Run():
    // Main connection loop (goroutine)
    for {
        select {
        case pkt := <-c.incoming:
            c.handlePacket(pkt)
        case <-c.idleTimer.C:
            c.closeIdle()
        case <-c.drainingTimer.C:
            c.closeDraining()
        case <-c.closeSignal:
            return
        default:
            c.maybeSendPackets()
            c.lossDetector.CheckTimeouts()
        }
    }
```

### 1.7 Transport Parameters

```
TransportParameters struct:
    OriginalDestConnectionID   []byte   // Server only
    MaxIdleTimeout             time.Duration
    StatelessResetToken        [16]byte // Server only
    MaxUDPPayloadSize          uint64   // Default: 65527
    InitialMaxData             uint64
    InitialMaxStreamDataBidiLocal  uint64
    InitialMaxStreamDataBidiRemote uint64
    InitialMaxStreamDataUni    uint64
    InitialMaxStreamsBidi      uint64
    InitialMaxStreamsUni        uint64
    AckDelayExponent           uint64   // Default: 3
    MaxAckDelay                time.Duration // Default: 25ms
    DisableActiveMigration     bool
    PreferredAddress           *PreferredAddress
    ActiveConnectionIDLimit    uint64   // Default: 2
    InitialSourceConnectionID  []byte
    RetrySourceConnectionID    []byte   // Server only

Serialize() → []byte   // TLS extension format
Deserialize([]byte) → error
```

### 1.8 TLS 1.3 Integration

```
internal/quic/tls/handshake.go

QUICTLSConn struct:
    tlsConn        *tls.Conn
    quicTransport  io.ReadWriter   // QUIC CRYPTO stream adapter
    keyLog         io.Writer       // SSLKEYLOGFILE support
    
    // Callback channels
    initialKeys    chan *CryptoKeys
    handshakeKeys  chan *CryptoKeys
    oneRTTKeys     chan *CryptoKeys
    zeroRTTKeys    chan *CryptoKeys

Key Derivation (RFC 9001):
    Initial Keys:
        initial_salt = 0x38762cf7f55934b34d179ae6a4c80cadccbb7f0a (QUIC v1)
        initial_secret = HKDF-Extract(initial_salt, client_dcid)
        client_initial_secret = HKDF-Expand-Label(initial_secret, "client in", "", 32)
        server_initial_secret = HKDF-Expand-Label(initial_secret, "server in", "", 32)
        
    From each directional secret:
        key = HKDF-Expand-Label(secret, "quic key", "", key_length)
        iv  = HKDF-Expand-Label(secret, "quic iv", "", 12)
        hp  = HKDF-Expand-Label(secret, "quic hp", "", key_length)

    HKDF-Expand-Label implementation using golang.org/x/crypto/hkdf

Cipher Suites (via crypto/tls):
    TLS_AES_128_GCM_SHA256        (0x1301) — Required
    TLS_AES_256_GCM_SHA384        (0x1302) — Recommended
    TLS_CHACHA20_POLY1305_SHA256  (0x1303) — Recommended

ALPN: "h3" (HTTP/3), "h3-29" (draft-29 compat)
```

### 1.9 Stream Manager

```
internal/quic/stream/manager.go

StreamManager struct:
    streams        sync.Map           // streamID → *Stream
    nextBidiLocal  uint64             // Next local bidi stream ID
    nextBidiRemote uint64             // Next remote bidi stream ID
    nextUniLocal   uint64             // Next local uni stream ID
    nextUniRemote  uint64             // Next remote uni stream ID
    
    maxBidiLocal   uint64             // Peer's MAX_STREAMS limit
    maxBidiRemote  uint64             // Our MAX_STREAMS limit
    maxUniLocal    uint64
    maxUniRemote   uint64
    
    role           Role               // Client/Server affects ID parity

OpenStream(bidi bool) → (*Stream, error)
AcceptStream() → (*Stream, error)
GetStream(id uint64) → (*Stream, bool)
CloseStream(id uint64)

Stream struct:
    id             uint64
    state          StreamState
    
    // Send side
    sendBuf        *sendBuffer
    sendOffset     uint64
    sendFin        bool
    sendBlocked    bool
    maxSendData    uint64              // Peer's MAX_STREAM_DATA
    
    // Receive side
    recvBuf        *recvBuffer        // Ordered reassembly buffer
    recvOffset     uint64             // Next expected offset
    recvFin        bool
    recvFinOffset  uint64
    maxRecvData    uint64              // Our MAX_STREAM_DATA
    
    // Flow control notifications
    flowUpdate     chan struct{}
    
    // io.ReadWriteCloser implementation
    Read(p []byte) → (int, error)
    Write(p []byte) → (int, error)
    Close() → error
    CloseWrite() → error              // Send FIN
    Reset(errorCode uint64)           // Send RESET_STREAM

recvBuffer:
    // Gap-based reassembly buffer for out-of-order data
    chunks     []dataChunk           // Sorted by offset
    
    dataChunk struct:
        offset uint64
        data   []byte
    
    Insert(offset uint64, data []byte, fin bool) → error
    Read(p []byte) → (int, error)
    Readable() int                    // Contiguous bytes available
```

### 1.10 Loss Detection

```
internal/quic/recovery/detector.go

LossDetector struct:
    // RTT
    smoothedRTT    time.Duration
    rttVar         time.Duration
    minRTT         time.Duration
    latestRTT      time.Duration
    firstRTTSample time.Time
    
    // Sent packets (per encryption level)
    initialSpace   *PacketNumberSpace
    handshakeSpace *PacketNumberSpace
    appDataSpace   *PacketNumberSpace
    
    // Timers
    lossTime       time.Time          // Earliest loss timer
    ptoCount       int                // PTO backoff counter
    
    // Config
    kPacketThreshold int              // Default: 3
    kTimeThreshold   float64          // Default: 9/8
    kGranularity     time.Duration    // Default: 1ms
    maxAckDelay      time.Duration    // From transport params

PacketNumberSpace struct:
    largestAcked     int64            // -1 if none
    sentPackets      map[uint64]*SentPacket
    ackEliciting     int              // Count of unacked ack-eliciting
    lossTime         time.Time

SentPacket struct:
    packetNumber   uint64
    ackEliciting   bool
    inFlight       bool
    sentBytes      int
    timeSent       time.Time
    frames         []Frame            // For retransmission

OnAckReceived(space, ackFrame):
    1. Update RTT if newly acked largest
    2. Mark acked packets
    3. Detect lost packets (threshold check)
    4. Reset PTO timer
    5. Notify congestion controller

DetectLostPackets(space) → []SentPacket:
    loss_delay = max(latest_rtt, smoothed_rtt) * kTimeThreshold
    loss_delay = max(loss_delay, kGranularity)
    lost_send_time = now - loss_delay
    
    for each unacked packet in space:
        if packet.timeSent < lost_send_time:
            mark as lost
        else if largestAcked - packet.number >= kPacketThreshold:
            mark as lost

GetPTOTimeout() → time.Duration:
    smoothed_rtt + max(4*rttVar, kGranularity) + maxAckDelay
    Then multiply by 2^ptoCount for backoff
```

### 1.11 Congestion Control

```
internal/quic/congestion/controller.go

CongestionController interface:
    OnPacketSent(bytes int)
    OnPacketAcked(bytes int, rtt time.Duration)
    OnPacketLost(bytes int)
    OnCongestionEvent()
    CWND() int
    InSlowStart() bool
    InRecovery() bool

internal/quic/congestion/newreno.go

NewReno struct:
    cwnd              int             // Congestion window (bytes)
    ssthresh          int             // Slow start threshold
    bytesInFlight     int
    recoveryStartTime time.Time
    maxDatagramSize   int             // Default: 1200

    // Constants
    initialWindow     int             // min(14720, max(14720, 2*mds))
    minimumWindow     int             // 2 * maxDatagramSize

OnPacketAcked(bytes int, rtt time.Duration):
    if InRecovery(): return          // No growth in recovery
    if InSlowStart():
        cwnd += bytes                // Exponential growth
    else:
        cwnd += maxDatagramSize * bytes / cwnd  // Linear growth (AIMD)

OnPacketLost(bytes int):
    bytesInFlight -= bytes
    if !InRecovery():
        recoveryStartTime = now
        ssthresh = max(cwnd/2, minimumWindow)
        cwnd = ssthresh

internal/quic/congestion/cubic.go
    // Cubic function: W(t) = C*(t-K)^3 + W_max
    // K = cbrt(W_max * beta / C)
    // C = 0.4, beta = 0.7

internal/quic/congestion/bbr.go
    // BBR v2 state machine:
    // Startup → Drain → ProbeBW (Cruise/Refill/Up/Down) → ProbeRTT
    // Pacing rate = bw * pacing_gain
    // CWND = bw * min_rtt * cwnd_gain
```

### 1.12 Connection Migration

```
internal/quic/connection/migration.go

PathValidator struct:
    pendingChallenges  map[string]*PathChallenge
    validated          map[string]bool         // addr → validated
    antiAmplification  map[string]int          // addr → bytes sent

PathChallenge struct:
    data      [8]byte
    addr      *net.UDPAddr
    sentTime  time.Time
    timeout   time.Duration

OnPeerAddressChange(newAddr *net.UDPAddr):
    1. Check DisableActiveMigration transport param
    2. Switch to new path
    3. Send PATH_CHALLENGE to new address
    4. Reset congestion state
    5. Apply anti-amplification limit (3×)
    6. Wait for PATH_RESPONSE

ValidatePath(addr *net.UDPAddr):
    1. Generate 8 random bytes
    2. Send PATH_CHALLENGE frame
    3. Start validation timer
    4. On PATH_RESPONSE match → path validated
    5. On timeout → retry or abandon

OnPathValidated(addr *net.UDPAddr):
    1. Remove anti-amplification limit
    2. Keep old path for a bit (in case of spurious migration)
    3. May retire old connection IDs
```

---

## 2. HTTP/3 LAYER IMPLEMENTATION

### 2.1 QPACK Encoder/Decoder

```
internal/h3/qpack/static_table.go
    // 99 pre-defined header field entries (RFC 9204 Appendix A)
    // Index 0: ":authority" ""
    // Index 1: ":path" "/"
    // ...
    // Index 98: "x-frame-options" "sameorigin"
    
    staticTable [99]HeaderField

internal/h3/qpack/dynamic_table.go

DynamicTable struct:
    entries       []HeaderField
    capacity      uint64          // Max capacity in bytes
    size          uint64          // Current size
    insertCount   uint64          // Absolute index counter
    droppedCount  uint64          // Entries evicted

    Insert(field HeaderField) → error
    Lookup(index uint64) → (HeaderField, bool)
    Evict() → removes oldest entries to make room

internal/h3/qpack/encoder.go

Encoder struct:
    dynamicTable   *DynamicTable
    maxTableCap    uint64
    blockedStreams int
    maxBlocked     int
    
    // Encoding strategies
    useStaticRef   bool
    useDynamicRef  bool
    useLiteral     bool
    huffman        bool

EncodeHeaders(headers []HeaderField) → (encoded []byte, instructions []byte)
    // encoded: goes on request stream (HEADERS frame)
    // instructions: goes on encoder stream

internal/h3/qpack/decoder.go

Decoder struct:
    dynamicTable     *DynamicTable
    maxTableCap      uint64
    requiredInserts  uint64         // Track Required Insert Count

DecodeHeaders(data []byte, requiredInsertCount uint64) → ([]HeaderField, error)

internal/h3/qpack/huffman.go
    // RFC 7541 Appendix B Huffman table (same as HPACK)
    HuffmanEncode(data []byte) → []byte
    HuffmanDecode(data []byte) → ([]byte, error)
```

### 2.2 HTTP/3 Server

```
internal/h3/server.go

H3Server struct:
    quicListener   *quic.Listener
    handler        http.Handler       // Standard http.Handler interface!
    qpackEncoder   *qpack.Encoder
    qpackDecoder   *qpack.Decoder
    settings       H3Settings
    
    // Control streams
    controlStream  quic.Stream        // Type 0x00
    encoderStream  quic.Stream        // Type 0x02
    decoderStream  quic.Stream        // Type 0x03

H3Settings struct:
    MaxFieldSectionSize  uint64       // SETTINGS_MAX_FIELD_SECTION_SIZE (0x06)
    QPACKMaxTableCap     uint64       // SETTINGS_QPACK_MAX_TABLE_CAPACITY (0x01)
    QPACKBlockedStreams   uint64       // SETTINGS_QPACK_BLOCKED_STREAMS (0x07)

Serve():
    1. Accept QUIC connection
    2. Open control stream, send SETTINGS
    3. Accept peer's control stream, receive SETTINGS
    4. Open QPACK encoder/decoder streams
    5. Accept request streams
    6. For each request stream (goroutine):
       a. Read HEADERS frame → decode via QPACK → build http.Request
       b. Read DATA frames → request body
       c. Call handler.ServeHTTP(responseWriter, request)
       d. Encode response headers via QPACK → write HEADERS frame
       e. Write DATA frames for response body
       f. Close stream
```

### 2.3 HTTP/3 Client

```
internal/h3/client.go

H3Client struct:
    quicConn       *quic.Connection
    qpackEncoder   *qpack.Encoder
    qpackDecoder   *qpack.Decoder
    settings       H3Settings
    
    // Stream management
    activeRequests sync.Map          // streamID → *requestState

RoundTrip(req *http.Request) → (*http.Response, error):
    1. Open bidirectional stream
    2. Encode request headers via QPACK
    3. Write HEADERS frame
    4. Write DATA frames (request body)
    5. Send FIN on write side
    6. Read HEADERS frame → decode via QPACK → build http.Response
    7. Return response (body reads from stream)
```

---

## 3. SERVER MODE IMPLEMENTATION

### 3.1 Test Endpoint Handlers

```
internal/server/endpoints.go

// All handlers implement http.Handler

PingHandler:        → 200, body: "pong" (2 bytes)
EchoHandler:        → 200, echo back all request headers + body
DownloadHandler:    → 200, body: N bytes of deterministic data (repeating pattern)
UploadHandler:      → 200, read full body, return upload stats JSON
DelayHandler:       → sleep(N ms), then 200
StreamsHandler:     → open N concurrent push streams with staggered data
HeadersHandler:     → 200 with N custom X-Triton-N headers
RedirectHandler:    → 302 chain of N redirects
StatusHandler:      → return specified HTTP status code
DripHandler:        → write bytes one-at-a-time with delay
TLSInfoHandler:     → JSON with TLS connection details
QUICInfoHandler:    → JSON with QUIC transport parameters
MigrationHandler:   → long-lived response for migration testing
CapabilitiesHandler → JSON server capabilities (/.well-known/triton)
```

### 3.2 HTTP/1.1 + HTTP/2 Fallback

```
internal/server/fallback.go

FallbackServer struct:
    httpServer     *http.Server      // Standard library HTTP server
    handler        http.Handler      // Same handlers as H3
    tlsConfig      *tls.Config       // ALPN: h2, http/1.1

    // Auto-negotiation
    // TLS ALPN selects h2 or http/1.1
    // Same test endpoints available on TCP
    // Enables fair H1/H2/H3 comparison
```

### 3.3 ACME Client

```
internal/server/acme.go

ACMEClient struct:
    directory    string              // Let's Encrypt directory URL
    accountKey   crypto.PrivateKey   // ECDSA P-256
    domain       string
    email        string
    certDir      string

    // Challenge handlers
    httpChallenge  map[string]string  // token → key_authorization
    tlsAlpnChallenge map[string]*tls.Certificate

Workflow:
    1. Create/load account key
    2. Register/find ACME account
    3. Create order for domain
    4. Solve HTTP-01 or TLS-ALPN-01 challenge
    5. Finalize with CSR
    6. Download certificate chain
    7. Auto-renewal (30 days before expiry)

Implementation: Pure HTTP client using net/http, JSON encoding via encoding/json.
No external ACME library needed.
```

---

## 4. PROBE MODE IMPLEMENTATION

### 4.1 Probe Orchestrator

```
internal/probe/probe.go

Probe struct:
    target       string              // URL to probe
    tests        []TestType
    config       ProbeConfig
    h3Client     *H3Client
    h2Client     *http.Client        // For comparison
    h1Client     *http.Client        // For comparison
    tracer       *ConnectionTracer
    results      *ProbeResult

Run() → (*ProbeResult, error):
    1. DNS resolution + timing
    2. For each selected test:
       a. Create fresh connection (or reuse for 0-RTT)
       b. Execute test with tracing enabled
       c. Collect metrics
    3. Generate ProbeResult with all metrics
    4. Output in requested format

ProbeResult struct:
    Target        string
    Timestamp     time.Time
    Duration      time.Duration
    Tests         map[TestType]*TestResult
    ConnectionTrace *ConnectionTrace
    Errors        []ProbeError
```

### 4.2 0-RTT Test Implementation

```
internal/probe/zerortt_test.go

ZeroRTTTest:
    Phase 1 — Full Handshake:
        1. Connect to target with session ticket callback
        2. Make a simple GET request
        3. Record full handshake timing
        4. Save session ticket
        5. Close connection
    
    Phase 2 — 0-RTT Resumption:
        1. Connect using saved session ticket
        2. Send early data (GET request in 0-RTT)
        3. Record timing:
           - Time to send first 0-RTT byte
           - Time to receive response
           - Whether server accepted 0-RTT
        4. Compare with Phase 1 timing
    
    Result:
        full_handshake_time:   duration
        zero_rtt_time:         duration
        time_saved:            duration
        zero_rtt_accepted:     bool
        early_data_bytes:      int
        ticket_size:           int
```

### 4.3 Migration Test Implementation

```
internal/probe/migration_test.go

MigrationTest:
    1. Establish QUIC connection to target
    2. Send initial request, verify response
    3. Create new UDP socket (different local port)
    4. Rebind connection to new socket
    5. Send PATH_CHALLENGE on new path
    6. Wait for PATH_RESPONSE
    7. Send request on new path
    8. Verify response correctness
    9. Record:
       - Migration latency
       - Path validation time
       - Whether server supports migration
       - Any errors
```

---

## 5. BENCH MODE IMPLEMENTATION

### 5.1 Benchmark Runner

```
internal/bench/runner.go

BenchRunner struct:
    config       BenchConfig
    target       string
    protocols    []Protocol          // h1, h2, h3
    results      map[Protocol]*ProtocolBenchResult

Run() → (*BenchReport, error):
    1. Warmup phase (configurable duration)
    2. For each protocol:
       a. Spawn N worker goroutines (concurrency)
       b. Each worker:
          - Create connection
          - Send requests in loop
          - Record per-request timing
       c. Run for specified duration
       d. Collect results
    3. Calculate percentiles (p50, p95, p99)
    4. Generate comparison report

ProtocolBenchResult struct:
    Protocol       string
    TotalRequests  uint64
    TotalBytes     uint64
    Duration       time.Duration
    Errors         uint64
    
    TTFB           PercentileSet
    Throughput     PercentileSet
    ReqPerSec      float64
    ConnectTime    PercentileSet
    
    Latencies      []time.Duration   // Raw samples for histograms
```

### 5.2 Network Condition Simulator

```
internal/bench/simulator.go

NetworkSimulator struct:
    latency       time.Duration     // Added per-packet delay
    jitter        time.Duration     // Random +/- jitter
    lossRate      float64           // 0.0-1.0 packet loss probability
    bandwidth     int64             // Bytes per second (0 = unlimited)
    reorderRate   float64           // Packet reordering probability
    
    // Token bucket for bandwidth
    tokenBucket   int64
    lastRefill    time.Time

WrapConn(conn net.PacketConn) → net.PacketConn:
    Returns a wrapped connection that applies network conditions
    
SimulatedConn:
    ReadFrom(p []byte) → (n int, addr net.Addr, err error):
        // Apply loss: randomly drop packets
        // Apply reordering: delay random packets
        // Apply latency + jitter: sleep before returning
    
    WriteTo(p []byte, addr net.Addr) → (n int, err error):
        // Apply bandwidth throttling via token bucket
        // Apply loss on write side
```

---

## 6. ANALYTICS ENGINE IMPLEMENTATION

### 6.1 Connection Tracer

```
internal/analytics/collector.go

ConnectionTracer struct:
    trace        *ConnectionTrace
    timeline     *Timeline
    mu           sync.Mutex
    startTime    time.Time

// Hook methods called by QUIC engine
OnPacketSent(pn uint64, size int, frames []Frame)
OnPacketReceived(pn uint64, size int, frames []Frame)
OnPacketLost(pn uint64)
OnRTTSample(rtt time.Duration)
OnCWNDUpdate(cwnd int)
OnStreamOpened(id uint64, bidi bool)
OnStreamClosed(id uint64)
OnHandshakeComplete(params TransportParameters)
OnZeroRTTAttempt(accepted bool, bytes int)
OnMigration(from, to *net.UDPAddr, validated bool)
OnConnectionClose(err error)

Finalize() → *ConnectionTrace
```

### 6.2 qlog Writer

```
internal/analytics/qlog.go

// draft-ietf-quic-qlog-main-schema compatible output

QLogWriter struct:
    encoder *json.Encoder
    refTime time.Time

Write(event QLogEvent):
    {
        "time": relativeTime,
        "name": "transport:packet_sent",
        "data": { ... }
    }

Events follow:
    transport:packet_sent
    transport:packet_received
    transport:packet_lost
    transport:parameters_set
    recovery:metrics_updated
    connectivity:connection_started
    connectivity:connection_closed
    http:frame_created
    http:frame_parsed
    qpack:state_updated
```

---

## 7. WEB DASHBOARD IMPLEMENTATION

### 7.1 Dashboard Server

```
internal/dashboard/server.go

DashboardServer struct:
    httpServer   *http.Server
    mux          *http.ServeMux
    assets       embed.FS          // Embedded static files
    sseHub       *SSEHub
    wsHub        *WSHub
    apiHandler   *APIHandler

Routes:
    // Static assets
    GET /                      → index.html
    GET /dashboard/*           → SPA routes (serve index.html)
    GET /assets/*              → CSS/JS files
    
    // API (see SPECIFICATION §9)
    /api/v1/*                  → apiHandler
    
    // Real-time
    GET /api/v1/events         → SSE endpoint
    GET /api/v1/ws             → WebSocket endpoint
```

### 7.2 SSE Hub

```
internal/dashboard/sse.go

SSEHub struct:
    clients    map[string]chan SSEEvent  // clientID → channel
    mu         sync.RWMutex

SSEEvent struct:
    Type  string    // "probe_progress", "bench_update", "connection_event"
    Data  string    // JSON payload
    ID    string    // Event ID for resumption

Register(clientID string) → <-chan SSEEvent
Unregister(clientID string)
Broadcast(event SSEEvent)
```

### 7.3 Canvas Charts (Vanilla JS)

```
web/js/charts.js

// Self-contained chart library, zero dependencies

class TritonChart:
    constructor(canvas, options)
    
    // Chart types
    static LineChart(canvas, data, options)     → RTT, CWND over time
    static BarChart(canvas, data, options)      → Protocol comparison
    static Histogram(canvas, data, options)     → Latency distribution
    static RadarChart(canvas, data, options)    → Multi-metric comparison
    static WaterfallChart(canvas, data, options) → Connection timeline
    static GaugeChart(canvas, data, options)    → Live metrics
    static StreamDiagram(canvas, data, options) → Concurrent streams
    
    // Features
    - Dark/light theme via CSS custom properties
    - Responsive (ResizeObserver)
    - Animation (requestAnimationFrame)
    - Tooltip on hover
    - Legend
    - Axis labels with auto-scaling
    - Grid lines
    - Data point highlighting

web/js/timeline.js

class ConnectionTimeline:
    // Waterfall-style visualization of connection phases
    // DNS → UDP → Initial → Handshake → 1-RTT → Streams
    // Color-coded phases
    // Hover for details
    // Zoom support

web/js/inspector.js

class PacketInspector:
    // Table view of packets with expandable frame details
    // Filtering by packet type, stream ID
    // Packet loss highlighting
    // Retransmission linking
```

---

## 8. CLI IMPLEMENTATION

### 8.1 Command Router (No Cobra)

```
internal/cli/root.go

// Custom CLI parser — zero dependencies

type Command struct {
    Name        string
    Description string
    Flags       []Flag
    SubCommands []*Command
    Run         func(args []string, flags FlagSet) error
}

type Flag struct {
    Name     string
    Short    string
    Type     FlagType    // String, Int, Bool, Duration, Float
    Default  interface{}
    Usage    string
    Required bool
}

Parse(args []string) → (*Command, FlagSet, error):
    1. Match command name
    2. Match subcommand if present
    3. Parse flags (--flag=value, --flag value, -f value, -f=value)
    4. Validate required flags
    5. Apply defaults

Help():
    // Auto-generated help text with usage, commands, flags
```

### 8.2 Output Formatters

```
internal/cli/output.go

type OutputFormatter interface {
    Format(data interface{}) (string, error)
}

TableFormatter:    ASCII table with aligned columns
JSONFormatter:     Pretty-printed JSON
CSVFormatter:      RFC 4180 compliant
YAMLFormatter:     Via gopkg.in/yaml.v3
MarkdownFormatter: GFM table format

// Color support (auto-detect terminal)
type Color int
const (
    Red Color = iota
    Green
    Yellow
    Blue
    Cyan
    Bold
    Reset
)

Colorize(text string, color Color) → string:
    if !isTerminal { return text }
    return fmt.Sprintf("\033[%dm%s\033[0m", ansiCode, text)
```

---

## 9. CONFIGURATION IMPLEMENTATION

```
internal/config/config.go

Load(path string) → (*Config, error):
    1. Set defaults
    2. If config file exists:
       a. Read file
       b. Parse YAML via gopkg.in/yaml.v3
       c. Merge with defaults
    3. Override with environment variables (TRITON_ prefix)
    4. Override with CLI flags
    5. Validate
    6. Return final config

Validate() → error:
    - Port ranges valid
    - File paths accessible
    - TLS cert/key pair valid
    - Duration values positive
    - Congestion algorithm recognized
```

---

## 10. STORAGE IMPLEMENTATION

```
internal/storage/filesystem.go

FileStore struct:
    baseDir    string
    maxResults int
    retention  time.Duration

Save(category string, id string, data interface{}) → error:
    1. Marshal to JSON
    2. Gzip compress
    3. Write to baseDir/category/id.json.gz
    4. Run cleanup if needed

Load(category string, id string, target interface{}) → error:
    1. Read baseDir/category/id.json.gz
    2. Gzip decompress
    3. Unmarshal JSON

List(category string) → ([]StoredItem, error):
    // List all items in category, sorted by time

Cleanup():
    // Remove items exceeding maxResults or older than retention
```

---

## 11. BUILD SYSTEM

```
Makefile

VERSION ?= $(shell git describe --tags --always)
LDFLAGS = -s -w -X main.version=$(VERSION) -X main.buildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: build build-all test lint clean

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/triton ./cmd/triton

build-all:
	GOOS=linux   GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/triton-linux-amd64 ./cmd/triton
	GOOS=linux   GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/triton-linux-arm64 ./cmd/triton
	GOOS=darwin  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/triton-darwin-amd64 ./cmd/triton
	GOOS=darwin  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/triton-darwin-arm64 ./cmd/triton
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/triton-windows-amd64.exe ./cmd/triton

test:
	go test -race -cover ./...

test-fuzz:
	go test -fuzz=FuzzPacketParser ./internal/quic/packet/
	go test -fuzz=FuzzFrameParser ./internal/quic/frame/
	go test -fuzz=FuzzQPACK ./internal/h3/qpack/

lint:
	go vet ./...

clean:
	rm -rf bin/ triton-data/

dev:
	go run ./cmd/triton server --dashboard --log-level debug

docker:
	docker build -t tritonprobe/triton:$(VERSION) .
```

---

## 12. CRYPTO KEY DERIVATION DETAILS

```
Initial Keys (hardcoded salt for QUIC v1):

    salt_v1 = []byte{
        0x38, 0x76, 0x2c, 0xf7, 0xf5, 0x59, 0x34, 0xb3,
        0x4d, 0x17, 0x9a, 0xe6, 0xa4, 0xc8, 0x0c, 0xad,
        0xcc, 0xbb, 0x7f, 0x0a,
    }
    
    initial_secret = HKDF-Extract(salt_v1, destination_connection_id)
    
    client_initial_secret = hkdfExpandLabel(initial_secret, "client in", nil, 32)
    server_initial_secret = hkdfExpandLabel(initial_secret, "server in", nil, 32)
    
    // From each directional secret:
    key = hkdfExpandLabel(secret, "quic key", nil, keyLen)  // 16 for AES-128
    iv  = hkdfExpandLabel(secret, "quic iv", nil, 12)
    hp  = hkdfExpandLabel(secret, "quic hp", nil, keyLen)

hkdfExpandLabel implementation:
    func hkdfExpandLabel(secret []byte, label string, context []byte, length int) []byte {
        fullLabel := "tls13 " + label
        hkdfLabel := make([]byte, 2+1+len(fullLabel)+1+len(context))
        hkdfLabel[0] = byte(length >> 8)
        hkdfLabel[1] = byte(length)
        hkdfLabel[2] = byte(len(fullLabel))
        copy(hkdfLabel[3:], fullLabel)
        hkdfLabel[3+len(fullLabel)] = byte(len(context))
        copy(hkdfLabel[4+len(fullLabel):], context)
        
        r := hkdf.Expand(sha256.New, secret, hkdfLabel)  // golang.org/x/crypto/hkdf
        out := make([]byte, length)
        r.Read(out)
        return out
    }

Packet Protection:
    AES-128-GCM:
        nonce = iv XOR packet_number (padded to 12 bytes)
        aad = header bytes (up to and including packet number)
        ciphertext = AES-GCM-Encrypt(key, nonce, aad, plaintext)
    
    ChaCha20-Poly1305:
        nonce = iv XOR packet_number (padded to 12 bytes)
        ciphertext = ChaCha20Poly1305-Encrypt(key, nonce, aad, plaintext)
```

---

*Three Prongs. One Binary. Every Packet.*
