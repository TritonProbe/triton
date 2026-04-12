package probe

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tritonprobe/triton/internal/appmux"
	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/h3"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/quic/transport"
	"github.com/tritonprobe/triton/internal/realh3"
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
	TLS        map[string]any   `json:"tls" yaml:"tls"`
	Headers    http.Header      `json:"headers" yaml:"headers"`
	Analysis   map[string]any   `json:"analysis,omitempty" yaml:"analysis,omitempty"`
}

type testPlan struct {
	requested []string
	executed  []string
	skipped   []map[string]any
}

type testDefinition struct {
	Coverage string
	Summary  string
	Reason   string
}

var knownProbeTests = []string{
	"handshake",
	"tls",
	"latency",
	"throughput",
	"streams",
	"alt-svc",
	"0rtt",
	"migration",
	"ecn",
	"retry",
	"version",
	"qpack",
	"congestion",
	"loss",
	"spin-bit",
}

var probeTestDefinitions = map[string]testDefinition{
	"handshake":  {Coverage: "full", Summary: "basic handshake timing is implemented"},
	"tls":        {Coverage: "full", Summary: "TLS metadata capture is implemented"},
	"latency":    {Coverage: "full", Summary: "sampled latency measurements are implemented"},
	"throughput": {Coverage: "full", Summary: "response size and throughput estimates are implemented"},
	"streams":    {Coverage: "full", Summary: "basic concurrent stream sampling is implemented"},
	"alt-svc":    {Coverage: "full", Summary: "Alt-Svc header observation is implemented"},
	"0rtt": {
		Coverage: "partial",
		Summary:  "measured via HTTP/3 session resumption checks rather than true early data",
		Reason:   "true 0-RTT early-data probing is not implemented yet",
	},
	"migration": {
		Coverage: "partial",
		Summary:  "measured via endpoint capability checks rather than live path rebinding",
		Reason:   "live connection migration probing is not implemented yet",
	},
	"ecn": {
		Coverage: "partial",
		Summary:  "approximated from observable response and protocol metadata rather than packet markings",
		Reason:   "ECN observation currently requires an HTTP/3 probe target",
	},
	"retry": {
		Coverage: "partial",
		Summary:  "approximated from handshake outcome because Retry packets are not exposed at this client layer",
		Reason:   "retry observation currently requires an HTTP/3 probe target",
	},
	"version": {
		Coverage: "partial",
		Summary:  "approximated from observed protocol and ALPN rather than packet-level QUIC version negotiation",
		Reason:   "version observation currently requires an HTTP/3 probe target",
	},
	"qpack": {
		Coverage: "partial",
		Summary:  "approximated via header-block size estimates rather than real QPACK dynamic-table inspection",
		Reason:   "QPACK approximation requires an HTTP/3 probe target",
	},
	"congestion": {
		Coverage: "partial",
		Summary:  "approximated from latency spread and concurrent request pressure",
		Reason:   "congestion profiling currently relies on latency and concurrency signals",
	},
	"loss": {
		Coverage: "partial",
		Summary:  "approximated from repeated request failures and timeout/error pressure",
		Reason:   "loss probing currently relies on repeated request error signals",
	},
	"spin-bit": {
		Coverage: "partial",
		Summary:  "approximated from sampled RTT spread rather than packet-level spin-bit observation",
		Reason:   "spin-bit observation currently requires an HTTP/3 probe target",
	},
}

func Run(target string, cfg config.ProbeConfig) (*Result, error) {
	plan := newTestPlan(cfg.DefaultTests)
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}
	if parsed.Scheme == "h3" {
		return runStandardH3Probe(parsed, cfg)
	}
	if parsed.Scheme == "triton" && parsed.Host == "loopback" {
		return runLoopbackProbe(parsed, cfg)
	}
	if parsed.Scheme == "triton" {
		return runRemoteTritonProbe(parsed, cfg, plan)
	}

	var dnsStart, dnsDone, connectStart, connectDone, tlsStart, tlsDone, gotFirstByte time.Time
	trace := &httptrace.ClientTrace{
		DNSStart:             func(httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:              func(httptrace.DNSDoneInfo) { dnsDone = time.Now() },
		ConnectStart:         func(_, _ string) { connectStart = time.Now() },
		ConnectDone:          func(_, _ string, _ error) { connectDone = time.Now() },
		TLSHandshakeStart:    func() { tlsStart = time.Now() },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { tlsDone = time.Now() },
		GotFirstResponseByte: func() { gotFirstByte = time.Now() },
	}

	client := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			ForceAttemptHTTP2: true,
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: cfg.Insecure, NextProtos: []string{"h2", "http/1.1"}},
			DialContext: (&net.Dialer{
				Timeout: cfg.Timeout,
			}).DialContext,
		},
	}

	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return nil, err
	}

	result := &Result{
		ID:        fmt.Sprintf("pr-%s", time.Now().UTC().Format("20060102-150405")),
		Target:    parsed.String(),
		Timestamp: time.Now().UTC(),
		Duration:  time.Since(start),
		Status:    resp.StatusCode,
		Proto:     resp.Proto,
		Headers:   resp.Header.Clone(),
		Timings: map[string]int64{
			"dns":        millisBetween(dnsStart, dnsDone),
			"connect":    millisBetween(connectStart, connectDone),
			"tls":        millisBetween(tlsStart, tlsDone),
			"first_byte": millisBetween(start, gotFirstByte),
			"total":      time.Since(start).Milliseconds(),
		},
		TLS:      map[string]any{},
		Analysis: analyzeHeaders(resp.Header, plan),
	}
	if resp.TLS != nil && plan.shouldRun("tls") {
		plan.executed = append(plan.executed, "tls")
		result.TLS = tlsMetadata(resp.TLS)
	}
	if plan.shouldRun("handshake") {
		plan.executed = append(plan.executed, "handshake")
	}
	if plan.shouldRun("throughput") {
		plan.executed = append(plan.executed, "throughput")
		enrichResponseAnalysis(result, bodyBytes)
	}
	if plan.shouldRun("latency") {
		plan.executed = append(plan.executed, "latency")
		enrichLatencyAnalysis(result, cfg, func() (int, int64, error) {
			return doStandardRequest(client, parsed.String())
		})
	}
	if plan.shouldRun("streams") {
		plan.executed = append(plan.executed, "streams")
		enrichStreamAnalysis(result, cfg, func() (int, int64, error) {
			return doStandardRequest(client, parsed.String())
		})
	}
	if plan.shouldRun("loss") {
		plan.executed = append(plan.executed, "loss")
		result.Analysis["loss"] = estimateLossAnalysis(cfg, func() (int, int64, error) {
			return doStandardRequest(client, parsed.String())
		})
	}
	if plan.shouldRun("congestion") {
		plan.executed = append(plan.executed, "congestion")
		result.Analysis["congestion"] = estimateCongestionAnalysis(cfg, func() (int, int64, error) {
			return doStandardRequest(client, parsed.String())
		})
	}
	if plan.shouldRun("migration") {
		if migration, ok := measureHTTPMigration(parsed.String(), cfg); ok {
			plan.executed = append(plan.executed, "migration")
			result.Analysis["migration"] = migration
		}
	}
	finalizeTestPlan(result, plan)
	return result, nil
}

func runStandardH3Probe(parsed *url.URL, cfg config.ProbeConfig) (*Result, error) {
	plan := newTestPlan(cfg.DefaultTests)
	target := *parsed
	target.Scheme = "https"
	before, err := observability.ListQLOGFiles(cfg.TraceDir)
	if err != nil {
		return nil, err
	}

	var dnsStart, dnsDone, connectStart, connectDone, tlsStart, tlsDone, gotFirstByte time.Time
	trace := &httptrace.ClientTrace{
		DNSStart:             func(httptrace.DNSStartInfo) { dnsStart = time.Now() },
		DNSDone:              func(httptrace.DNSDoneInfo) { dnsDone = time.Now() },
		ConnectStart:         func(_, _ string) { connectStart = time.Now() },
		ConnectDone:          func(_, _ string, _ error) { connectDone = time.Now() },
		TLSHandshakeStart:    func() { tlsStart = time.Now() },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { tlsDone = time.Now() },
		GotFirstResponseByte: func() { gotFirstByte = time.Now() },
	}

	client, transport := realh3.NewClient(cfg.Timeout, cfg.Insecure, cfg.TraceDir)
	defer transport.Close()

	req, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return nil, err
	}

	result := &Result{
		ID:        fmt.Sprintf("pr-%s", time.Now().UTC().Format("20060102-150405")),
		Target:    parsed.String(),
		Timestamp: time.Now().UTC(),
		Duration:  time.Since(start),
		Status:    resp.StatusCode,
		Proto:     resp.Proto,
		Headers:   resp.Header.Clone(),
		Timings: map[string]int64{
			"dns":        millisBetween(dnsStart, dnsDone),
			"connect":    millisBetween(connectStart, connectDone),
			"tls":        millisBetween(tlsStart, tlsDone),
			"first_byte": millisBetween(start, gotFirstByte),
			"total":      time.Since(start).Milliseconds(),
		},
		TLS:      map[string]any{},
		Analysis: analyzeHeaders(resp.Header, plan),
	}
	if resp.TLS != nil && plan.shouldRun("tls") {
		plan.executed = append(plan.executed, "tls")
		result.TLS = tlsMetadata(resp.TLS)
	}
	after, err := observability.ListQLOGFiles(cfg.TraceDir)
	if err != nil {
		return nil, err
	}
	result.TraceFiles = observability.DiffQLOGFiles(before, after)
	if plan.shouldRun("handshake") {
		plan.executed = append(plan.executed, "handshake")
	}
	if plan.shouldRun("throughput") {
		plan.executed = append(plan.executed, "throughput")
		enrichResponseAnalysis(result, bodyBytes)
	}
	if plan.shouldRun("latency") {
		plan.executed = append(plan.executed, "latency")
		enrichLatencyAnalysis(result, cfg, func() (int, int64, error) {
			return doStandardRequest(client, target.String())
		})
	}
	if plan.shouldRun("streams") {
		plan.executed = append(plan.executed, "streams")
		enrichStreamAnalysis(result, cfg, func() (int, int64, error) {
			return doStandardRequest(client, target.String())
		})
	}
	if plan.shouldRun("loss") {
		plan.executed = append(plan.executed, "loss")
		result.Analysis["loss"] = estimateLossAnalysis(cfg, func() (int, int64, error) {
			return doStandardRequest(client, target.String())
		})
	}
	if plan.shouldRun("congestion") {
		plan.executed = append(plan.executed, "congestion")
		result.Analysis["congestion"] = estimateCongestionAnalysis(cfg, func() (int, int64, error) {
			return doStandardRequest(client, target.String())
		})
	}
	if plan.shouldRun("ecn") {
		plan.executed = append(plan.executed, "ecn")
		result.Analysis["ecn"] = estimateECNAnalysis(resp.Header, result.Proto, result.TLS)
	}
	if plan.shouldRun("spin-bit") {
		plan.executed = append(plan.executed, "spin-bit")
		result.Analysis["spin-bit"] = estimateSpinBitAnalysis(cfg, func() (int, int64, error) {
			return doStandardRequest(client, target.String())
		})
	}
	if plan.shouldRun("version") {
		plan.executed = append(plan.executed, "version")
		result.Analysis["version"] = estimateVersionAnalysis(result.Proto, result.TLS)
	}
	if plan.shouldRun("retry") {
		plan.executed = append(plan.executed, "retry")
		result.Analysis["retry"] = estimateRetryAnalysis(result.Proto, result.TLS, result.Timings["connect"], result.Timings["tls"])
	}
	if plan.shouldRun("qpack") {
		plan.executed = append(plan.executed, "qpack")
		result.Analysis["qpack"] = estimateQPACKAnalysis(resp.Header, resp.StatusCode)
	}
	if plan.shouldRun("0rtt") {
		if zeroRTT, ok := measureH3Resumption(target.String(), cfg); ok {
			plan.executed = append(plan.executed, "0rtt")
			result.Analysis["0rtt"] = zeroRTT
		}
	}
	if plan.shouldRun("migration") {
		if migration, ok := measureHTTPMigration(target.String(), cfg); ok {
			plan.executed = append(plan.executed, "migration")
			result.Analysis["migration"] = migration
		}
	}
	finalizeTestPlan(result, plan)
	return result, nil
}

func millisBetween(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func runLoopbackProbe(parsed *url.URL, cfg config.ProbeConfig) (*Result, error) {
	plan := newTestPlan(cfg.DefaultTests)
	listener, err := transport.ListenQUIC("127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	listener.SetAutoEcho(false)

	dialer := transport.NewDialer(cfg.Timeout)
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

	path := parsed.Path
	if path == "" {
		path = "/ping"
	}
	start := time.Now()
	resp, err := service.ServeRoundTrip(client, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	headers := make(http.Header)
	for k, v := range resp.Headers {
		if len(k) > 0 && k[0] == ':' {
			continue
		}
		headers.Set(k, v)
	}

	result := &Result{
		ID:        fmt.Sprintf("pr-%s", time.Now().UTC().Format("20060102-150405")),
		Target:    parsed.String(),
		Timestamp: time.Now().UTC(),
		Duration:  time.Since(start),
		Status:    resp.StatusCode,
		Proto:     "HTTP/3-loopback",
		Headers:   headers,
		Timings: map[string]int64{
			"total": time.Since(start).Milliseconds(),
		},
		TLS: map[string]any{
			"mode": "in-process-loopback",
		},
		Analysis: map[string]any{
			"transport": "loopback",
		},
	}
	if plan.shouldRun("handshake") {
		plan.executed = append(plan.executed, "handshake")
	}
	if plan.shouldRun("throughput") {
		plan.executed = append(plan.executed, "throughput")
		enrichResponseAnalysis(result, int64(len(resp.Body)))
	}
	if plan.shouldRun("latency") {
		plan.executed = append(plan.executed, "latency")
		enrichLatencyAnalysis(result, cfg, func() (int, int64, error) {
			repeatResp, err := runSingleProbeH3Request("loopback", path, cfg.Timeout, true)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("streams") {
		plan.executed = append(plan.executed, "streams")
		enrichStreamAnalysis(result, cfg, func() (int, int64, error) {
			repeatResp, err := runSingleProbeH3Request("loopback", path, cfg.Timeout, true)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("loss") {
		plan.executed = append(plan.executed, "loss")
		result.Analysis["loss"] = estimateLossAnalysis(cfg, func() (int, int64, error) {
			repeatResp, err := runSingleProbeH3Request("loopback", path, cfg.Timeout, true)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("congestion") {
		plan.executed = append(plan.executed, "congestion")
		result.Analysis["congestion"] = estimateCongestionAnalysis(cfg, func() (int, int64, error) {
			repeatResp, err := runSingleProbeH3Request("loopback", path, cfg.Timeout, true)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("ecn") {
		plan.executed = append(plan.executed, "ecn")
		result.Analysis["ecn"] = estimateECNAnalysis(headers, result.Proto, result.TLS)
	}
	if plan.shouldRun("spin-bit") {
		plan.executed = append(plan.executed, "spin-bit")
		result.Analysis["spin-bit"] = estimateSpinBitAnalysis(cfg, func() (int, int64, error) {
			repeatResp, err := runSingleProbeH3Request("loopback", path, cfg.Timeout, true)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("version") {
		plan.executed = append(plan.executed, "version")
		result.Analysis["version"] = estimateVersionAnalysis(result.Proto, result.TLS)
	}
	if plan.shouldRun("retry") {
		plan.executed = append(plan.executed, "retry")
		result.Analysis["retry"] = estimateRetryAnalysis(result.Proto, result.TLS, result.Timings["total"], 0)
	}
	if plan.shouldRun("qpack") {
		plan.executed = append(plan.executed, "qpack")
		result.Analysis["qpack"] = estimateQPACKAnalysis(headers, resp.StatusCode)
	}
	if plan.shouldRun("migration") {
		if migration, ok := measureLoopbackMigration(cfg); ok {
			plan.executed = append(plan.executed, "migration")
			result.Analysis["migration"] = migration
		}
	}
	finalizeTestPlan(result, plan)
	return result, nil
}

func runRemoteTritonProbe(parsed *url.URL, cfg config.ProbeConfig, plan testPlan) (*Result, error) {
	path := parsed.Path
	if path == "" {
		path = "/ping"
	}
	start := time.Now()
	resp, err := h3.RoundTripAddress(parsed.Host, http.MethodGet, path, nil, cfg.Timeout)
	if err != nil {
		return nil, err
	}

	headers := make(http.Header)
	for k, v := range resp.Headers {
		if len(k) > 0 && k[0] == ':' {
			continue
		}
		headers.Set(k, v)
	}

	result := &Result{
		ID:        fmt.Sprintf("pr-%s", time.Now().UTC().Format("20060102-150405")),
		Target:    parsed.String(),
		Timestamp: time.Now().UTC(),
		Duration:  time.Since(start),
		Status:    resp.StatusCode,
		Proto:     "HTTP/3-triton",
		Headers:   headers,
		Timings: map[string]int64{
			"total": time.Since(start).Milliseconds(),
		},
		TLS: map[string]any{
			"mode": "experimental-udp-h3",
		},
		Analysis: map[string]any{
			"transport": "experimental-triton-h3",
		},
	}
	if plan.shouldRun("handshake") {
		plan.executed = append(plan.executed, "handshake")
	}
	if plan.shouldRun("throughput") {
		plan.executed = append(plan.executed, "throughput")
		enrichResponseAnalysis(result, int64(len(resp.Body)))
	}
	if plan.shouldRun("latency") {
		plan.executed = append(plan.executed, "latency")
		enrichLatencyAnalysis(result, cfg, func() (int, int64, error) {
			repeatResp, err := h3.RoundTripAddress(parsed.Host, http.MethodGet, path, nil, cfg.Timeout)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("streams") {
		plan.executed = append(plan.executed, "streams")
		enrichStreamAnalysis(result, cfg, func() (int, int64, error) {
			repeatResp, err := h3.RoundTripAddress(parsed.Host, http.MethodGet, path, nil, cfg.Timeout)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("loss") {
		plan.executed = append(plan.executed, "loss")
		result.Analysis["loss"] = estimateLossAnalysis(cfg, func() (int, int64, error) {
			repeatResp, err := h3.RoundTripAddress(parsed.Host, http.MethodGet, path, nil, cfg.Timeout)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("congestion") {
		plan.executed = append(plan.executed, "congestion")
		result.Analysis["congestion"] = estimateCongestionAnalysis(cfg, func() (int, int64, error) {
			repeatResp, err := h3.RoundTripAddress(parsed.Host, http.MethodGet, path, nil, cfg.Timeout)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("ecn") {
		plan.executed = append(plan.executed, "ecn")
		result.Analysis["ecn"] = estimateECNAnalysis(headers, result.Proto, result.TLS)
	}
	if plan.shouldRun("spin-bit") {
		plan.executed = append(plan.executed, "spin-bit")
		result.Analysis["spin-bit"] = estimateSpinBitAnalysis(cfg, func() (int, int64, error) {
			repeatResp, err := h3.RoundTripAddress(parsed.Host, http.MethodGet, path, nil, cfg.Timeout)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("version") {
		plan.executed = append(plan.executed, "version")
		result.Analysis["version"] = estimateVersionAnalysis(result.Proto, result.TLS)
	}
	if plan.shouldRun("retry") {
		plan.executed = append(plan.executed, "retry")
		result.Analysis["retry"] = estimateRetryAnalysis(result.Proto, result.TLS, result.Timings["total"], 0)
	}
	if plan.shouldRun("qpack") {
		plan.executed = append(plan.executed, "qpack")
		result.Analysis["qpack"] = estimateQPACKAnalysis(headers, resp.StatusCode)
	}
	if plan.shouldRun("migration") {
		if migration, ok := measureRemoteTritonMigration(parsed.Host, cfg); ok {
			plan.executed = append(plan.executed, "migration")
			result.Analysis["migration"] = migration
		}
	}
	finalizeTestPlan(result, plan)
	return result, nil
}

func runSingleProbeH3Request(address, path string, timeout time.Duration, loopbackOnly bool) (*h3.Response, error) {
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

func doStandardRequest(client *http.Client, target string) (int, int64, error) {
	status, body, err := doStandardRequestBody(client, target)
	return status, int64(len(body)), err
}

func doStandardRequestBody(client *http.Client, target string) (int, []byte, error) {
	resp, err := client.Get(target)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

func measureH3Resumption(target string, cfg config.ProbeConfig) (map[string]any, bool) {
	cache := tls.NewLRUClientSessionCache(8)

	firstClient, firstTransport := realh3.NewClientWithSessionCache(cfg.Timeout, cfg.Insecure, "", cache)
	defer firstTransport.Close()
	firstStart := time.Now()
	firstResp, err := firstClient.Get(target)
	if err != nil {
		return map[string]any{
			"supported": false,
			"error":     err.Error(),
			"mode":      "tls-resumption-check",
		}, true
	}
	_, _ = io.Copy(io.Discard, firstResp.Body)
	firstResp.Body.Close()
	firstDuration := time.Since(firstStart)
	firstResumed := firstResp.TLS != nil && firstResp.TLS.DidResume

	secondClient, secondTransport := realh3.NewClientWithSessionCache(cfg.Timeout, cfg.Insecure, "", cache)
	defer secondTransport.Close()
	secondStart := time.Now()
	secondResp, err := secondClient.Get(target)
	if err != nil {
		return map[string]any{
			"supported":       false,
			"initial_ms":      float64(firstDuration) / float64(time.Millisecond),
			"initial_resumed": firstResumed,
			"error":           err.Error(),
			"mode":            "tls-resumption-check",
		}, true
	}
	_, _ = io.Copy(io.Discard, secondResp.Body)
	secondResp.Body.Close()
	secondDuration := time.Since(secondStart)
	secondResumed := secondResp.TLS != nil && secondResp.TLS.DidResume

	saved := firstDuration - secondDuration
	return map[string]any{
		"supported":       secondResumed,
		"mode":            "tls-resumption-check",
		"initial_ms":      float64(firstDuration) / float64(time.Millisecond),
		"resumed_ms":      float64(secondDuration) / float64(time.Millisecond),
		"initial_resumed": firstResumed,
		"resumed":         secondResumed,
		"time_saved_ms":   float64(saved) / float64(time.Millisecond),
		"requested_0rtt":  true,
		"note":            "measures HTTP/3 connection resumption; true early data is not exposed at this layer",
	}, true
}

func measureHTTPMigration(target string, cfg config.ProbeConfig) (map[string]any, bool) {
	parsed, err := url.Parse(target)
	if err != nil {
		return map[string]any{
			"supported": false,
			"error":     err.Error(),
			"mode":      "endpoint-capability-check",
		}, true
	}
	parsed.Path = "/migration-test"
	parsed.RawQuery = ""

	client := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			ForceAttemptHTTP2: true,
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: cfg.Insecure, NextProtos: []string{"h2", "http/1.1"}},
			DialContext: (&net.Dialer{
				Timeout: cfg.Timeout,
			}).DialContext,
		},
	}
	status, body, err := doStandardRequestBody(client, parsed.String())
	result := map[string]any{
		"mode":            "endpoint-capability-check",
		"target":          parsed.String(),
		"status_class":    status / 100,
		"body_bytes":      len(body),
		"requested_check": true,
		"note":            "checks the migration endpoint contract; it does not perform live path rebinding",
	}
	if err != nil {
		result["supported"] = false
		result["error"] = err.Error()
		return result, true
	}
	mergeMigrationContract(result, body, status/100 == 2)
	return result, true
}

func measureLoopbackMigration(cfg config.ProbeConfig) (map[string]any, bool) {
	start := time.Now()
	resp, err := runSingleProbeH3Request("loopback", "/migration-test", cfg.Timeout, true)
	result := map[string]any{
		"mode":            "endpoint-capability-check",
		"target":          "triton://loopback/migration-test",
		"requested_check": true,
		"note":            "checks the migration endpoint contract; it does not perform live path rebinding",
	}
	if err != nil {
		result["supported"] = false
		result["error"] = err.Error()
		return result, true
	}
	result["status_class"] = resp.StatusCode / 100
	result["body_bytes"] = len(resp.Body)
	result["duration_ms"] = float64(time.Since(start)) / float64(time.Millisecond)
	mergeMigrationContract(result, resp.Body, resp.StatusCode/100 == 2)
	return result, true
}

func measureRemoteTritonMigration(address string, cfg config.ProbeConfig) (map[string]any, bool) {
	start := time.Now()
	resp, err := h3.RoundTripAddress(address, http.MethodGet, "/migration-test", nil, cfg.Timeout)
	result := map[string]any{
		"mode":            "endpoint-capability-check",
		"target":          "triton://" + address + "/migration-test",
		"requested_check": true,
		"note":            "checks the migration endpoint contract; it does not perform live path rebinding",
	}
	if err != nil {
		result["supported"] = false
		result["error"] = err.Error()
		return result, true
	}
	result["status_class"] = resp.StatusCode / 100
	result["body_bytes"] = len(resp.Body)
	result["duration_ms"] = float64(time.Since(start)) / float64(time.Millisecond)
	mergeMigrationContract(result, resp.Body, resp.StatusCode/100 == 2)
	return result, true
}

func mergeMigrationContract(result map[string]any, body []byte, fallbackSupported bool) {
	if result == nil {
		return
	}
	result["supported"] = fallbackSupported
	if len(body) == 0 {
		return
	}

	var payload struct {
		Supported *bool  `json:"supported"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return
	}
	if payload.Supported != nil {
		result["supported"] = *payload.Supported
	}
	if payload.Message != "" {
		result["message"] = payload.Message
	}
}

func estimateQPACKAnalysis(headers http.Header, status int) map[string]any {
	blockHeaders := map[string]string{
		":status": fmt.Sprintf("%d", status),
	}
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

	return map[string]any{
		"supported":          true,
		"mode":               "header-block-estimate",
		"header_count":       headerCount,
		"raw_bytes":          rawBytes,
		"estimated_block":    encodedBytes,
		"estimated_ratio":    ratio,
		"compression_saving": rawBytes - encodedBytes,
		"note":               "approximates header block size from serialized H3 headers; it does not inspect real QPACK dynamic table behavior",
	}
}

func estimateVersionAnalysis(proto string, tlsMeta map[string]any) map[string]any {
	alpn, _ := tlsMeta["alpn"].(string)
	return map[string]any{
		"supported":        true,
		"mode":             "protocol-observation",
		"observed_proto":   proto,
		"alpn":             alpn,
		"quic_version":     "not_exposed",
		"negotiation_seen": false,
		"note":             "approximates QUIC version state from observed HTTP/3 protocol and ALPN; packet-level version negotiation is not exposed here",
	}
}

func estimateRetryAnalysis(proto string, tlsMeta map[string]any, connectMS, tlsMS int64) map[string]any {
	alpn, _ := tlsMeta["alpn"].(string)
	return map[string]any{
		"supported":            true,
		"mode":                 "handshake-observation",
		"observed_proto":       proto,
		"alpn":                 alpn,
		"retry_observed":       false,
		"connect_ms":           connectMS,
		"tls_ms":               tlsMS,
		"visibility":           "client-layer-limited",
		"note":                 "approximates Retry behavior from successful handshake observation; actual Retry packets are not exposed at this layer",
	}
}

func estimateECNAnalysis(headers http.Header, proto string, tlsMeta map[string]any) map[string]any {
	alpn, _ := tlsMeta["alpn"].(string)
	observedSignal := false
	for key := range headers {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "ecn") || strings.Contains(lower, "ce") {
			observedSignal = true
			break
		}
	}
	return map[string]any{
		"supported":        true,
		"mode":             "metadata-observation",
		"observed_proto":   proto,
		"alpn":             alpn,
		"ecn_visible":      observedSignal,
		"packet_marks":     "not_exposed",
		"note":             "approximates ECN visibility from observable protocol metadata and headers; packet-level ECN markings are not exposed here",
	}
}

func estimateSpinBitAnalysis(cfg config.ProbeConfig, request func() (int, int64, error)) map[string]any {
	latency := sampleLatencyProfile(cfg, request)
	rttEstimate := latency.P50
	stability := "steady"
	if latency.P95-latency.P50 > latency.P50 {
		stability = "variable"
	}
	return map[string]any{
		"supported":        true,
		"mode":             "rtt-sampling-estimate",
		"rtt_estimate_ms":  rttEstimate,
		"p95_ms":           latency.P95,
		"stability":        stability,
		"spin_observed":    false,
		"note":             "approximates spin-bit style RTT visibility from sampled request timings; actual packet-level spin-bit observation is not exposed here",
	}
}

func estimateLossAnalysis(cfg config.ProbeConfig, request func() (int, int64, error)) map[string]any {
	latency := sampleLatencyProfile(cfg, request)
	streams := runConcurrentSamples(minInt(cfg.DefaultStreams, 4), request)
	signal := "low"
	if latency.Errors > 0 || streams.Errors > 0 {
		signal = "elevated"
	}
	if latency.Errors >= 2 || streams.SuccessRate < 0.75 {
		signal = "high"
	}

	return map[string]any{
		"supported":           true,
		"mode":                "request-error-signal",
		"signal":              signal,
		"latency_errors":      latency.Errors,
		"latency_samples":     latency.Samples,
		"stream_attempts":     streams.Attempts,
		"stream_errors":       streams.Errors,
		"success_rate":        streams.SuccessRate,
		"error_categories":    streams.ErrorCategories,
		"timeout_indicators":  streams.ErrorCategories["timeout"],
		"note":                "approximates packet-loss pressure from repeated request failures and timeout/error categories; it does not inspect packet-level recovery",
	}
}

func estimateCongestionAnalysis(cfg config.ProbeConfig, request func() (int, int64, error)) map[string]any {
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

	return map[string]any{
		"supported":           true,
		"mode":                "latency-spread-estimate",
		"signal":              signal,
		"p50_ms":              latency.P50,
		"p95_ms":              latency.P95,
		"jitter_ms":           jitter,
		"spread_ratio":        spreadRatio,
		"stream_avg_ms":       streams.AverageMS,
		"stream_p95_ms":       streams.P95,
		"concurrent_attempts": streams.Attempts,
		"success_rate":        streams.SuccessRate,
		"note":                "approximates congestion pressure from latency spread and concurrent request slowdown; it does not read congestion-window telemetry",
	}
}

func tlsMetadata(state *tls.ConnectionState) map[string]any {
	meta := map[string]any{
		"version":         tlsVersion(state.Version),
		"cipher":          tls.CipherSuiteName(state.CipherSuite),
		"alpn":            state.NegotiatedProtocol,
		"server_name":     state.ServerName,
		"peer_certs":      len(state.PeerCertificates),
		"resumed":         state.DidResume,
		"handshake_state": "complete",
	}
	if len(state.VerifiedChains) > 0 {
		meta["verified_chains"] = len(state.VerifiedChains)
	}
	if len(state.PeerCertificates) > 0 {
		meta["leaf_cert"] = certificateSummary(state.PeerCertificates[0])
	}
	return meta
}

func certificateSummary(cert *x509.Certificate) map[string]any {
	if cert == nil {
		return map[string]any{}
	}
	return map[string]any{
		"subject":     cert.Subject.String(),
		"issuer":      cert.Issuer.String(),
		"dns_names":   append([]string(nil), cert.DNSNames...),
		"not_before":  cert.NotBefore.UTC().Format(time.RFC3339),
		"not_after":   cert.NotAfter.UTC().Format(time.RFC3339),
		"is_ca":       cert.IsCA,
		"serial":      cert.SerialNumber.String(),
		"fingerprint": fmt.Sprintf("%X", cert.Signature[:min(len(cert.Signature), 8)]),
	}
}

func analyzeHeaders(headers http.Header, plan testPlan) map[string]any {
	analysis := map[string]any{}
	altSvc := headers.Values("Alt-Svc")
	if len(altSvc) > 0 && plan.shouldRun("alt-svc") {
		analysis["alt_svc"] = map[string]any{
			"present": true,
			"values":  append([]string(nil), altSvc...),
		}
	}
	return analysis
}

func newTestPlan(requested []string) testPlan {
	normalized := make([]string, 0, len(requested))
	seen := map[string]struct{}{}
	for _, value := range requested {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		normalized = []string{"handshake", "tls", "latency", "throughput", "streams", "alt-svc"}
	}
	return testPlan{requested: normalized}
}

func (p testPlan) shouldRun(name string) bool {
	for _, requested := range p.requested {
		if requested == "all" || requested == name {
			return true
		}
	}
	return false
}

func finalizeTestPlan(result *Result, plan testPlan) {
	if result == nil {
		return
	}
	if result.Analysis == nil {
		result.Analysis = map[string]any{}
	}
	for _, requested := range expandRequestedTests(plan.requested) {
		if containsString(plan.executed, requested) {
			continue
		}
		definition, ok := probeTestDefinitions[requested]
		if !ok {
			plan.skipped = append(plan.skipped, map[string]any{
				"name":   requested,
				"reason": "unknown probe test",
			})
			continue
		}
		if definition.Reason == "" {
			continue
		}
		plan.skipped = append(plan.skipped, map[string]any{
			"name":   requested,
			"reason": definition.Reason,
		})
	}
	result.Analysis["test_plan"] = map[string]any{
		"requested": append([]string(nil), plan.requested...),
		"executed":  append([]string(nil), plan.executed...),
		"skipped":   append([]map[string]any(nil), plan.skipped...),
	}
	support := buildSupportSummary(result.Analysis, plan)
	if len(support) > 0 {
		result.Analysis["support"] = support
		result.Analysis["support_summary"] = buildSupportRollup(support)
	}
}

func buildSupportSummary(analysis map[string]any, plan testPlan) map[string]any {
	support := map[string]any{}
	for _, name := range expandRequestedTests(plan.requested) {
		addSupportEntry(support, name, analysis[name], true, containsString(plan.executed, name))
	}
	if len(support) == 0 {
		return nil
	}
	return support
}

func addSupportEntry(dst map[string]any, name string, value any, requested, executed bool) {
	if dst == nil || !requested {
		return
	}

	entry := map[string]any{
		"requested": requested,
	}

	definition, known := probeTestDefinitions[name]
	details, _ := value.(map[string]any)
	if len(details) == 0 {
		coverage := "unavailable"
		summary := "not requested or not executed"
		if known && definition.Coverage != "" {
			coverage = definition.Coverage
			summary = definition.Summary
		}
		entry["coverage"] = coverage
		entry["state"] = "not_run"
		if known && coverage == "unavailable" {
			entry["state"] = "unavailable"
		}
		if executed {
			entry["state"] = "available"
		}
		entry["summary"] = summary
		dst[name] = entry
		return
	}

	mode, _ := details["mode"].(string)
	message, _ := details["message"].(string)
	note, _ := details["note"].(string)
	supported, _ := details["supported"].(bool)
	entry["mode"] = mode

	switch {
	case known && definition.Coverage != "":
		entry["coverage"] = definition.Coverage
	case mode == "tls-resumption-check", mode == "endpoint-capability-check":
		entry["coverage"] = "partial"
	default:
		entry["coverage"] = "full"
		if mode == "" {
			entry["coverage"] = "observed"
		}
	}

	switch mode {
	case "tls-resumption-check", "endpoint-capability-check":
	}

	if supported {
		entry["state"] = "available"
	} else {
		entry["state"] = "unavailable"
	}

	summary := message
	if summary == "" {
		summary = note
	}
	if summary == "" && known {
		summary = definition.Summary
	}
	if summary == "" && mode != "" {
		summary = mode
	}
	entry["summary"] = summary
	dst[name] = entry
}

func expandRequestedTests(requested []string) []string {
	if containsString(requested, "all") {
		return append([]string(nil), knownProbeTests...)
	}
	return append([]string(nil), requested...)
}

func buildSupportRollup(support map[string]any) map[string]any {
	summary := map[string]any{
		"requested_tests": 0,
		"available":       0,
		"not_run":         0,
		"unavailable":     0,
		"full":            0,
		"partial":         0,
	}
	if len(support) == 0 {
		return summary
	}

	for _, key := range sortedProbeKeys(support) {
		entry, ok := support[key].(map[string]any)
		if !ok {
			continue
		}
		summary["requested_tests"] = summary["requested_tests"].(int) + 1
		if coverage, _ := entry["coverage"].(string); coverage != "" {
			if _, ok := summary[coverage]; ok {
				summary[coverage] = summary[coverage].(int) + 1
			}
		}
		if state, _ := entry["state"].(string); state != "" {
			if _, ok := summary[state]; ok {
				summary[state] = summary[state].(int) + 1
			}
		}
	}

	requested := summary["requested_tests"].(int)
	available := summary["available"].(int)
	if requested > 0 {
		summary["coverage_ratio"] = float64(available) / float64(requested)
	} else {
		summary["coverage_ratio"] = 0.0
	}
	return summary
}

func sortedProbeKeys(input map[string]any) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
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
	result.Analysis["response"] = map[string]any{
		"body_bytes":           bodyBytes,
		"throughput_bytes_sec": throughput,
		"throughput_bits_sec":  throughput * 8,
		"status_class":         result.Status / 100,
	}
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
	result.Analysis["latency"] = map[string]any{
		"samples":    latency.Samples,
		"avg_ms":     latency.AverageMS,
		"p50":        latency.P50,
		"p95":        latency.P95,
		"p99":        latency.P99,
		"errors":     latency.Errors,
		"samples_ms": append([]float64(nil), latency.SamplesMS...),
	}
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
	result.Analysis["streams"] = map[string]any{
		"attempted":        streams.Attempts,
		"successful":       streams.Successes,
		"errors":           streams.Errors,
		"success_rate":     streams.SuccessRate,
		"avg_latency_ms":   streams.AverageMS,
		"p95_latency_ms":   streams.P95,
		"throughput_bytes": streams.TotalBytes,
		"status_classes":   streams.StatusClasses,
		"error_categories": streams.ErrorCategories,
	}
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
	return latencyProfile{
		Samples:   len(values),
		Errors:    errorsCount,
		AverageMS: avg,
		P50:       percentile(sorted, 0.50),
		P95:       percentile(sorted, 0.95),
		P99:       percentile(sorted, 0.99),
		SamplesMS: values,
	}
}

func runConcurrentSamples(concurrency int, request func() (int, int64, error)) concurrentSummary {
	type sample struct {
		status  int
		bytes   int64
		latency float64
		err     error
	}

	results := make(chan sample, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			status, bytes, err := request()
			results <- sample{
				status:  status,
				bytes:   bytes,
				latency: float64(time.Since(start)) / float64(time.Millisecond),
				err:     err,
			}
		}()
	}
	wg.Wait()
	close(results)

	latencies := make([]float64, 0, concurrency)
	summary := concurrentSummary{
		Attempts:        concurrency,
		StatusClasses:   map[string]int{},
		ErrorCategories: map[string]int{},
	}
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
