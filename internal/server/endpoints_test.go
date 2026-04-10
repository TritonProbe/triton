package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
