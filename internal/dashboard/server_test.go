package dashboard

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/storage"
)

func TestDashboardBasicAuth(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := New("127.0.0.1:0", store, Options{
		Username: "admin",
		Password: "secret",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.SetBasicAuth("admin", "secret")
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected security headers, got %q", got)
	}
	if got := rec.Header().Get(observability.RequestIDHeader); got == "" {
		t.Fatal("expected request id header")
	}
}

func TestDashboardStatusRejectsWrongMethod(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := New("127.0.0.1:0", store, Options{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestDashboardListsAndServesTraces(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	traceDir := t.TempDir()
	traceFile := filepath.Join(traceDir, "abc_client.sqlog")
	if err := os.WriteFile(traceFile, []byte("trace-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := New("127.0.0.1:0", store, Options{TraceDir: traceDir})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "abc_client.sqlog") {
		t.Fatalf("expected trace listing, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/traces/abc_client.sqlog", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != "trace-data" {
		t.Fatalf("unexpected trace body: %q", got)
	}
}

func TestDashboardAssetsAndHead(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := New("127.0.0.1:0", store, Options{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<!doctype html>") {
		t.Fatalf("unexpected root asset response: code=%d body=%q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodHead, "/assets/app.js", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for HEAD asset, got %d", rec.Code)
	}
}

func TestDashboardTraceErrorsAndNotFound(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := New("127.0.0.1:0", store, Options{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traces/missing.sqlog", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 without trace dir, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/traces/not-a-trace.txt", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid extension, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown route, got %d", rec.Code)
	}
}

func TestDashboardProbeAndBenchDetailsNotFound(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := New("127.0.0.1:0", store, Options{})

	for _, path := range []string{"/api/v1/probes/missing", "/api/v1/benches/missing"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.http.Handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 for %s, got %d", path, rec.Code)
		}
	}
}
