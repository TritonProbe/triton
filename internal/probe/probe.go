package probe

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
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

func Run(target string, cfg config.ProbeConfig) (*Result, error) {
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
		return runRemoteTritonProbe(parsed, cfg)
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
		Analysis: analyzeHeaders(resp.Header),
	}
	if resp.TLS != nil {
		result.TLS = tlsMetadata(resp.TLS)
	}
	return result, nil
}

func runStandardH3Probe(parsed *url.URL, cfg config.ProbeConfig) (*Result, error) {
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
		Analysis: analyzeHeaders(resp.Header),
	}
	if resp.TLS != nil {
		result.TLS = tlsMetadata(resp.TLS)
	}
	after, err := observability.ListQLOGFiles(cfg.TraceDir)
	if err != nil {
		return nil, err
	}
	result.TraceFiles = observability.DiffQLOGFiles(before, after)
	return result, nil
}

func millisBetween(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func runLoopbackProbe(parsed *url.URL, cfg config.ProbeConfig) (*Result, error) {
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

	return &Result{
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
	}, nil
}

func runRemoteTritonProbe(parsed *url.URL, cfg config.ProbeConfig) (*Result, error) {
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

	return &Result{
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
	}, nil
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

func analyzeHeaders(headers http.Header) map[string]any {
	analysis := map[string]any{}
	altSvc := headers.Values("Alt-Svc")
	if len(altSvc) > 0 {
		analysis["alt_svc"] = map[string]any{
			"present": true,
			"values":  append([]string(nil), altSvc...),
		}
	}
	return analysis
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
