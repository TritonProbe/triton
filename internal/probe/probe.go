package probe

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"time"

	"github.com/tritonprobe/triton/internal/appmux"
	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/h3"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/quic/transport"
	"github.com/tritonprobe/triton/internal/realh3"
	"github.com/tritonprobe/triton/internal/runid"
)

const maxProbeResponseBodyBytes int64 = 1 << 20

func Run(target string, cfg config.ProbeConfig) (*Result, error) {
	if cfg.Insecure && !cfg.AllowInsecureTLS {
		return nil, fmt.Errorf("probe insecure TLS requires allow_insecure_tls")
	}
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
			// #nosec G402 -- cfg.Insecure is gated by explicit allow_insecure_tls validation for lab use.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.Insecure, NextProtos: []string{"h2", "http/1.1"}},
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
		ID:        runid.New("pr"),
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
		TLS:      TLSMetadata{},
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
		plan.executed = append(plan.executed, "migration")
		result.Analysis["migration"] = measureHTTPMigration(parsed.String(), cfg)
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
		ID:        runid.New("pr"),
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
		TLS:      TLSMetadata{},
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
		plan.executed = append(plan.executed, "0rtt")
		result.Analysis["0rtt"] = measureH3Resumption(target.String(), cfg)
	}
	if plan.shouldRun("migration") {
		plan.executed = append(plan.executed, "migration")
		result.Analysis["migration"] = measureHTTPMigration(target.String(), cfg)
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
		ID:        runid.New("pr"),
		Target:    parsed.String(),
		Timestamp: time.Now().UTC(),
		Duration:  time.Since(start),
		Status:    resp.StatusCode,
		Proto:     "HTTP/3-loopback",
		Headers:   headers,
		Timings: map[string]int64{
			"total": time.Since(start).Milliseconds(),
		},
		TLS: TLSMetadata{
			Mode: "in-process-loopback",
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
			repeatResp, err := runSingleProbeH3Request(path, cfg.Timeout)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("streams") {
		plan.executed = append(plan.executed, "streams")
		enrichStreamAnalysis(result, cfg, func() (int, int64, error) {
			repeatResp, err := runSingleProbeH3Request(path, cfg.Timeout)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("loss") {
		plan.executed = append(plan.executed, "loss")
		result.Analysis["loss"] = estimateLossAnalysis(cfg, func() (int, int64, error) {
			repeatResp, err := runSingleProbeH3Request(path, cfg.Timeout)
			if err != nil {
				return 0, 0, err
			}
			return repeatResp.StatusCode, int64(len(repeatResp.Body)), nil
		})
	}
	if plan.shouldRun("congestion") {
		plan.executed = append(plan.executed, "congestion")
		result.Analysis["congestion"] = estimateCongestionAnalysis(cfg, func() (int, int64, error) {
			repeatResp, err := runSingleProbeH3Request(path, cfg.Timeout)
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
			repeatResp, err := runSingleProbeH3Request(path, cfg.Timeout)
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
		plan.executed = append(plan.executed, "migration")
		result.Analysis["migration"] = measureLoopbackMigration(cfg)
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
		ID:        runid.New("pr"),
		Target:    parsed.String(),
		Timestamp: time.Now().UTC(),
		Duration:  time.Since(start),
		Status:    resp.StatusCode,
		Proto:     "HTTP/3-triton",
		Headers:   headers,
		Timings: map[string]int64{
			"total": time.Since(start).Milliseconds(),
		},
		TLS: TLSMetadata{
			Mode: "experimental-udp-h3",
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
		plan.executed = append(plan.executed, "migration")
		result.Analysis["migration"] = measureRemoteTritonMigration(parsed.Host, cfg)
	}
	finalizeTestPlan(result, plan)
	return result, nil
}

func runSingleProbeH3Request(path string, timeout time.Duration) (*h3.Response, error) {
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
	limited := &io.LimitedReader{R: resp.Body, N: maxProbeResponseBodyBytes + 1}
	body, err := io.ReadAll(limited)
	if err != nil {
		return 0, nil, err
	}
	if int64(len(body)) > maxProbeResponseBodyBytes {
		return resp.StatusCode, nil, fmt.Errorf("response body exceeds limit of %d bytes", maxProbeResponseBodyBytes)
	}
	return resp.StatusCode, body, nil
}

func measureH3Resumption(target string, cfg config.ProbeConfig) ZeroRTTAnalysis {
	cache := tls.NewLRUClientSessionCache(8)

	firstClient, firstTransport := realh3.NewClientWithSessionCache(cfg.Timeout, cfg.Insecure, "", cache)
	defer firstTransport.Close()
	firstStart := time.Now()
	firstResp, err := firstClient.Get(target)
	if err != nil {
		return ZeroRTTAnalysis{
			Supported:     false,
			Error:         err.Error(),
			Mode:          "tls-resumption-check",
			Requested0RTT: true,
		}
	}
	_, _ = io.Copy(io.Discard, firstResp.Body)
	if err := firstResp.Body.Close(); err != nil {
		return ZeroRTTAnalysis{
			Supported:     false,
			Error:         err.Error(),
			Mode:          "tls-resumption-check",
			Requested0RTT: true,
		}
	}
	firstDuration := time.Since(firstStart)
	firstResumed := firstResp.TLS != nil && firstResp.TLS.DidResume

	secondClient, secondTransport := realh3.NewClientWithSessionCache(cfg.Timeout, cfg.Insecure, "", cache)
	defer secondTransport.Close()
	secondStart := time.Now()
	secondResp, err := secondClient.Get(target)
	if err != nil {
		return ZeroRTTAnalysis{
			Supported:      false,
			InitialMS:      float64(firstDuration) / float64(time.Millisecond),
			InitialResumed: firstResumed,
			Error:          err.Error(),
			Mode:           "tls-resumption-check",
			Requested0RTT:  true,
		}
	}
	_, _ = io.Copy(io.Discard, secondResp.Body)
	if err := secondResp.Body.Close(); err != nil {
		return ZeroRTTAnalysis{
			Supported:      false,
			InitialMS:      float64(firstDuration) / float64(time.Millisecond),
			InitialResumed: firstResumed,
			Error:          err.Error(),
			Mode:           "tls-resumption-check",
			Requested0RTT:  true,
		}
	}
	secondDuration := time.Since(secondStart)
	secondResumed := secondResp.TLS != nil && secondResp.TLS.DidResume

	saved := firstDuration - secondDuration
	return ZeroRTTAnalysis{
		Supported:      secondResumed,
		Mode:           "tls-resumption-check",
		InitialMS:      float64(firstDuration) / float64(time.Millisecond),
		ResumedMS:      float64(secondDuration) / float64(time.Millisecond),
		InitialResumed: firstResumed,
		Resumed:        secondResumed,
		TimeSavedMS:    float64(saved) / float64(time.Millisecond),
		Requested0RTT:  true,
		Note:           "measures HTTP/3 connection resumption; true early data is not exposed at this layer",
	}
}

func measureHTTPMigration(target string, cfg config.ProbeConfig) MigrationAnalysis {
	parsed, err := url.Parse(target)
	if err != nil {
		return MigrationAnalysis{
			Supported:      false,
			Error:          err.Error(),
			Mode:           "endpoint-capability-check",
			RequestedCheck: true,
		}
	}
	parsed.Path = "/migration-test"
	parsed.RawQuery = ""

	client := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			ForceAttemptHTTP2: true,
			// #nosec G402 -- cfg.Insecure is gated by explicit allow_insecure_tls validation for lab use.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.Insecure, NextProtos: []string{"h2", "http/1.1"}},
			DialContext: (&net.Dialer{
				Timeout: cfg.Timeout,
			}).DialContext,
		},
	}
	status, body, err := doStandardRequestBody(client, parsed.String())
	result := MigrationAnalysis{
		Mode:           "endpoint-capability-check",
		Target:         parsed.String(),
		StatusClass:    status / 100,
		BodyBytes:      len(body),
		RequestedCheck: true,
		Note:           "checks the migration endpoint contract; it does not perform live path rebinding",
	}
	if err != nil {
		result.Supported = false
		result.Error = err.Error()
		return result
	}
	mergeMigrationContract(&result, body, status/100 == 2)
	return result
}

func measureLoopbackMigration(cfg config.ProbeConfig) MigrationAnalysis {
	start := time.Now()
	resp, err := runSingleProbeH3Request("/migration-test", cfg.Timeout)
	result := MigrationAnalysis{
		Mode:           "endpoint-capability-check",
		Target:         "triton://loopback/migration-test",
		RequestedCheck: true,
		Note:           "checks the migration endpoint contract; it does not perform live path rebinding",
	}
	if err != nil {
		result.Supported = false
		result.Error = err.Error()
		return result
	}
	result.StatusClass = resp.StatusCode / 100
	result.BodyBytes = len(resp.Body)
	result.DurationMS = float64(time.Since(start)) / float64(time.Millisecond)
	mergeMigrationContract(&result, resp.Body, resp.StatusCode/100 == 2)
	return result
}

func measureRemoteTritonMigration(address string, cfg config.ProbeConfig) MigrationAnalysis {
	start := time.Now()
	resp, err := h3.RoundTripAddress(address, http.MethodGet, "/migration-test", nil, cfg.Timeout)
	result := MigrationAnalysis{
		Mode:           "endpoint-capability-check",
		Target:         "triton://" + address + "/migration-test",
		RequestedCheck: true,
		Note:           "checks the migration endpoint contract; it does not perform live path rebinding",
	}
	if err != nil {
		result.Supported = false
		result.Error = err.Error()
		return result
	}
	result.StatusClass = resp.StatusCode / 100
	result.BodyBytes = len(resp.Body)
	result.DurationMS = float64(time.Since(start)) / float64(time.Millisecond)
	mergeMigrationContract(&result, resp.Body, resp.StatusCode/100 == 2)
	return result
}

func mergeMigrationContract(result *MigrationAnalysis, body []byte, fallbackSupported bool) {
	if result == nil {
		return
	}
	result.Supported = fallbackSupported
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
		result.Supported = *payload.Supported
	}
	if payload.Message != "" {
		result.Message = payload.Message
	}
}
