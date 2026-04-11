package probe

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/testutil"
)

func TestRunHTTPSProbeRespectsTLSVerification(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	_, err := Run(srv.URL, config.ProbeConfig{Timeout: 2 * time.Second})
	if err == nil {
		t.Fatal("expected certificate verification failure")
	}

	result, err := Run(srv.URL, config.ProbeConfig{
		Timeout:  2 * time.Second,
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("expected insecure probe to succeed: %v", err)
	}
	if result.Status != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", result.Status)
	}
	if result.TLS["cipher"] == "" {
		t.Fatalf("expected TLS metadata, got %#v", result.TLS)
	}
}

func TestRunLoopbackProbe(t *testing.T) {
	result, err := Run("triton://loopback/ping", config.ProbeConfig{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("unexpected status: %d", result.Status)
	}
	if result.Proto != "HTTP/3-loopback" {
		t.Fatalf("unexpected proto: %q", result.Proto)
	}
}

func TestRunStandardH3Probe(t *testing.T) {
	addr, shutdown := testutil.StartHTTP3Server(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Test", "h3")
		w.WriteHeader(http.StatusCreated)
	}))
	defer shutdown()
	traceDir := t.TempDir()

	_, err := Run("h3://"+addr+"/ping", config.ProbeConfig{Timeout: 2 * time.Second})
	if err == nil {
		t.Fatal("expected certificate verification failure")
	}

	result, err := Run("h3://"+addr+"/ping", config.ProbeConfig{
		Timeout:  2 * time.Second,
		Insecure: true,
		TraceDir: traceDir,
	})
	if err != nil {
		t.Fatalf("expected h3 probe to succeed: %v", err)
	}
	if result.Status != http.StatusCreated {
		t.Fatalf("unexpected status: %d", result.Status)
	}
	if result.Proto == "" {
		t.Fatal("expected protocol string")
	}
	if result.TLS["alpn"] != "h3" {
		t.Fatalf("expected h3 ALPN, got %#v", result.TLS)
	}
	if len(result.TraceFiles) == 0 {
		t.Fatal("expected trace files to be linked in result")
	}
	ok, err := observability.HasQLOGFiles(traceDir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected client qlog file")
	}
}
