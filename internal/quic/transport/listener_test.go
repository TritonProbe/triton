package transport

import (
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/quic/connection"
	"github.com/tritonprobe/triton/internal/quic/stream"
)

func TestListenerDialerLoopback(t *testing.T) {
	listener, err := ListenQUIC("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	dialer := NewDialer(2 * time.Second)
	session, err := dialer.DialSession(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	clientConn := session.Connection()
	if clientConn.State() != connection.StateConnected {
		t.Fatalf("unexpected client state: %v", clientConn.State())
	}

	serverConn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if serverConn.State() != connection.StateConnected {
		t.Fatalf("unexpected server state: %v", serverConn.State())
	}

	if err := session.SendStream(0, []byte("ping"), true); err != nil {
		t.Fatal(err)
	}

	serverStream, err := acceptStreamWithTimeout(serverConn.Streams(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	n, _ := serverStream.Read(buf)
	if got := string(buf[:n]); got != "ping" {
		t.Fatalf("unexpected stream payload: %q", got)
	}

	if err := session.ReceiveFrames(); err != nil {
		t.Fatal(err)
	}
	clientStream, ok := clientConn.Streams().GetStream(0)
	if !ok {
		t.Fatal("expected response stream on client")
	}
	reply := make([]byte, 8)
	n, _ = clientStream.Read(reply)
	if got := string(reply[:n]); got != "ack:ping" {
		t.Fatalf("unexpected reply payload: %q", got)
	}
}

func acceptStreamWithTimeout(m *stream.Manager, timeout time.Duration) (*stream.Stream, error) {
	type result struct {
		s   *stream.Stream
		err error
	}
	ch := make(chan result, 1)
	go func() {
		s, err := m.AcceptStream()
		ch <- result{s: s, err: err}
	}()
	select {
	case res := <-ch:
		return res.s, res.err
	case <-time.After(timeout):
		return nil, connectionTimeoutError{}
	}
}

type connectionTimeoutError struct{}

func (connectionTimeoutError) Error() string { return "timeout waiting for stream" }
