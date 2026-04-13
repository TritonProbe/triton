package probe

import (
	"net/http"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/appmux"
	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/h3"
	"github.com/tritonprobe/triton/internal/quic/transport"
)

func TestRunRemoteTritonProbe(t *testing.T) {
	listener, err := transport.ListenQUIC("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener.SetAutoEcho(false)

	server := h3.NewUDPServer(listener, appmux.New())
	done := make(chan error, 1)
	go func() {
		done <- server.Serve()
	}()

	result, err := Run("triton://"+listener.Addr().String()+"/ping", config.ProbeConfig{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("expected remote triton probe to succeed: %v", err)
	}
	if result.Status != http.StatusOK {
		t.Fatalf("unexpected status: %d", result.Status)
	}
	if result.Proto != "HTTP/3-triton" {
		t.Fatalf("unexpected proto: %q", result.Proto)
	}
	tlsMeta, ok := result.TLS.(TLSMetadata)
	if !ok {
		t.Fatalf("expected typed TLS metadata, got %#v", result.TLS)
	}
	if tlsMeta.Mode != "experimental-udp-h3" {
		t.Fatalf("unexpected TLS mode: %#v", result.TLS)
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected serve error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for server shutdown")
	}
}
