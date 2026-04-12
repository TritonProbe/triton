package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterBlocksAfterLimit(t *testing.T) {
	limiter := newRateLimiter(1)
	handler := limiter.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected first request to pass, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request to be limited, got %d", rec.Code)
	}
}

func TestRateLimiterSweepsIdleBuckets(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	current := base

	limiter := newRateLimiter(10)
	limiter.now = func() time.Time { return current }
	limiter.sweepEvery = 1
	limiter.maxIdle = time.Minute

	handler := limiter.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	makeRequest := func(remoteAddr string) {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected request from %s to pass, got %d", remoteAddr, rec.Code)
		}
	}

	makeRequest("127.0.0.1:1000")
	if got := len(limiter.buckets); got != 1 {
		t.Fatalf("expected one bucket after first request, got %d", got)
	}

	current = current.Add(2 * time.Minute)
	makeRequest("127.0.0.2:1000")

	if got := len(limiter.buckets); got != 1 {
		t.Fatalf("expected stale bucket to be swept, got %d buckets", got)
	}
	if _, ok := limiter.buckets["127.0.0.1"]; ok {
		t.Fatal("expected stale bucket to be removed")
	}
}
