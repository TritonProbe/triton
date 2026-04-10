package h3

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/quic/transport"
	tritonserver "github.com/tritonprobe/triton/internal/server"
)

func TestTritonMuxOverH3LoopbackPing(t *testing.T) {
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

	resp, err := RoundTripHandlerLoopback(listener, session, serverConn, tritonserver.NewMux(), http.MethodGet, "/ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	if got := string(resp.Body); got != "pong" {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestTritonMuxOverH3LoopbackEcho(t *testing.T) {
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

	resp, err := RoundTripHandlerLoopback(listener, session, serverConn, tritonserver.NewMux(), http.MethodPost, "/echo", []byte("demo"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	body := string(resp.Body)
	if !strings.Contains(body, `"method":"POST"`) {
		t.Fatalf("missing method in body: %q", body)
	}
	if !strings.Contains(body, `"body":"demo"`) {
		t.Fatalf("missing echoed body in body: %q", body)
	}
}
