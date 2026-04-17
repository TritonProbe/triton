package transport

import (
	"net"
	"testing"
	"time"

	"github.com/tritonprobe/triton/internal/quic/connection"
	"github.com/tritonprobe/triton/internal/quic/frame"
)

func TestDialerDefaultsAndRandomCID(t *testing.T) {
	d := NewDialer(0)
	if d.timeout <= 0 {
		t.Fatalf("expected default timeout, got %v", d.timeout)
	}
	cid, err := randomCID(8)
	if err != nil {
		t.Fatalf("randomCID returned error: %v", err)
	}
	if len(cid) != 8 {
		t.Fatalf("unexpected cid length: %d", len(cid))
	}
}

func TestUDPTransportClosedAndBatchPaths(t *testing.T) {
	tp := &UDPTransport{}
	if _, _, err := tp.ReadPacket(); err == nil {
		t.Fatal("expected read on closed transport to fail")
	}
	if err := tp.WritePacket([]byte("x"), &net.UDPAddr{}); err == nil {
		t.Fatal("expected write on closed transport to fail")
	}
	if err := tp.SetReadDeadline(time.Now()); err == nil {
		t.Fatal("expected read deadline on closed transport to fail")
	}
	if tp.LocalAddr() != nil {
		t.Fatal("expected nil local addr for closed transport")
	}
	if got := tp.mtu; got != 0 {
		t.Fatalf("expected zero mtu on zero-value transport, got %d", got)
	}
	if err := tp.Close(); err != nil {
		t.Fatalf("expected nil close on zero-value transport, got %v", err)
	}

	server, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	clientConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()

	client := New(clientConn)
	client.SetWriteTimeout(time.Second)
	server.SetReadTimeout(time.Second)
	if err := client.WriteBatch([][]byte{[]byte("a"), []byte("b")}, server.LocalAddr()); err != nil {
		t.Fatalf("WriteBatch returned error: %v", err)
	}
	got, _, err := server.ReadPacket()
	if err != nil || string(got) != "a" {
		t.Fatalf("unexpected first batch packet: %q err=%v", got, err)
	}
	got, _, err = server.ReadPacket()
	if err != nil || string(got) != "b" {
		t.Fatalf("unexpected second batch packet: %q err=%v", got, err)
	}
}

func TestListenerHelpersAndErrorPaths(t *testing.T) {
	listener, err := ListenQUIC("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	listener.SetAutoEcho(false)
	if listener.AutoEcho() {
		t.Fatal("expected auto echo to be disabled")
	}
	if _, err := waitForConnections(listener, 1, 20*time.Millisecond); err == nil {
		t.Fatal("expected timeout waiting for connections")
	}

	conn := connection.New(connection.RoleServer, []byte{0x01})
	if err := listener.SendFrames(conn, []frame.Frame{frame.PingFrame{}}); err == nil {
		t.Fatal("expected missing remote cid error")
	}
	conn.StoreRemoteCID([]byte{0xaa})
	if err := listener.SendFrames(conn, []frame.Frame{frame.PingFrame{}}); err == nil {
		t.Fatal("expected unknown address error")
	}
	if listener.RemoteAddr(nil) != nil {
		t.Fatal("expected nil remote addr for nil connection")
	}

	if err := listener.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if _, err := listener.Accept(); err == nil {
		t.Fatal("expected accept on closed listener to fail")
	}
	if _, err := waitForConnections(listener, 1, time.Second); err == nil {
		t.Fatal("expected wait on closed listener to fail")
	}
}

func TestDialSessionResolveFailure(t *testing.T) {
	d := NewDialer(50 * time.Millisecond)
	if _, err := d.DialSession("bad-address"); err == nil {
		t.Fatal("expected DialSession to fail for invalid address")
	}
	if _, err := d.Dial("bad-address"); err == nil {
		t.Fatal("expected Dial to fail for invalid address")
	}
}
