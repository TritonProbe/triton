package probe

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/h3"
)

func estimateQPACKAnalysis(headers http.Header, status int) QPACKAnalysis {
	blockHeaders := map[string]string{":status": fmt.Sprintf("%d", status)}
	rawBytes := len(":status") + len(blockHeaders[":status"]) + 4
	headerCount := 1
	for key, values := range headers {
		lowerKey := strings.ToLower(key)
		for _, value := range values {
			blockHeaders[lowerKey] = value
			rawBytes += len(lowerKey) + len(value) + 4
			headerCount++
		}
	}
	encodedBytes := len(h3.EncodeHeaders(blockHeaders))
	ratio := 1.0
	if rawBytes > 0 {
		ratio = float64(encodedBytes) / float64(rawBytes)
	}
	return QPACKAnalysis{
		Supported:         true,
		Mode:              "header-block-estimate",
		HeaderCount:       headerCount,
		RawBytes:          rawBytes,
		EstimatedBlock:    encodedBytes,
		EstimatedRatio:    ratio,
		CompressionSaving: rawBytes - encodedBytes,
		Note:              "approximates header block size from serialized H3 headers; it does not inspect real QPACK dynamic table behavior",
	}
}

func estimateVersionAnalysis(proto string, tlsMeta any) VersionAnalysis {
	return VersionAnalysis{Supported: true, Mode: "protocol-observation", ObservedProto: proto, ALPN: tlsALPN(tlsMeta), QUICVersion: "not_exposed", NegotiationSeen: false, Note: "approximates QUIC version state from observed HTTP/3 protocol and ALPN; packet-level version negotiation is not exposed here"}
}

func estimateRetryAnalysis(proto string, tlsMeta any, connectMS, tlsMS int64) RetryAnalysis {
	return RetryAnalysis{Supported: true, Mode: "handshake-observation", ObservedProto: proto, ALPN: tlsALPN(tlsMeta), RetryObserved: false, ConnectMS: connectMS, TLSMS: tlsMS, Visibility: "client-layer-limited", Note: "approximates Retry behavior from successful handshake observation; actual Retry packets are not exposed at this layer"}
}

func estimateECNAnalysis(headers http.Header, proto string, tlsMeta any) ECNAnalysis {
	observedSignal := false
	for key := range headers {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "ecn") || strings.Contains(lower, "ce") {
			observedSignal = true
			break
		}
	}
	return ECNAnalysis{Supported: true, Mode: "metadata-observation", ObservedProto: proto, ALPN: tlsALPN(tlsMeta), ECNVisible: observedSignal, PacketMarks: "not_exposed", Note: "approximates ECN visibility from observable protocol metadata and headers; packet-level ECN markings are not exposed here"}
}

func estimateSpinBitAnalysis(cfg config.ProbeConfig, request func() (int, int64, error)) SpinBitAnalysis {
	latency := sampleLatencyProfile(cfg, request)
	stability := "steady"
	if latency.P95-latency.P50 > latency.P50 {
		stability = "variable"
	}
	return SpinBitAnalysis{Supported: true, Mode: "rtt-sampling-estimate", RTTEstimateMS: latency.P50, P95MS: latency.P95, Stability: stability, SpinObserved: false, Note: "approximates spin-bit style RTT visibility from sampled request timings; actual packet-level spin-bit observation is not exposed here"}
}

func estimateLossAnalysis(cfg config.ProbeConfig, request func() (int, int64, error)) LossAnalysis {
	latency := sampleLatencyProfile(cfg, request)
	streams := runConcurrentSamples(minInt(cfg.DefaultStreams, 4), request)
	signal := "low"
	if latency.Errors > 0 || streams.Errors > 0 {
		signal = "elevated"
	}
	if latency.Errors >= 2 || streams.SuccessRate < 0.75 {
		signal = "high"
	}
	return LossAnalysis{Supported: true, Mode: "request-error-signal", Signal: signal, LatencyErrors: latency.Errors, LatencySamples: latency.Samples, StreamAttempts: streams.Attempts, StreamErrors: streams.Errors, SuccessRate: streams.SuccessRate, ErrorCategories: copyIntMap(streams.ErrorCategories), TimeoutIndicators: streams.ErrorCategories["timeout"], Note: "approximates packet-loss pressure from repeated request failures and timeout/error categories; it does not inspect packet-level recovery"}
}

func estimateCongestionAnalysis(cfg config.ProbeConfig, request func() (int, int64, error)) CongestionAnalysis {
	latency := sampleLatencyProfile(cfg, request)
	streams := runConcurrentSamples(minInt(cfg.DefaultStreams, 4), request)
	jitter := latency.P95 - latency.P50
	spreadRatio := 0.0
	if latency.P50 > 0 {
		spreadRatio = jitter / latency.P50
	}
	signal := "low"
	if spreadRatio > 0.5 || streams.P95 > streams.AverageMS*1.5 {
		signal = "moderate"
	}
	if spreadRatio > 1.0 || streams.P95 > streams.AverageMS*2 {
		signal = "high"
	}
	return CongestionAnalysis{Supported: true, Mode: "latency-spread-estimate", Signal: signal, P50MS: latency.P50, P95MS: latency.P95, JitterMS: jitter, SpreadRatio: spreadRatio, StreamAverageMS: streams.AverageMS, StreamP95MS: streams.P95, ConcurrentAttempts: streams.Attempts, SuccessRate: streams.SuccessRate, Note: "approximates congestion pressure from latency spread and concurrent request slowdown; it does not read congestion-window telemetry"}
}

func tlsMetadata(state *tls.ConnectionState) TLSMetadata {
	meta := TLSMetadata{Version: tlsVersion(state.Version), Cipher: tls.CipherSuiteName(state.CipherSuite), ALPN: state.NegotiatedProtocol, ServerName: state.ServerName, PeerCerts: len(state.PeerCertificates), Resumed: state.DidResume, HandshakeState: "complete"}
	if len(state.VerifiedChains) > 0 {
		meta.VerifiedChains = len(state.VerifiedChains)
	}
	if len(state.PeerCertificates) > 0 {
		leaf := certificateSummary(state.PeerCertificates[0])
		meta.LeafCert = &leaf
	}
	return meta
}

func certificateSummary(cert *x509.Certificate) CertificateSummary {
	if cert == nil {
		return CertificateSummary{}
	}
	return CertificateSummary{Subject: cert.Subject.String(), Issuer: cert.Issuer.String(), DNSNames: append([]string(nil), cert.DNSNames...), NotBefore: cert.NotBefore.UTC().Format(time.RFC3339), NotAfter: cert.NotAfter.UTC().Format(time.RFC3339), IsCA: cert.IsCA, Serial: cert.SerialNumber.String(), Fingerprint: fmt.Sprintf("%X", cert.Signature[:min(len(cert.Signature), 8)])}
}

func analyzeHeaders(headers http.Header, plan testPlan) map[string]any {
	analysis := map[string]any{}
	altSvc := headers.Values("Alt-Svc")
	if len(altSvc) > 0 && plan.shouldRun("alt-svc") {
		analysis["alt_svc"] = AltSvcAnalysis{Present: true, Values: append([]string(nil), altSvc...)}
	}
	return analysis
}

func tlsALPN(meta any) string {
	switch typed := meta.(type) {
	case TLSMetadata:
		return typed.ALPN
	case map[string]any:
		alpn, _ := typed["alpn"].(string)
		return alpn
	default:
		return ""
	}
}

func enrichResponseAnalysis(result *Result, bodyBytes int64) {
	if result == nil {
		return
	}
	if result.Analysis == nil {
		result.Analysis = map[string]any{}
	}
	throughput := 0.0
	if result.Duration > 0 {
		throughput = float64(bodyBytes) / result.Duration.Seconds()
	}
	result.Analysis["response"] = ResponseAnalysis{BodyBytes: bodyBytes, ThroughputBytesSec: throughput, ThroughputBitsSec: throughput * 8, StatusClass: result.Status / 100}
}

func enrichLatencyAnalysis(result *Result, cfg config.ProbeConfig, request func() (int, int64, error)) {
	if result == nil || request == nil {
		return
	}
	latency := sampleLatencyProfile(cfg, request)
	if len(latency.SamplesMS) == 0 {
		return
	}
	if result.Analysis == nil {
		result.Analysis = map[string]any{}
	}
	result.Analysis["latency"] = LatencyAnalysis{Samples: latency.Samples, AverageMS: latency.AverageMS, P50: latency.P50, P95: latency.P95, P99: latency.P99, Errors: latency.Errors, SamplesMS: append([]float64(nil), latency.SamplesMS...)}
}

func enrichStreamAnalysis(result *Result, cfg config.ProbeConfig, request func() (int, int64, error)) {
	if result == nil || request == nil || cfg.DefaultStreams <= 1 {
		return
	}
	streams := runConcurrentSamples(cfg.DefaultStreams, request)
	if streams.Attempts == 0 {
		return
	}
	if result.Analysis == nil {
		result.Analysis = map[string]any{}
	}
	result.Analysis["streams"] = StreamAnalysis{Attempted: streams.Attempts, Successful: streams.Successes, Errors: streams.Errors, SuccessRate: streams.SuccessRate, AverageLatency: streams.AverageMS, P95Latency: streams.P95, ThroughputBytes: streams.TotalBytes, StatusClasses: copyIntMap(streams.StatusClasses), ErrorCategories: copyIntMap(streams.ErrorCategories)}
}

func copyIntMap(input map[string]int) map[string]int {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]int, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func sampleLatencyProfile(cfg config.ProbeConfig, request func() (int, int64, error)) latencyProfile {
	samples := 5
	if cfg.DefaultStreams > 0 && cfg.DefaultStreams < samples {
		samples = cfg.DefaultStreams
	}
	values := make([]float64, 0, samples)
	errorsCount := 0
	for i := 0; i < samples; i++ {
		start := time.Now()
		_, _, err := request()
		elapsed := time.Since(start)
		if err != nil {
			errorsCount++
			continue
		}
		values = append(values, float64(elapsed)/float64(time.Millisecond))
	}
	avg := 0.0
	for _, value := range values {
		avg += value
	}
	if len(values) > 0 {
		avg /= float64(len(values))
	}
	sorted := append([]float64(nil), values...)
	sortFloat64s(sorted)
	return latencyProfile{Samples: len(values), Errors: errorsCount, AverageMS: avg, P50: percentile(sorted, 0.50), P95: percentile(sorted, 0.95), P99: percentile(sorted, 0.99), SamplesMS: values}
}

func runConcurrentSamples(concurrency int, request func() (int, int64, error)) concurrentSummary {
	type sample struct{ status int; bytes int64; latency float64; err error }
	results := make(chan sample, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			status, bytes, err := request()
			results <- sample{status: status, bytes: bytes, latency: float64(time.Since(start)) / float64(time.Millisecond), err: err}
		}()
	}
	wg.Wait()
	close(results)
	latencies := make([]float64, 0, concurrency)
	summary := concurrentSummary{Attempts: concurrency, StatusClasses: map[string]int{}, ErrorCategories: map[string]int{}}
	for result := range results {
		if result.err != nil {
			summary.Errors++
			summary.ErrorCategories[categorizeProbeError(result.err)]++
			continue
		}
		summary.Successes++
		summary.TotalBytes += result.bytes
		latencies = append(latencies, result.latency)
		summary.StatusClasses[fmt.Sprintf("%dxx", result.status/100)]++
	}
	for _, latency := range latencies {
		summary.AverageMS += latency
	}
	if len(latencies) > 0 {
		summary.AverageMS /= float64(len(latencies))
	}
	sorted := append([]float64(nil), latencies...)
	sortFloat64s(sorted)
	summary.P95 = percentile(sorted, 0.95)
	if summary.Attempts > 0 {
		summary.SuccessRate = float64(summary.Successes) / float64(summary.Attempts)
	}
	return summary
}

func categorizeProbeError(err error) string {
	if err == nil {
		return ""
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "certificate"):
		return "tls_verification"
	case strings.Contains(lower, "timeout"):
		return "timeout"
	case strings.Contains(lower, "connection refused"):
		return "connection_refused"
	default:
		return "request_failed"
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	return sorted[int(float64(len(sorted)-1)*p)]
}

func sortFloat64s(values []float64) { sort.Float64s(values) }

func minInt(a, b int) int {
	if a <= 0 {
		return b
	}
	if a < b {
		return a
	}
	return b
}

func tlsVersion(v uint16) string {
	switch v {
	case tls.VersionTLS13:
		return "TLS1.3"
	case tls.VersionTLS12:
		return "TLS1.2"
	default:
		return fmt.Sprintf("0x%x", v)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
