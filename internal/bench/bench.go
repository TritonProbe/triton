package bench

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tritonprobe/triton/internal/appmux"
	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/h3"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/quic/transport"
	"github.com/tritonprobe/triton/internal/realh3"
)

type Result struct {
	ID          string           `json:"id" yaml:"id"`
	Target      string           `json:"target" yaml:"target"`
	Timestamp   time.Time        `json:"timestamp" yaml:"timestamp"`
	Duration    time.Duration    `json:"duration" yaml:"duration"`
	Protocols   []string         `json:"protocols" yaml:"protocols"`
	Concurrency int              `json:"concurrency" yaml:"concurrency"`
	TraceFiles  []string         `json:"trace_files,omitempty" yaml:"trace_files,omitempty"`
	Stats       map[string]Stats `json:"stats" yaml:"stats"`
	Summary     Summary          `json:"summary,omitempty" yaml:"summary,omitempty"`
}

type Summary struct {
	Protocols         int     `json:"protocols" yaml:"protocols"`
	HealthyProtocols  int     `json:"healthy_protocols" yaml:"healthy_protocols"`
	DegradedProtocols int     `json:"degraded_protocols" yaml:"degraded_protocols"`
	FailedProtocols   int     `json:"failed_protocols" yaml:"failed_protocols"`
	BestProtocol      string  `json:"best_protocol,omitempty" yaml:"best_protocol,omitempty"`
	BestReqPerSec     float64 `json:"best_req_per_sec,omitempty" yaml:"best_req_per_sec,omitempty"`
	RiskiestProtocol  string  `json:"riskiest_protocol,omitempty" yaml:"riskiest_protocol,omitempty"`
	HighestErrorRate  float64 `json:"highest_error_rate,omitempty" yaml:"highest_error_rate,omitempty"`
}

type Stats struct {
	Requests      int64            `json:"requests" yaml:"requests"`
	Errors        int64            `json:"errors" yaml:"errors"`
	AverageMS     float64          `json:"avg_ms" yaml:"avg_ms"`
	RequestsPerS  float64          `json:"req_per_sec" yaml:"req_per_sec"`
	Transferred   int64            `json:"bytes" yaml:"bytes"`
	ErrorRate     float64          `json:"error_rate" yaml:"error_rate"`
	Latency       Percentiles      `json:"latency_ms" yaml:"latency_ms"`
	Phases        PhaseAverages    `json:"phases_ms" yaml:"phases_ms"`
	ErrorSummary  map[string]int64 `json:"error_summary,omitempty" yaml:"error_summary,omitempty"`
	SampledPoints int              `json:"sampled_points" yaml:"sampled_points"`
}

type Percentiles struct {
	P50 float64 `json:"p50" yaml:"p50"`
	P95 float64 `json:"p95" yaml:"p95"`
	P99 float64 `json:"p99" yaml:"p99"`
}

type PhaseAverages struct {
	ConnectMS   float64 `json:"connect" yaml:"connect"`
	TLSMS       float64 `json:"tls" yaml:"tls"`
	FirstByteMS float64 `json:"first_byte" yaml:"first_byte"`
	TransferMS  float64 `json:"transfer" yaml:"transfer"`
}

func Run(target string, cfg config.BenchConfig) (*Result, error) {
	if cfg.Insecure && !cfg.AllowInsecureTLS {
		return nil, fmt.Errorf("bench insecure TLS requires allow_insecure_tls")
	}
	before, err := observability.ListQLOGFiles(cfg.TraceDir)
	if err != nil {
		return nil, err
	}
	stats := make(map[string]Stats, len(cfg.DefaultProtocols))
	for _, protocol := range cfg.DefaultProtocols {
		if cfg.Warmup > 0 {
			if _, err := runProtocol(target, protocol, cfg.Warmup, cfg.DefaultConcurrency, cfg.Insecure, ""); err != nil {
				return nil, fmt.Errorf("warmup %s: %w", protocol, err)
			}
		}
		run, err := runProtocol(target, protocol, cfg.DefaultDuration, cfg.DefaultConcurrency, cfg.Insecure, cfg.TraceDir)
		if err != nil {
			return nil, err
		}
		stats[protocol] = run
	}
	after, err := observability.ListQLOGFiles(cfg.TraceDir)
	if err != nil {
		return nil, err
	}
	return &Result{
		ID:          fmt.Sprintf("bn-%s", time.Now().UTC().Format("20060102-150405")),
		Target:      target,
		Timestamp:   time.Now().UTC(),
		Duration:    cfg.DefaultDuration,
		Protocols:   append([]string(nil), cfg.DefaultProtocols...),
		Concurrency: cfg.DefaultConcurrency,
		TraceFiles:  observability.DiffQLOGFiles(before, after),
		Stats:       stats,
		Summary:     buildSummary(stats),
	}, nil
}

func buildSummary(stats map[string]Stats) Summary {
	summary := Summary{
		Protocols: len(stats),
	}
	if len(stats) == 0 {
		return summary
	}

	var bestProtocol string
	bestReqPerS := -1.0
	var riskiestProtocol string
	highestErrorRate := -1.0

	for protocol, stat := range stats {
		health := protocolHealth(stat)
		switch health {
		case "healthy":
			summary.HealthyProtocols++
		case "degraded":
			summary.DegradedProtocols++
		default:
			summary.FailedProtocols++
		}
		if stat.RequestsPerS > bestReqPerS {
			bestReqPerS = stat.RequestsPerS
			bestProtocol = protocol
		}
		if stat.ErrorRate > highestErrorRate {
			highestErrorRate = stat.ErrorRate
			riskiestProtocol = protocol
		}
	}

	summary.BestProtocol = bestProtocol
	summary.BestReqPerSec = bestReqPerS
	summary.RiskiestProtocol = riskiestProtocol
	summary.HighestErrorRate = highestErrorRate
	return summary
}

func protocolHealth(stat Stats) string {
	switch {
	case stat.Requests == 0 && stat.Errors > 0:
		return "failed"
	case stat.ErrorRate >= 0.10:
		return "degraded"
	default:
		return "healthy"
	}
}

func runProtocol(target, protocol string, duration time.Duration, concurrency int, insecure bool, traceDir string) (Stats, error) {
	if protocol == "h3" {
		if strings.HasPrefix(target, "triton://") {
			return runLoopbackH3Protocol(target, duration, concurrency)
		}
		return runHTTP3Protocol(target, duration, concurrency, insecure, traceDir)
	}

	collector := newBenchmarkCollector()

	transport := &http.Transport{
		ForceAttemptHTTP2: protocol == "h2",
		// #nosec G402 -- insecure TLS is gated by explicit allow_insecure_tls validation for lab use.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure, MinVersion: tls.VersionTLS12},
	}
	if protocol == "h1" {
		transport.ForceAttemptHTTP2 = false
		transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	} else {
		transport.TLSClientConfig.NextProtos = []string{"h2", "http/1.1"}
	}
	client := &http.Client{
		Timeout:   duration + 5*time.Second,
		Transport: transport,
	}
	defer transport.CloseIdleConnections()

	stop := time.Now().Add(duration)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(stop) {
				measurement, err := doHTTPBenchmarkRequest(client, target)
				if err != nil {
					collector.recordError(err)
					continue
				}
				collector.recordSuccess(measurement)
			}
		}()
	}
	wg.Wait()

	stats := collector.finalize(duration)
	if stats.Requests == 0 && stats.Errors > 0 {
		return stats, errors.New("benchmark failed: all requests errored")
	}
	return stats, nil
}

func runHTTP3Protocol(target string, duration time.Duration, concurrency int, insecure bool, traceDir string) (Stats, error) {
	resolvedTarget, err := normalizeHTTP3BenchmarkTarget(target)
	if err != nil {
		return Stats{}, err
	}

	client, transport := realh3.NewClient(duration+5*time.Second, insecure, traceDir)
	defer transport.Close()

	stats := runHTTP3BenchmarkOnce(client, resolvedTarget, duration, concurrency)
	if stats.Requests == 0 && stats.Errors > 0 {
		fallbackTarget, err := discoverAltSvcH3Target(resolvedTarget, insecure)
		if err == nil && fallbackTarget != "" && fallbackTarget != resolvedTarget {
			stats = runHTTP3BenchmarkOnce(client, fallbackTarget, duration, concurrency)
		}
	}
	if stats.Requests == 0 && stats.Errors > 0 {
		return stats, errors.New("benchmark failed: all requests errored")
	}
	return stats, nil
}

func runHTTP3BenchmarkOnce(client *http.Client, target string, duration time.Duration, concurrency int) Stats {
	collector := newBenchmarkCollector()
	stop := time.Now().Add(duration)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(stop) {
				measurement, err := doHTTPBenchmarkRequest(client, target)
				if err != nil {
					collector.recordError(err)
					continue
				}
				collector.recordSuccess(measurement)
			}
		}()
	}
	wg.Wait()
	return collector.finalize(duration)
}

func normalizeHTTP3BenchmarkTarget(target string) (string, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return "", err
	}

	switch parsed.Scheme {
	case "h3":
		parsed.Scheme = "https"
		return parsed.String(), nil
	case "https":
		return parsed.String(), nil
	default:
		return "", errors.New("h3 benchmark requires https:// or h3:// target")
	}
}

func discoverAltSvcH3Target(target string, insecure bool) (string, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "https" {
		return "", nil
	}

	transport := &http.Transport{
		ForceAttemptHTTP2: true,
		// #nosec G402 -- insecure TLS is gated by explicit allow_insecure_tls validation for lab use.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure, NextProtos: []string{"h2", "http/1.1"}},
	}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}
	defer transport.CloseIdleConnections()

	resp, err := client.Get(parsed.String())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	authority := extractH3Authority(resp.Header.Values("Alt-Svc"))
	if authority == "" {
		return "", nil
	}

	resolved := *parsed
	if strings.HasPrefix(authority, ":") {
		resolved.Host = parsed.Hostname() + authority
		return resolved.String(), nil
	}
	if strings.Contains(authority, ":") {
		resolved.Host = authority
		return resolved.String(), nil
	}
	return "", nil
}

func extractH3Authority(values []string) string {
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			for _, prefix := range []string{"h3=", "h3-29=", "h3-32="} {
				if !strings.HasPrefix(part, prefix) {
					continue
				}
				start := strings.Index(part, "\"")
				if start < 0 {
					continue
				}
				quoted := part[start+1:]
				end := strings.Index(quoted, "\"")
				if end < 0 {
					continue
				}
				return quoted[:end]
			}
		}
	}
	return ""
}

func runLoopbackH3Protocol(target string, duration time.Duration, concurrency int) (Stats, error) {
	address, path, loopbackOnly, err := parseTritonTarget(target)
	if err != nil {
		return Stats{}, err
	}

	collector := newBenchmarkCollector()
	stop := time.Now().Add(duration)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(stop) {
				start := time.Now()
				resp, err := runSingleH3Request(address, path, duration, loopbackOnly)
				if err != nil {
					collector.recordError(err)
					continue
				}
				total := time.Since(start)
				firstByte := total
				collector.recordSuccess(benchMeasurement{
					Total:     total,
					FirstByte: firstByte,
					BytesRead: int64(len(resp.Body)),
				})
			}
		}()
	}
	wg.Wait()

	stats := collector.finalize(duration)
	if stats.Requests == 0 && stats.Errors > 0 {
		return stats, errors.New("benchmark failed: all requests errored")
	}
	return stats, nil
}

type benchMeasurement struct {
	Total     time.Duration
	Connect   time.Duration
	TLS       time.Duration
	FirstByte time.Duration
	Transfer  time.Duration
	BytesRead int64
}

type benchmarkCollector struct {
	requests       int64
	errors         int64
	totalMS        int64
	connectMS      int64
	tlsMS          int64
	firstByteMS    int64
	transferMS     int64
	connectCount   int64
	tlsCount       int64
	firstByteCount int64
	transferCount  int64
	bytesRead      int64
	sampleMu       sync.Mutex
	totalSamples   []float64
	errorSummary   map[string]int64
}

const maxBenchSamples = 8192

func newBenchmarkCollector() *benchmarkCollector {
	return &benchmarkCollector{
		errorSummary: make(map[string]int64),
	}
}

func (c *benchmarkCollector) recordSuccess(m benchMeasurement) {
	atomic.AddInt64(&c.requests, 1)
	atomic.AddInt64(&c.bytesRead, m.BytesRead)
	atomic.AddInt64(&c.totalMS, durationToMillis(m.Total))
	if m.Connect > 0 {
		atomic.AddInt64(&c.connectMS, durationToMillis(m.Connect))
		atomic.AddInt64(&c.connectCount, 1)
	}
	if m.TLS > 0 {
		atomic.AddInt64(&c.tlsMS, durationToMillis(m.TLS))
		atomic.AddInt64(&c.tlsCount, 1)
	}
	if m.FirstByte > 0 {
		atomic.AddInt64(&c.firstByteMS, durationToMillis(m.FirstByte))
		atomic.AddInt64(&c.firstByteCount, 1)
	}
	if m.Transfer > 0 {
		atomic.AddInt64(&c.transferMS, durationToMillis(m.Transfer))
		atomic.AddInt64(&c.transferCount, 1)
	}
	c.recordSample(float64(m.Total) / float64(time.Millisecond))
}

func (c *benchmarkCollector) recordError(err error) {
	atomic.AddInt64(&c.errors, 1)
	category := categorizeBenchError(err)
	c.sampleMu.Lock()
	c.errorSummary[category]++
	c.sampleMu.Unlock()
}

func (c *benchmarkCollector) recordSample(value float64) {
	c.sampleMu.Lock()
	defer c.sampleMu.Unlock()
	if len(c.totalSamples) < maxBenchSamples {
		c.totalSamples = append(c.totalSamples, value)
		return
	}
	idx := int(atomic.LoadInt64(&c.requests)-1) % maxBenchSamples
	c.totalSamples[idx] = value
}

func (c *benchmarkCollector) finalize(duration time.Duration) Stats {
	requests := atomic.LoadInt64(&c.requests)
	errorsCount := atomic.LoadInt64(&c.errors)
	totalAttempts := requests + errorsCount
	avg := 0.0
	if requests > 0 {
		avg = float64(atomic.LoadInt64(&c.totalMS)) / float64(requests)
	}

	c.sampleMu.Lock()
	samples := append([]float64(nil), c.totalSamples...)
	summary := cloneInt64Map(c.errorSummary)
	c.sampleMu.Unlock()

	return Stats{
		Requests:      requests,
		Errors:        errorsCount,
		AverageMS:     avg,
		RequestsPerS:  float64(requests) / duration.Seconds(),
		Transferred:   atomic.LoadInt64(&c.bytesRead),
		ErrorRate:     ratio(errorsCount, totalAttempts),
		Latency:       computePercentiles(samples),
		Phases:        c.phaseAverages(),
		ErrorSummary:  summary,
		SampledPoints: len(samples),
	}
}

func (c *benchmarkCollector) phaseAverages() PhaseAverages {
	return PhaseAverages{
		ConnectMS:   averageAtomic(atomic.LoadInt64(&c.connectMS), atomic.LoadInt64(&c.connectCount)),
		TLSMS:       averageAtomic(atomic.LoadInt64(&c.tlsMS), atomic.LoadInt64(&c.tlsCount)),
		FirstByteMS: averageAtomic(atomic.LoadInt64(&c.firstByteMS), atomic.LoadInt64(&c.firstByteCount)),
		TransferMS:  averageAtomic(atomic.LoadInt64(&c.transferMS), atomic.LoadInt64(&c.transferCount)),
	}
}

func doHTTPBenchmarkRequest(client *http.Client, target string) (benchMeasurement, error) {
	var connectStart, connectDone, tlsStart, tlsDone, gotFirstByte time.Time
	trace := &httptrace.ClientTrace{
		ConnectStart:         func(_, _ string) { connectStart = time.Now() },
		ConnectDone:          func(_, _ string, _ error) { connectDone = time.Now() },
		TLSHandshakeStart:    func() { tlsStart = time.Now() },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { tlsDone = time.Now() },
		GotFirstResponseByte: func() { gotFirstByte = time.Now() },
	}

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return benchMeasurement{}, err
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return benchMeasurement{}, err
	}
	defer resp.Body.Close()

	n, copyErr := io.Copy(io.Discard, resp.Body)
	end := time.Now()
	if copyErr != nil {
		return benchMeasurement{}, copyErr
	}

	return benchMeasurement{
		Total:     end.Sub(start),
		Connect:   durationBetween(connectStart, connectDone),
		TLS:       durationBetween(tlsStart, tlsDone),
		FirstByte: durationBetween(start, gotFirstByte),
		Transfer:  durationBetween(gotFirstByte, end),
		BytesRead: n,
	}, nil
}

func durationBetween(start, end time.Time) time.Duration {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	return end.Sub(start)
}

func durationToMillis(v time.Duration) int64 {
	return v.Milliseconds()
}

func averageAtomic(total, count int64) float64 {
	if count <= 0 {
		return 0
	}
	return float64(total) / float64(count)
}

func ratio(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func categorizeBenchError(err error) string {
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

func computePercentiles(samples []float64) Percentiles {
	if len(samples) == 0 {
		return Percentiles{}
	}
	sorted := append([]float64(nil), samples...)
	sortFloat64s(sorted)
	return Percentiles{
		P50: percentile(sorted, 0.50),
		P95: percentile(sorted, 0.95),
		P99: percentile(sorted, 0.99),
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

func sortFloat64s(values []float64) {
	for i := 1; i < len(values); i++ {
		current := values[i]
		j := i - 1
		for ; j >= 0 && values[j] > current; j-- {
			values[j+1] = values[j]
		}
		values[j+1] = current
	}
}

func cloneInt64Map(input map[string]int64) map[string]int64 {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]int64, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func parseTritonTarget(target string) (address string, path string, loopbackOnly bool, err error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return "", "", false, err
	}
	if parsed.Scheme != "triton" {
		return "", "", false, errors.New("h3 benchmark currently supports only triton:// targets")
	}
	loopbackOnly = parsed.Host == "loopback"
	if parsed.Path == "" {
		path = "/ping"
	} else {
		path = parsed.Path
	}
	return parsed.Host, path, loopbackOnly, nil
}

func runSingleH3Request(address, path string, timeout time.Duration, loopbackOnly bool) (*h3.Response, error) {
	if !loopbackOnly {
		return h3.RoundTripAddress(address, http.MethodGet, path, nil, timeout)
	}

	listener, err := transport.ListenQUIC("127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	listener.SetAutoEcho(false)

	dialer := transport.NewDialer(timeout)
	session, err := dialer.DialSession(listener.Addr().String())
	if err != nil {
		return nil, err
	}
	defer session.Close()

	serverConn, err := listener.Accept()
	if err != nil {
		return nil, err
	}

	service := h3.NewServer(listener, serverConn, appmux.New())
	client := h3.NewClient(session)
	return service.ServeRoundTrip(client, http.MethodGet, path, nil)
}
