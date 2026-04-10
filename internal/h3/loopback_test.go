package h3

import (
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/quic/transport"
)

func TestRoundTripLoopback(t *testing.T) {
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

	resp, err := RoundTripLoopback(listener, session, serverConn, "GET", "/demo", []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Headers[":status"] != "200" {
		t.Fatalf("unexpected status header: %#v", resp.Headers)
	}
	if got := string(resp.Body); got != "ok:GET:/demo" {
		t.Fatalf("unexpected body: %q", got)
	}
}
