package realh3

import (
	"crypto/tls"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	client, transport := NewClient(3*time.Second, true, "")
	defer transport.Close()

	if client.Timeout != 3*time.Second {
		t.Fatalf("unexpected client timeout: %v", client.Timeout)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("expected TLS client config")
	}
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected TLS 1.3 minimum, got %v", transport.TLSClientConfig.MinVersion)
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("expected insecure skip verify to be enabled")
	}
	if transport.QUICConfig == nil {
		t.Fatal("expected QUIC config")
	}
	if transport.QUICConfig.Tracer != nil {
		t.Fatal("expected nil tracer when traceDir is empty")
	}
}

func TestNewClientWithTraceDir(t *testing.T) {
	client, transport := NewClient(2*time.Second, false, t.TempDir())
	defer transport.Close()

	if client.Timeout != 2*time.Second {
		t.Fatalf("unexpected client timeout: %v", client.Timeout)
	}
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("unexpected TLS config: %+v", transport.TLSClientConfig)
	}
	if transport.QUICConfig == nil || transport.QUICConfig.Tracer == nil {
		t.Fatal("expected qlog tracer when traceDir is set")
	}
}

func TestNewClientWithSessionCache(t *testing.T) {
	cache := tls.NewLRUClientSessionCache(8)
	client, transport := NewClientWithSessionCache(2*time.Second, false, "", cache)
	defer transport.Close()

	if client.Timeout != 2*time.Second {
		t.Fatalf("unexpected client timeout: %v", client.Timeout)
	}
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.ClientSessionCache == nil {
		t.Fatalf("expected TLS session cache to be configured: %+v", transport.TLSClientConfig)
	}
}
