package dashboard

import (
	"encoding/json"
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
	if !strings.Contains(rec.Body.String(), `"status":"error"`) {
		t.Fatalf("expected structured JSON auth error, got %q", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.SetBasicAuth("admin", "secret")
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected JSON content type, got %q", got)
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
	if !strings.Contains(rec.Body.String(), `"status":"error"`) {
		t.Fatalf("expected structured JSON error, got %q", rec.Body.String())
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from status, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("expected valid JSON status payload: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("expected status ok, got %#v", payload["status"])
	}
	storagePayload, ok := payload["storage"].(map[string]any)
	if !ok {
		t.Fatalf("expected storage object in status payload, got %#v", payload["storage"])
	}
	if storagePayload["traces"] != float64(1) {
		t.Fatalf("expected one trace in status payload, got %#v", storagePayload["traces"])
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/traces", nil)
	rec = httptest.NewRecorder()
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

	req = httptest.NewRequest(http.MethodGet, "/api/v1/traces/meta/abc_client.sqlog", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from trace metadata, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"preview":"trace-data"`) {
		t.Fatalf("expected preview in trace metadata, got %q", rec.Body.String())
	}
}

func TestDashboardConfigEndpoint(t *testing.T) {
	store, err := storage.NewFileStore(t.TempDir(), 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	srv := New("127.0.0.1:0", store, Options{
		Config: map[string]any{
			"dashboard": map[string]any{
				"enabled":      true,
				"auth_enabled": true,
			},
			"listeners": map[string]any{
				"dashboard": "127.0.0.1:9090",
			},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from config, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("config endpoint should not leak secrets, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"auth_enabled":true`) {
		t.Fatalf("expected sanitized config payload, got %q", rec.Body.String())
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

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `id="config"`) {
		t.Fatalf("expected config card in dashboard html, got %q", rec.Body.String())
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
	if !strings.Contains(rec.Body.String(), `"status":"error"`) {
		t.Fatalf("expected JSON trace error, got %q", rec.Body.String())
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

	req = httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil)
	rec = httptest.NewRecorder()
	srv.http.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown api route, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"api route not found"`) {
		t.Fatalf("expected API not found payload, got %q", rec.Body.String())
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
		if !strings.Contains(rec.Body.String(), `"status":"error"`) {
			t.Fatalf("expected JSON error for %s, got %q", path, rec.Body.String())
		}
	}
}
