package h3

import (
	"net/http"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/quic/transport"
	tritonserver "github.com/tritonprobe/triton/internal/server"
)

func TestServiceRoundTrip(t *testing.T) {
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

	service := NewServer(listener, serverConn, tritonserver.NewMux())
	client := NewClient(session)

	resp, err := service.ServeRoundTrip(client, http.MethodGet, "/status/204", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}
