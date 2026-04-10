package h3

import (
	"net/http"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/quic/transport"
)

func TestRoundTripHandlerLoopback(t *testing.T) {
	listener, err := transport.ListenQUIC("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	listener.SetAutoEcho(false)

	dialer := transport.NewDialer(2 * time.Second)
	session, err := dialer.DialSession(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	serverConn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Method", r.Method)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("handled:" + r.URL.Path))
	})

	service := NewServer(listener, serverConn, handler)
	client := NewClient(session)
	resp, err := service.ServeRoundTrip(client, "POST", "/hello", []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	if resp.Headers["X-Method"] != "POST" {
		t.Fatalf("unexpected response headers: %#v", resp.Headers)
	}
	if got := string(resp.Body); got != "handled:/hello" {
		t.Fatalf("unexpected body: %q", got)
	}
}
