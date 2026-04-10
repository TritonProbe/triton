package probe

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"time"

	"github.com/tritonprobe/triton/internal/config"
)

type Result struct {
	ID        string           `json:"id" yaml:"id"`
	Target    string           `json:"target" yaml:"target"`
	Timestamp time.Time        `json:"timestamp" yaml:"timestamp"`
	Duration  time.Duration    `json:"duration" yaml:"duration"`
	Status    int              `json:"status" yaml:"status"`
	Proto     string           `json:"proto" yaml:"proto"`
	Timings   map[string]int64 `json:"timings_ms" yaml:"timings_ms"`
	TLS       map[string]any   `json:"tls" yaml:"tls"`
	Headers   http.Header      `json:"headers" yaml:"headers"`
}

func Run(target string, cfg config.ProbeConfig) (*Result, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
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
		TLS: map[string]any{},
	}
	if resp.TLS != nil {
		result.TLS = map[string]any{
			"version": resp.TLS.Version,
			"cipher":  tls.CipherSuiteName(resp.TLS.CipherSuite),
			"alpn":    resp.TLS.NegotiatedProtocol,
		}
	}
	return result, nil
}

func millisBetween(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	return end.Sub(start).Milliseconds()
}
