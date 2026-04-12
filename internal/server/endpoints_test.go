package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	quichttp3 "github.com/quic-go/quic-go/http3"

	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/h3"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/probe"
	"github.com/tritonprobe/triton/internal/storage"
)

func TestPingHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	NewMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "pong" {
		t.Fatalf("unexpected body %q", rec.Body.String())
	}
}

func TestCapabilitiesHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/.well-known/triton", nil)
	rec := httptest.NewRecorder()

	NewMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["name"] != "triton" {
		t.Fatalf("unexpected name: %#v", payload["name"])
	}
}

func TestH3HandlerIncludesMiddlewareAndRateLimit(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	srv, err := New(config.ServerConfig{
		Listen:              "127.0.0.1:0",
		AllowExperimentalH3: true,
		ListenTCP:           "127.0.0.1:0",
		ReadTimeout:         2 * time.Second,
		WriteTimeout:        2 * time.Second,
		IdleTimeout:         2 * time.Second,
		RateLimit:           1,
		MaxBodyBytes:        1 << 20,
		Dashboard:           false,
		DashboardPass:       "",
		DashboardUser:       "",
	}, t.TempDir(), store)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- srv.h3.Serve()
	}()

	resp, err := h3.RoundTripAddress(srv.udp.Addr().String(), http.MethodGet, "/ping", nil, 2*time.Second)
	if err != nil {
		t.Fatalf("expected h3 request to succeed: %v", err)
	}
	if resp.Headers[observability.RequestIDHeader] == "" {
		t.Fatalf("expected %s header in h3 response", observability.RequestIDHeader)
	}
	if got := resp.Headers["X-Content-Type-Options"]; got != "nosniff" {
		t.Fatalf("expected security headers on h3 response, got %q", got)
	}

	resp, err = h3.RoundTripAddress(srv.udp.Addr().String(), http.MethodGet, "/ping", nil, 2*time.Second)
	if err != nil {
		t.Fatalf("expected second h3 request to succeed at transport level: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected h3 rate limit response, got %d", resp.StatusCode)
	}

	if err := srv.udp.Close(); err != nil {
		t.Fatalf("close udp listener: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected h3 serve error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for h3 shutdown")
	}
}

func TestRealHTTP3ServerSupportsProbe(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	h3Addr := pc.LocalAddr().String()
	_ = pc.Close()

	srv, err := New(config.ServerConfig{
		Listen:              "127.0.0.1:0",
		AllowExperimentalH3: true,
		ListenH3:            h3Addr,
		ListenTCP:           "127.0.0.1:0",
		ReadTimeout:         2 * time.Second,
		WriteTimeout:        2 * time.Second,
		IdleTimeout:         2 * time.Second,
		MaxBodyBytes:        1 << 20,
		TraceDir:            t.TempDir(),
		Dashboard:           false,
	}, t.TempDir(), store)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- srv.h3real.ListenAndServeTLS(srv.cfg.CertFile, srv.cfg.KeyFile)
	}()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.h3real.Shutdown(ctx)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for real h3 shutdown")
		}
	}()

	var result *probe.Result
	for i := 0; i < 20; i++ {
		result, err = probe.Run("h3://"+h3Addr+"/ping", config.ProbeConfig{
			Timeout:  2 * time.Second,
			Insecure: true,
		})
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("expected real h3 probe to succeed: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("unexpected status: %d", result.Status)
	}
	if result.TLS["alpn"] != "h3" {
		t.Fatalf("expected h3 ALPN, got %#v", result.TLS)
	}
	ok, err := observability.HasQLOGFiles(srv.cfg.TraceDir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected qlog trace file")
	}
}

func TestEnsureCertificateAndHelpers(t *testing.T) {
	dir := t.TempDir()
	cfg := config.ServerConfig{}
	certFile, keyFile, err := ensureCertificate(cfg, dir)
	if err != nil {
		t.Fatalf("ensureCertificate returned error: %v", err)
	}
	if certFile == "" || keyFile == "" {
		t.Fatalf("expected generated cert paths, got cert=%q key=%q", certFile, keyFile)
	}

	certFile2, keyFile2, err := ensureCertificate(config.ServerConfig{}, dir)
	if err != nil {
		t.Fatalf("second ensureCertificate returned error: %v", err)
	}
	if certFile2 != certFile || keyFile2 != keyFile {
		t.Fatalf("expected generated certs to be reused")
	}

	certFile3, keyFile3, err := ensureCertificate(config.ServerConfig{CertFile: "a.pem", KeyFile: "b.pem"}, dir)
	if err != nil {
		t.Fatalf("explicit ensureCertificate returned error: %v", err)
	}
	if certFile3 != "a.pem" || keyFile3 != "b.pem" {
		t.Fatalf("expected explicit paths to be preserved, got %q %q", certFile3, keyFile3)
	}
}

func TestBuildHandlerAndMiddlewareHelpers(t *testing.T) {
	logger, err := observability.NewLogger("")
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	handler := buildHandler(config.ServerConfig{
		MaxBodyBytes: 16,
		RateLimit:    1,
	}, logger.Logger)

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get(observability.RequestIDHeader); got == "" {
		t.Fatal("expected request id header")
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected security headers, got %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected rate-limited response, got %d", rec.Code)
	}

	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	rec = httptest.NewRecorder()
	withSecurityHeaders(base).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("expected CSP header")
	}

	rec = httptest.NewRecorder()
	withAltSvc(base, nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected passthrough without h3 server, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	withAltSvc(base, &quichttp3.Server{Addr: "127.0.0.1:4444"}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected passthrough with h3 server, got %d", rec.Code)
	}
}

func TestActiveListenersSummary(t *testing.T) {
	summary := activeListenersSummary(config.ServerConfig{
		ListenTCP:           "127.0.0.1:8443",
		ListenH3:            "127.0.0.1:4434",
		Listen:              "127.0.0.1:4433",
		AllowExperimentalH3: true,
		Dashboard:           true,
		DashboardListen:     "127.0.0.1:9090",
	})
	if len(summary) != 4 {
		t.Fatalf("expected four listener summary entries, got %v", summary)
	}
	if summary[0] != "https/tcp=127.0.0.1:8443" {
		t.Fatalf("unexpected https summary: %q", summary[0])
	}
	if summary[1] != "http3/quic=127.0.0.1:4434" {
		t.Fatalf("unexpected h3 summary: %q", summary[1])
	}
	if summary[2] != "experimental-h3/udp=127.0.0.1:4433" {
		t.Fatalf("unexpected experimental summary: %q", summary[2])
	}
	if summary[3] != "dashboard=127.0.0.1:9090" {
		t.Fatalf("unexpected dashboard summary: %q", summary[3])
	}
}

func TestDashboardConfigSnapshotRedactsSecrets(t *testing.T) {
	snapshot := dashboardConfigSnapshot(config.ServerConfig{
		ListenTCP:            "127.0.0.1:8443",
		ListenH3:             "127.0.0.1:4434",
		Listen:               "127.0.0.1:4433",
		AllowExperimentalH3:  true,
		Dashboard:            true,
		DashboardListen:      "127.0.0.1:9090",
		AllowRemoteDashboard: true,
		DashboardUser:        "admin",
		DashboardPass:        "secret",
		MaxBodyBytes:         1024,
		RateLimit:            5,
		ReadTimeout:          time.Second,
		WriteTimeout:         2 * time.Second,
		IdleTimeout:          3 * time.Second,
		TraceDir:             "traces",
		AccessLog:            "access.log",
		CertFile:             "cert.pem",
		KeyFile:              "key.pem",
	})

	dashboardCfg := snapshot["dashboard"].(map[string]any)
	if dashboardCfg["auth_enabled"] != true {
		t.Fatalf("expected auth_enabled=true, got %#v", dashboardCfg)
	}
	if _, ok := dashboardCfg["password"]; ok {
		t.Fatal("dashboard snapshot must not expose passwords")
	}
}

func TestCapabilityHelpersReflectConfig(t *testing.T) {
	cfg := config.ServerConfig{
		ListenH3:            "127.0.0.1:4434",
		Listen:              "127.0.0.1:4433",
		AllowExperimentalH3: true,
		Dashboard:           true,
		TraceDir:            "traces",
	}

	protocols := supportedProtocols(cfg)
	if len(protocols) != 4 || protocols[2] != "h3" || protocols[3] != "triton-h3" {
		t.Fatalf("unexpected supported protocols: %v", protocols)
	}

	flags := capabilityFlags(cfg)
	if len(flags) == 0 {
		t.Fatal("expected capability flags")
	}
	if flags[len(flags)-1] != "qlog" {
		t.Fatalf("expected qlog capability, got %v", flags)
	}

	experimental := experimentalFeatures(cfg)
	if len(experimental) != 1 || experimental[0] != "triton-udp-h3" {
		t.Fatalf("unexpected experimental features: %v", experimental)
	}

	if profile := deploymentProfile(cfg); profile != "mixed" {
		t.Fatalf("unexpected deployment profile: %q", profile)
	}
	if stability := stabilityLevel(cfg); stability != "mixed-stability" {
		t.Fatalf("unexpected stability level: %q", stability)
	}
}
