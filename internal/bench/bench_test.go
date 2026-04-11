package bench

import (
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
}

func TestRunProtocolLoopbackH3(t *testing.T) {
	stats, err := runProtocol("triton://loopback/ping", "h3", 200*time.Millisecond, 1, false, "")
	if err != nil {
		t.Fatalf("expected loopback h3 benchmark to succeed: %v", err)
	}
	if stats.Requests == 0 {
		t.Fatal("expected at least one h3 request")
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
	ok, err := observability.HasQLOGFiles(traceDir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected client qlog file")
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
		TraceDir:           traceDir,
	})
	if err != nil {
		t.Fatalf("expected bench run to succeed: %v", err)
	}
	if len(result.TraceFiles) == 0 {
		t.Fatal("expected trace files linked in bench result")
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
