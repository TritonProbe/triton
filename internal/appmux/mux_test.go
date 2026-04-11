package appmux

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadRejectsWrongMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/upload", nil)
	rec := httptest.NewRecorder()

	New().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "POST" {
		t.Fatalf("unexpected Allow header: %q", got)
	}
}

func TestEchoRejectsOversizedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("toolarge"))
	rec := httptest.NewRecorder()

	NewWithOptions(Options{MaxBodyBytes: 4}).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
}

func TestHealthzEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	New().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	handler := New()

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/status/204", nil))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "triton_requests_total") {
		t.Fatalf("expected total requests metric, got %q", body)
	}
	if !strings.Contains(body, `triton_requests_by_route_total{route="/ping"} 1`) {
		t.Fatalf("expected ping route metric, got %q", body)
	}
	if !strings.Contains(body, `triton_responses_by_status_total{status="204"} 1`) {
		t.Fatalf("expected 204 status metric, got %q", body)
	}
}

func TestRootAndReadyEndpoints(t *testing.T) {
	for _, path := range []string{"/", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()

		New().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", path, rec.Code)
		}
	}
}

func TestDownloadAndUploadEndpoints(t *testing.T) {
	handler := NewWithOptions(Options{MaxBodyBytes: 32})

	req := httptest.NewRequest(http.MethodGet, "/download/5", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() != 5 {
		t.Fatalf("unexpected download response: code=%d len=%d", rec.Code, rec.Body.Len())
	}

	req = httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("hello"))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"bytes":5`) {
		t.Fatalf("unexpected upload response: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestRouteValidationErrors(t *testing.T) {
	handler := New()
	paths := []string{
		"/download/0",
		"/delay/-1",
		"/headers/-1",
		"/status/999",
		"/drip/1",
		"/drip/x/1",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", path, rec.Code)
		}
	}
}

func TestHeadersRedirectStreamsAndCapabilities(t *testing.T) {
	handler := New()

	req := httptest.NewRequest(http.MethodGet, "/headers/2", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("X-Triton-1") != "value-1" {
		t.Fatalf("unexpected headers response: code=%d headers=%v", rec.Code, rec.Header())
	}

	req = httptest.NewRequest(http.MethodGet, "/redirect/1", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/redirect/0" {
		t.Fatalf("unexpected redirect response: code=%d headers=%v", rec.Code, rec.Header())
	}

	req = httptest.NewRequest(http.MethodGet, "/streams/2", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"requested":2`) {
		t.Fatalf("unexpected streams response: code=%d body=%q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/.well-known/triton", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"protocols"`) {
		t.Fatalf("unexpected capabilities response: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestDripTLSQUICMigrationAndHelpers(t *testing.T) {
	handler := New()

	req := httptest.NewRequest(http.MethodGet, "/drip/3/0", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() != 3 {
		t.Fatalf("unexpected drip response: code=%d len=%d", rec.Code, rec.Body.Len())
	}

	req = httptest.NewRequest(http.MethodGet, "/tls-info", nil)
	req.TLS = &tls.ConnectionState{
		Version:            tls.VersionTLS13,
		CipherSuite:        tls.TLS_AES_128_GCM_SHA256,
		NegotiatedProtocol: "h2",
		ServerName:         "example.com",
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"TLS1.3"`) {
		t.Fatalf("unexpected tls-info response: code=%d body=%q", rec.Code, rec.Body.String())
	}

	for _, path := range []string{"/quic-info", "/migration-test"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		rec = httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", path, rec.Code)
		}
	}

	if got := tlsVersion(0x9999); got != "0x9999" {
		t.Fatalf("unexpected fallback tls version: %q", got)
	}
	if _, err := parseTailInt("/status/204", "/status/"); err != nil {
		t.Fatalf("parseTailInt returned error: %v", err)
	}
}

func TestHandleBodyReadErrorBadRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	handleBodyReadError(rec, io.ErrUnexpectedEOF)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMethodHandlerRejectsMethod(t *testing.T) {
	handler := methodHandler(func(http.ResponseWriter, *http.Request) {}, http.MethodGet, http.MethodPost)
	req := httptest.NewRequest(http.MethodDelete, "/echo", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, POST" {
		t.Fatalf("unexpected method rejection: code=%d headers=%v", rec.Code, rec.Header())
	}
}

func TestReadLimitedBodySuccess(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("ok"))
	rec := httptest.NewRecorder()
	body, err := readLimitedBody(rec, req, 8)
	if err != nil {
		t.Fatalf("readLimitedBody returned error: %v", err)
	}
	if body != "ok" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusCreated, map[string]string{"ok": "true"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("unexpected content type: %q", got)
	}
}

func TestHandleBodyReadErrorMaxBytes(t *testing.T) {
	rec := httptest.NewRecorder()
	handleBodyReadError(rec, &http.MaxBytesError{Limit: 1})
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
}

func TestHandleBodyReadErrorUnknown(t *testing.T) {
	rec := httptest.NewRecorder()
	handleBodyReadError(rec, errors.New("boom"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
