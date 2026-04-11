package h3

import (
	"net/http"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/appmux"
	"github.com/tritonprobe/triton/internal/quic/transport"
)

func TestRoundTripAddress(t *testing.T) {
	listener, err := transport.ListenQUIC("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	listener.SetAutoEcho(false)

	server := NewUDPServer(listener, appmux.New())
	done := make(chan error, 1)
	go func() {
		done <- server.Serve()
	}()

	resp, err := RoundTripAddress(listener.Addr().String(), http.MethodGet, "/ping", nil, 2*time.Second)
	if err != nil {
		t.Fatalf("expected UDP h3 round trip to succeed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	if string(resp.Body) != "pong" {
		t.Fatalf("unexpected body: %q", string(resp.Body))
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
