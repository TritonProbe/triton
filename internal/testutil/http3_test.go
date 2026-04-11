package testutil

import (
	"crypto/tls"
	"net/http"
	"os"
	"testing"
)

func TestGenerateSelfSignedCertFiles(t *testing.T) {
	certFile, keyFile := GenerateSelfSignedCertFiles(t)
	if _, err := os.Stat(certFile); err != nil {
		t.Fatalf("expected cert file: %v", err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		t.Fatalf("expected key file: %v", err)
	}
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		t.Fatalf("expected valid key pair: %v", err)
	}
}

func TestStartHTTP3Server(t *testing.T) {
	addr, shutdown := StartHTTP3Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer shutdown()

	if addr == "" {
		t.Fatal("expected server address")
	}
}
