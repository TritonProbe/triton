package transport

import (
	"net"
	"sync"
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

func TestWaitForConnectionsDoesNotConsumeAcceptQueue(t *testing.T) {
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

	conns, err := listener.WaitForConnections(1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(conns) != 1 {
		t.Fatalf("expected one observed connection, got %d", len(conns))
	}

	accepted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if accepted != conns[0] {
		t.Fatal("expected Accept to return the same connection observed by WaitForConnections")
	}
}

func TestWaitForConnectionsPreservesAcceptOrderForMultipleConnections(t *testing.T) {
	listener, err := ListenQUIC("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	const total = 3
	dialer := NewDialer(2 * time.Second)
	sessions := make([]*Session, total)
	defer func() {
		for _, session := range sessions {
			if session != nil {
				_ = session.Close()
			}
		}
	}()

	var wg sync.WaitGroup
	errCh := make(chan error, total)
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			session, err := dialer.DialSession(listener.Addr().String())
			if err != nil {
				errCh <- err
				return
			}
			sessions[idx] = session
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("DialSession returned error: %v", err)
		}
	}

	observed, err := listener.WaitForConnections(total, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(observed) != total {
		t.Fatalf("expected %d observed connections, got %d", total, len(observed))
	}

	seen := make(map[*connection.Connection]bool, total)
	for i := 0; i < total; i++ {
		accepted, err := listener.Accept()
		if err != nil {
			t.Fatalf("Accept(%d) returned error: %v", i, err)
		}
		if accepted != observed[i] {
			t.Fatalf("accept order mismatch at %d", i)
		}
		if seen[accepted] {
			t.Fatalf("connection accepted more than once at %d", i)
		}
		seen[accepted] = true
	}
}

func TestListenerCloseUnblocksAccept(t *testing.T) {
	listener, err := ListenQUIC("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := listener.Accept()
		result <- err
	}()

	time.Sleep(50 * time.Millisecond)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected Accept to return an error after listener close")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Accept to unblock on listener close")
	}
}

func TestListenerCloseUnblocksWaitForConnections(t *testing.T) {
	listener, err := ListenQUIC("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	type result struct {
		conns []*connection.Connection
		err   error
	}
	done := make(chan result, 1)
	go func() {
		conns, err := listener.WaitForConnections(1, 5*time.Second)
		done <- result{conns: conns, err: err}
	}()

	time.Sleep(50 * time.Millisecond)
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case res := <-done:
		if res.err == nil {
			t.Fatal("expected WaitForConnections to return an error after listener close")
		}
		if len(res.conns) != 0 {
			t.Fatalf("expected no observed connections, got %d", len(res.conns))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for WaitForConnections to unblock on listener close")
	}
}

func TestListenerIgnoresMalformedPacketsWithoutCreatingConnections(t *testing.T) {
	listener, err := ListenQUIC("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	conn, err := net.ListenUDP("udp", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	addr := listener.Addr()
	if addr == nil {
		t.Fatal("expected listener addr")
	}

	payloads := [][]byte{
		nil,
		{0x00},
		{0x41, 0x01},
		{0xc1, 0x00, 0x00, 0x00},
		{0xff, 0xff, 0xff, 0xff, 0xff},
	}
	for _, payload := range payloads {
		if _, err := conn.WriteToUDP(payload, addr); err != nil {
			t.Fatalf("WriteToUDP returned error: %v", err)
		}
	}

	time.Sleep(100 * time.Millisecond)

	conns, err := listener.WaitForConnections(1, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout while waiting for a connection from malformed packets")
	}
	if len(conns) != 0 {
		t.Fatalf("expected no observed connections, got %d", len(conns))
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
