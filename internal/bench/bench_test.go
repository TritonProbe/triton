package bench

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/appmux"
	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/h3"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/quic/transport"
	"github.com/tritonprobe/triton/internal/testutil"
)

func TestRunProtocolTLSVerification(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	if _, err := runProtocol(srv.URL, "h1", 150*time.Millisecond, 1, false, ""); err == nil {
		t.Fatal("expected TLS verification failure")
	}

	stats, err := runProtocol(srv.URL, "h1", 150*time.Millisecond, 1, true, "")
	if err != nil {
		t.Fatalf("expected insecure benchmark to succeed: %v", err)
	}
	if stats.Requests == 0 {
		t.Fatal("expected at least one successful request")
	}
	if stats.Latency.P95 <= 0 {
		t.Fatalf("expected latency percentiles to be populated: %+v", stats.Latency)
	}
	if stats.Phases.FirstByteMS <= 0 {
		t.Fatalf("expected first-byte phase timing: %+v", stats.Phases)
	}
}

func TestRunProtocolLoopbackH3(t *testing.T) {
	stats, err := runProtocol("triton://loopback/ping", "h3", 200*time.Millisecond, 1, false, "")
	if err != nil {
		t.Fatalf("expected loopback h3 benchmark to succeed: %v", err)
	}
	if stats.Requests == 0 {
		t.Fatal("expected at least one h3 request")
	}
	if stats.SampledPoints == 0 {
		t.Fatalf("expected latency percentiles to be populated: %+v", stats.Latency)
	}
	if stats.Latency.P99 < stats.Latency.P50 {
		t.Fatalf("expected percentile ordering: %+v", stats.Latency)
	}
}

func TestRunProtocolHTTPSH3(t *testing.T) {
	addr, shutdown := testutil.StartHTTP3Server(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer shutdown()
	traceDir := t.TempDir()

	if _, err := runProtocol("https://"+addr, "h3", 150*time.Millisecond, 1, false, traceDir); err == nil {
		t.Fatal("expected TLS verification failure for h3")
	}

	stats, err := runProtocol("https://"+addr, "h3", 200*time.Millisecond, 1, true, traceDir)
	if err != nil {
		t.Fatalf("expected https h3 benchmark to succeed: %v", err)
	}
	if stats.Requests == 0 {
		t.Fatal("expected at least one real h3 request")
	}
	if stats.Phases.FirstByteMS <= 0 {
		t.Fatalf("expected phase timings for h3 benchmark: %+v", stats.Phases)
	}
	ok, err := observability.HasQLOGFiles(traceDir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected client qlog file")
	}
}

func TestRunProtocolH3SchemeTarget(t *testing.T) {
	addr, shutdown := testutil.StartHTTP3Server(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer shutdown()
	traceDir := t.TempDir()

	stats, err := runProtocol("h3://"+addr, "h3", 200*time.Millisecond, 1, true, traceDir)
	if err != nil {
		t.Fatalf("expected h3 scheme benchmark to succeed: %v", err)
	}
	if stats.Requests == 0 {
		t.Fatal("expected at least one real h3 request")
	}
}

func TestRunProtocolHTTPSH3ViaAltSvc(t *testing.T) {
	h3Addr, shutdownH3 := testutil.StartHTTP3Server(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer shutdownH3()

	tcpSrv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Alt-Svc", `h3="`+h3Addr+`"; ma=2592000`)
		_, _ = w.Write([]byte("ok"))
	}))
	tcpSrv.TLS = &tls.Config{NextProtos: []string{"h2", "http/1.1"}}
	tcpSrv.StartTLS()
	defer tcpSrv.Close()

	traceDir := t.TempDir()
	stats, err := runProtocol(tcpSrv.URL, "h3", 200*time.Millisecond, 1, true, traceDir)
	if err != nil {
		t.Fatalf("expected https h3 benchmark via Alt-Svc to succeed: %v", err)
	}
	if stats.Requests == 0 {
		t.Fatal("expected at least one real h3 request")
	}
}

func TestRunBenchLinksTraceFiles(t *testing.T) {
	addr, shutdown := testutil.StartHTTP3Server(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer shutdown()
	traceDir := t.TempDir()

	result, err := Run("https://"+addr, config.BenchConfig{
		DefaultDuration:    200 * time.Millisecond,
		DefaultConcurrency: 1,
		DefaultProtocols:   []string{"h3"},
		Insecure:           true,
		AllowInsecureTLS:   true,
		TraceDir:           traceDir,
	})
	if err != nil {
		t.Fatalf("expected bench run to succeed: %v", err)
	}
	if len(result.TraceFiles) == 0 {
		t.Fatal("expected trace files linked in bench result")
	}
	if result.Stats["h3"].Latency.P99 <= 0 {
		t.Fatalf("expected bench result to include latency percentiles: %+v", result.Stats["h3"])
	}
	if result.Summary.Protocols != 1 || result.Summary.BestProtocol != "h3" {
		t.Fatalf("expected bench summary to be populated: %#v", result.Summary)
	}
}

func TestRunBenchKeepsPartialProtocolFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	result, err := Run(srv.URL, config.BenchConfig{
		DefaultDuration:    100 * time.Millisecond,
		DefaultConcurrency: 1,
		DefaultProtocols:   []string{"h1", "h3"},
	})
	if err != nil {
		t.Fatalf("expected partial bench run to return result: %v", err)
	}
	if result.Stats["h1"].Requests == 0 {
		t.Fatalf("expected h1 stats to succeed: %+v", result.Stats["h1"])
	}
	if result.Stats["h3"].Errors == 0 || result.Stats["h3"].ErrorRate != 1 {
		t.Fatalf("expected h3 failure stats to be preserved: %+v", result.Stats["h3"])
	}
	if result.Summary.FailedProtocols != 1 || result.Summary.HealthyProtocols != 1 {
		t.Fatalf("unexpected summary for partial failure: %#v", result.Summary)
	}
}

func TestRunBenchRejectsInsecureTLSWithoutOptIn(t *testing.T) {
	if _, err := Run("https://example.com", config.BenchConfig{
		DefaultDuration:    time.Second,
		DefaultConcurrency: 1,
		DefaultProtocols:   []string{"h1"},
		Insecure:           true,
	}); err == nil {
		t.Fatal("expected insecure bench without opt-in to fail")
	}
}

func TestRunProtocolRemoteTritonH3(t *testing.T) {
	listener, err := transport.ListenQUIC("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener.SetAutoEcho(false)

	server := h3.NewUDPServer(listener, appmux.New())
	done := make(chan error, 1)
	go func() {
		done <- server.Serve()
	}()

	stats, err := runProtocol("triton://"+listener.Addr().String()+"/ping", "h3", 200*time.Millisecond, 1, false, "")
	if err != nil {
		t.Fatalf("expected remote triton h3 benchmark to succeed: %v", err)
	}
	if stats.Requests == 0 {
		t.Fatal("expected at least one h3 request")
	}
	if stats.SampledPoints == 0 {
		t.Fatalf("expected sampled points for remote triton benchmark: %+v", stats)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected serve error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server shutdown")
	}
}

func TestRunProtocolSummarizesErrors(t *testing.T) {
	stats, err := runProtocol("https://127.0.0.1:1", "h1", 100*time.Millisecond, 1, true, "")
	if err == nil {
		t.Fatal("expected benchmark to fail when all requests error")
	}
	if stats.Errors == 0 {
		t.Fatalf("expected errors to be counted: %+v", stats)
	}
	if len(stats.ErrorSummary) == 0 {
		t.Fatalf("expected error summary to be populated: %+v", stats)
	}
}

func TestBuildSummaryClassifiesProtocols(t *testing.T) {
	summary := buildSummary(map[string]Stats{
		"h1": {Requests: 10, Errors: 0, RequestsPerS: 20, ErrorRate: 0.0},
		"h2": {Requests: 10, Errors: 2, RequestsPerS: 18, ErrorRate: 0.2},
		"h3": {Requests: 0, Errors: 5, RequestsPerS: 0, ErrorRate: 1.0},
	})
	if summary.HealthyProtocols != 1 || summary.DegradedProtocols != 1 || summary.FailedProtocols != 1 {
		t.Fatalf("unexpected bench summary classification: %#v", summary)
	}
	if summary.BestProtocol != "h1" || summary.RiskiestProtocol != "h3" {
		t.Fatalf("unexpected bench summary ranking: %#v", summary)
	}
}

func TestExtractH3Authority(t *testing.T) {
	got := extractH3Authority([]string{`h2=":443"; ma=2592000, h3=":15443"; ma=2592000`})
	if got != ":15443" {
		t.Fatalf("unexpected authority: %q", got)
	}
}

func TestNormalizeHTTP3BenchmarkTarget(t *testing.T) {
	got, err := normalizeHTTP3BenchmarkTarget("h3://example.com/ping")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://example.com/ping" {
		t.Fatalf("unexpected normalized target: %q", got)
	}
}
