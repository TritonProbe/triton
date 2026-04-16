package probe

import (
	"net/http"
	"time"
)

type Result struct {
	ID         string           `json:"id" yaml:"id"`
	Target     string           `json:"target" yaml:"target"`
	Timestamp  time.Time        `json:"timestamp" yaml:"timestamp"`
	Duration   time.Duration    `json:"duration" yaml:"duration"`
	Status     int              `json:"status" yaml:"status"`
	Proto      string           `json:"proto" yaml:"proto"`
	TraceFiles []string         `json:"trace_files,omitempty" yaml:"trace_files,omitempty"`
	Timings    map[string]int64 `json:"timings_ms" yaml:"timings_ms"`
	TLS        any              `json:"tls" yaml:"tls"`
	Headers    http.Header      `json:"headers" yaml:"headers"`
	Analysis   map[string]any   `json:"analysis,omitempty" yaml:"analysis,omitempty"`
}

type SupportEntry struct {
	Requested bool   `json:"requested" yaml:"requested"`
	Coverage  string `json:"coverage,omitempty" yaml:"coverage,omitempty"`
	State     string `json:"state,omitempty" yaml:"state,omitempty"`
	Summary   string `json:"summary,omitempty" yaml:"summary,omitempty"`
	Mode      string `json:"mode,omitempty" yaml:"mode,omitempty"`
}

type SupportSummary struct {
	RequestedTests int     `json:"requested_tests" yaml:"requested_tests"`
	Available      int     `json:"available" yaml:"available"`
	NotRun         int     `json:"not_run" yaml:"not_run"`
	Unavailable    int     `json:"unavailable" yaml:"unavailable"`
	Full           int     `json:"full" yaml:"full"`
	Partial        int     `json:"partial" yaml:"partial"`
	CoverageRatio  float64 `json:"coverage_ratio" yaml:"coverage_ratio"`
}

type FidelitySummary struct {
	Full        []string `json:"full,omitempty" yaml:"full,omitempty"`
	Partial     []string `json:"partial,omitempty" yaml:"partial,omitempty"`
	Observed    []string `json:"observed,omitempty" yaml:"observed,omitempty"`
	Unavailable []string `json:"unavailable,omitempty" yaml:"unavailable,omitempty"`
	PacketLevel bool     `json:"packet_level" yaml:"packet_level"`
	Notice      string   `json:"notice,omitempty" yaml:"notice,omitempty"`
}

type SkippedTest struct {
	Name   string `json:"name" yaml:"name"`
	Reason string `json:"reason" yaml:"reason"`
}

type TestPlan struct {
	Requested []string      `json:"requested" yaml:"requested"`
	Executed  []string      `json:"executed" yaml:"executed"`
	Skipped   []SkippedTest `json:"skipped,omitempty" yaml:"skipped,omitempty"`
}

type ResponseAnalysis struct {
	BodyBytes          int64   `json:"body_bytes" yaml:"body_bytes"`
	ThroughputBytesSec float64 `json:"throughput_bytes_sec" yaml:"throughput_bytes_sec"`
	ThroughputBitsSec  float64 `json:"throughput_bits_sec" yaml:"throughput_bits_sec"`
	StatusClass        int     `json:"status_class" yaml:"status_class"`
}

type LatencyAnalysis struct {
	Samples   int       `json:"samples" yaml:"samples"`
	AverageMS float64   `json:"avg_ms" yaml:"avg_ms"`
	P50       float64   `json:"p50" yaml:"p50"`
	P95       float64   `json:"p95" yaml:"p95"`
	P99       float64   `json:"p99" yaml:"p99"`
	Errors    int       `json:"errors" yaml:"errors"`
	SamplesMS []float64 `json:"samples_ms,omitempty" yaml:"samples_ms,omitempty"`
}

type StreamAnalysis struct {
	Attempted       int            `json:"attempted" yaml:"attempted"`
	Successful      int            `json:"successful" yaml:"successful"`
	Errors          int            `json:"errors" yaml:"errors"`
	SuccessRate     float64        `json:"success_rate" yaml:"success_rate"`
	AverageLatency  float64        `json:"avg_latency_ms" yaml:"avg_latency_ms"`
	P95Latency      float64        `json:"p95_latency_ms" yaml:"p95_latency_ms"`
	ThroughputBytes int64          `json:"throughput_bytes" yaml:"throughput_bytes"`
	StatusClasses   map[string]int `json:"status_classes,omitempty" yaml:"status_classes,omitempty"`
	ErrorCategories map[string]int `json:"error_categories,omitempty" yaml:"error_categories,omitempty"`
}

type CertificateSummary struct {
	Subject     string   `json:"subject" yaml:"subject"`
	Issuer      string   `json:"issuer" yaml:"issuer"`
	DNSNames    []string `json:"dns_names,omitempty" yaml:"dns_names,omitempty"`
	NotBefore   string   `json:"not_before" yaml:"not_before"`
	NotAfter    string   `json:"not_after" yaml:"not_after"`
	IsCA        bool     `json:"is_ca" yaml:"is_ca"`
	Serial      string   `json:"serial" yaml:"serial"`
	Fingerprint string   `json:"fingerprint" yaml:"fingerprint"`
}

type TLSMetadata struct {
	Mode           string              `json:"mode,omitempty" yaml:"mode,omitempty"`
	Version        string              `json:"version,omitempty" yaml:"version,omitempty"`
	Cipher         string              `json:"cipher,omitempty" yaml:"cipher,omitempty"`
	ALPN           string              `json:"alpn,omitempty" yaml:"alpn,omitempty"`
	ServerName     string              `json:"server_name,omitempty" yaml:"server_name,omitempty"`
	PeerCerts      int                 `json:"peer_certs,omitempty" yaml:"peer_certs,omitempty"`
	Resumed        bool                `json:"resumed" yaml:"resumed"`
	HandshakeState string              `json:"handshake_state,omitempty" yaml:"handshake_state,omitempty"`
	VerifiedChains int                 `json:"verified_chains,omitempty" yaml:"verified_chains,omitempty"`
	LeafCert       *CertificateSummary `json:"leaf_cert,omitempty" yaml:"leaf_cert,omitempty"`
}

type AltSvcAnalysis struct {
	Present bool     `json:"present" yaml:"present"`
	Values  []string `json:"values,omitempty" yaml:"values,omitempty"`
}

type QPACKAnalysis struct {
	Supported         bool    `json:"supported" yaml:"supported"`
	Mode              string  `json:"mode" yaml:"mode"`
	HeaderCount       int     `json:"header_count" yaml:"header_count"`
	RawBytes          int     `json:"raw_bytes" yaml:"raw_bytes"`
	EstimatedBlock    int     `json:"estimated_block" yaml:"estimated_block"`
	EstimatedRatio    float64 `json:"estimated_ratio" yaml:"estimated_ratio"`
	CompressionSaving int     `json:"compression_saving" yaml:"compression_saving"`
	Note              string  `json:"note,omitempty" yaml:"note,omitempty"`
}

type VersionAnalysis struct {
	Supported       bool   `json:"supported" yaml:"supported"`
	Mode            string `json:"mode" yaml:"mode"`
	ObservedProto   string `json:"observed_proto" yaml:"observed_proto"`
	ALPN            string `json:"alpn,omitempty" yaml:"alpn,omitempty"`
	QUICVersion     string `json:"quic_version" yaml:"quic_version"`
	NegotiationSeen bool   `json:"negotiation_seen" yaml:"negotiation_seen"`
	Note            string `json:"note,omitempty" yaml:"note,omitempty"`
}

type RetryAnalysis struct {
	Supported     bool   `json:"supported" yaml:"supported"`
	Mode          string `json:"mode" yaml:"mode"`
	ObservedProto string `json:"observed_proto" yaml:"observed_proto"`
	ALPN          string `json:"alpn,omitempty" yaml:"alpn,omitempty"`
	RetryObserved bool   `json:"retry_observed" yaml:"retry_observed"`
	ConnectMS     int64  `json:"connect_ms" yaml:"connect_ms"`
	TLSMS         int64  `json:"tls_ms" yaml:"tls_ms"`
	Visibility    string `json:"visibility" yaml:"visibility"`
	Note          string `json:"note,omitempty" yaml:"note,omitempty"`
}

type ECNAnalysis struct {
	Supported     bool   `json:"supported" yaml:"supported"`
	Mode          string `json:"mode" yaml:"mode"`
	ObservedProto string `json:"observed_proto" yaml:"observed_proto"`
	ALPN          string `json:"alpn,omitempty" yaml:"alpn,omitempty"`
	ECNVisible    bool   `json:"ecn_visible" yaml:"ecn_visible"`
	PacketMarks   string `json:"packet_marks" yaml:"packet_marks"`
	Note          string `json:"note,omitempty" yaml:"note,omitempty"`
}

type SpinBitAnalysis struct {
	Supported     bool    `json:"supported" yaml:"supported"`
	Mode          string  `json:"mode" yaml:"mode"`
	RTTEstimateMS float64 `json:"rtt_estimate_ms" yaml:"rtt_estimate_ms"`
	P95MS         float64 `json:"p95_ms" yaml:"p95_ms"`
	Stability     string  `json:"stability" yaml:"stability"`
	SpinObserved  bool    `json:"spin_observed" yaml:"spin_observed"`
	Note          string  `json:"note,omitempty" yaml:"note,omitempty"`
}

type LossAnalysis struct {
	Supported         bool           `json:"supported" yaml:"supported"`
	Mode              string         `json:"mode" yaml:"mode"`
	Signal            string         `json:"signal" yaml:"signal"`
	LatencyErrors     int            `json:"latency_errors" yaml:"latency_errors"`
	LatencySamples    int            `json:"latency_samples" yaml:"latency_samples"`
	StreamAttempts    int            `json:"stream_attempts" yaml:"stream_attempts"`
	StreamErrors      int            `json:"stream_errors" yaml:"stream_errors"`
	SuccessRate       float64        `json:"success_rate" yaml:"success_rate"`
	ErrorCategories   map[string]int `json:"error_categories,omitempty" yaml:"error_categories,omitempty"`
	TimeoutIndicators int            `json:"timeout_indicators" yaml:"timeout_indicators"`
	Note              string         `json:"note,omitempty" yaml:"note,omitempty"`
}

type CongestionAnalysis struct {
	Supported          bool    `json:"supported" yaml:"supported"`
	Mode               string  `json:"mode" yaml:"mode"`
	Signal             string  `json:"signal" yaml:"signal"`
	P50MS              float64 `json:"p50_ms" yaml:"p50_ms"`
	P95MS              float64 `json:"p95_ms" yaml:"p95_ms"`
	JitterMS           float64 `json:"jitter_ms" yaml:"jitter_ms"`
	SpreadRatio        float64 `json:"spread_ratio" yaml:"spread_ratio"`
	StreamAverageMS    float64 `json:"stream_avg_ms" yaml:"stream_avg_ms"`
	StreamP95MS        float64 `json:"stream_p95_ms" yaml:"stream_p95_ms"`
	ConcurrentAttempts int     `json:"concurrent_attempts" yaml:"concurrent_attempts"`
	SuccessRate        float64 `json:"success_rate" yaml:"success_rate"`
	Note               string  `json:"note,omitempty" yaml:"note,omitempty"`
}

type ZeroRTTAnalysis struct {
	Supported      bool    `json:"supported" yaml:"supported"`
	Mode           string  `json:"mode" yaml:"mode"`
	InitialMS      float64 `json:"initial_ms,omitempty" yaml:"initial_ms,omitempty"`
	ResumedMS      float64 `json:"resumed_ms,omitempty" yaml:"resumed_ms,omitempty"`
	InitialResumed bool    `json:"initial_resumed,omitempty" yaml:"initial_resumed,omitempty"`
	Resumed        bool    `json:"resumed,omitempty" yaml:"resumed,omitempty"`
	TimeSavedMS    float64 `json:"time_saved_ms,omitempty" yaml:"time_saved_ms,omitempty"`
	Requested0RTT  bool    `json:"requested_0rtt" yaml:"requested_0rtt"`
	Note           string  `json:"note,omitempty" yaml:"note,omitempty"`
	Error          string  `json:"error,omitempty" yaml:"error,omitempty"`
}

type MigrationAnalysis struct {
	Supported      bool    `json:"supported" yaml:"supported"`
	Mode           string  `json:"mode" yaml:"mode"`
	Target         string  `json:"target,omitempty" yaml:"target,omitempty"`
	StatusClass    int     `json:"status_class,omitempty" yaml:"status_class,omitempty"`
	BodyBytes      int     `json:"body_bytes,omitempty" yaml:"body_bytes,omitempty"`
	DurationMS     float64 `json:"duration_ms,omitempty" yaml:"duration_ms,omitempty"`
	RequestedCheck bool    `json:"requested_check" yaml:"requested_check"`
	Note           string  `json:"note,omitempty" yaml:"note,omitempty"`
	Message        string  `json:"message,omitempty" yaml:"message,omitempty"`
	Error          string  `json:"error,omitempty" yaml:"error,omitempty"`
}

type testPlan struct {
	requested []string
	executed  []string
	skipped   []SkippedTest
}

type testDefinition struct {
	Coverage string
	Summary  string
	Reason   string
}

type latencyProfile struct {
	Samples   int
	Errors    int
	AverageMS float64
	P50       float64
	P95       float64
	P99       float64
	SamplesMS []float64
}

type concurrentSummary struct {
	Attempts        int
	Successes       int
	Errors          int
	SuccessRate     float64
	AverageMS       float64
	P95             float64
	TotalBytes      int64
	StatusClasses   map[string]int
	ErrorCategories map[string]int
}

var knownProbeTests = []string{"handshake", "tls", "latency", "throughput", "streams", "alt-svc", "0rtt", "migration", "ecn", "retry", "version", "qpack", "congestion", "loss", "spin-bit"}

var probeTestDefinitions = map[string]testDefinition{
	"handshake":  {Coverage: "full", Summary: "basic handshake timing is implemented"},
	"tls":        {Coverage: "full", Summary: "TLS metadata capture is implemented"},
	"latency":    {Coverage: "full", Summary: "sampled latency measurements are implemented"},
	"throughput": {Coverage: "full", Summary: "response size and throughput estimates are implemented"},
	"streams":    {Coverage: "full", Summary: "basic concurrent stream sampling is implemented"},
	"alt-svc":    {Coverage: "full", Summary: "Alt-Svc header observation is implemented"},
	"0rtt":       {Coverage: "partial", Summary: "measured via HTTP/3 session resumption checks rather than true early data", Reason: "true 0-RTT early-data probing is not implemented yet"},
	"migration":  {Coverage: "partial", Summary: "measured via endpoint capability checks rather than live path rebinding", Reason: "live connection migration probing is not implemented yet"},
	"ecn":        {Coverage: "partial", Summary: "approximated from observable response and protocol metadata rather than packet markings", Reason: "ECN observation currently requires an HTTP/3 probe target"},
	"retry":      {Coverage: "partial", Summary: "approximated from handshake outcome because Retry packets are not exposed at this client layer", Reason: "retry observation currently requires an HTTP/3 probe target"},
	"version":    {Coverage: "partial", Summary: "approximated from observed protocol and ALPN rather than packet-level QUIC version negotiation", Reason: "version observation currently requires an HTTP/3 probe target"},
	"qpack":      {Coverage: "partial", Summary: "approximated via header-block size estimates rather than real QPACK dynamic-table inspection", Reason: "QPACK approximation requires an HTTP/3 probe target"},
	"congestion": {Coverage: "partial", Summary: "approximated from latency spread and concurrent request pressure", Reason: "congestion profiling currently relies on latency and concurrency signals"},
	"loss":       {Coverage: "partial", Summary: "approximated from repeated request failures and timeout/error pressure", Reason: "loss probing currently relies on repeated request error signals"},
	"spin-bit":   {Coverage: "partial", Summary: "approximated from sampled RTT spread rather than packet-level spin-bit observation", Reason: "spin-bit observation currently requires an HTTP/3 probe target"},
}
