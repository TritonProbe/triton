package observability

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Header() http.Header        { return make(http.Header) }
func (failingWriter) WriteHeader(statusCode int) {}
func (failingWriter) Write([]byte) (int, error)  { return 0, context.Canceled }

func TestWithRequestIDGeneratesAndExposesHeader(t *testing.T) {
	var seen string
	handler := WithRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if seen == "" {
		t.Fatal("expected request id in context")
	}
	if got := rec.Header().Get(RequestIDHeader); got != seen {
		t.Fatalf("expected response header %q to match context %q", got, seen)
	}
}

func TestWithRequestIDHonorsIncomingHeader(t *testing.T) {
	handler := WithRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestIDFromContext(r.Context()); got != "abc-123" {
			t.Fatalf("unexpected request id in context: %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(RequestIDHeader, "abc-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get(RequestIDHeader); got != "abc-123" {
		t.Fatalf("expected response header to preserve incoming id, got %q", got)
	}
}

func TestWithAccessLogIncludesRequestID(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	handler := WithAccessLog(logger, "test", WithRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if RequestIDFromContext(r.Context()) == "" {
			t.Fatal("expected request id in context")
		}
		_, _ = w.Write([]byte("ok"))
	})))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil).WithContext(context.Background())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := logBuf.String()
	if !strings.Contains(body, `"component":"test"`) {
		t.Fatalf("expected component in log, got %q", body)
	}
	if !strings.Contains(body, `"request_id":"`) {
		t.Fatalf("expected request id in log, got %q", body)
	}
	if !strings.Contains(body, `"status":200`) {
		t.Fatalf("expected status in log, got %q", body)
	}
}

func TestRequestIDFromContextMissingAndRecorderWrite(t *testing.T) {
	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Fatalf("expected empty request id, got %q", got)
	}

	rec := &responseRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	if _, err := rec.Write([]byte("abc")); err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if rec.bytes != 3 {
		t.Fatalf("expected 3 written bytes, got %d", rec.bytes)
	}
}

func TestWithAccessLogNilLoggerAndWriteError(t *testing.T) {
	handler := WithAccessLog(nil, "test", WithRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ignored"))
	})))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(failingWriter{}, req)
}
