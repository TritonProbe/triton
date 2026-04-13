package transport

import (
	"net"
	"testing"
	"time"
)

func TestUDPTransportReadWrite(t *testing.T) {
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

	want := []byte("hello-quic")
	if err := client.WritePacket(want, server.LocalAddr()); err != nil {
		t.Fatal(err)
	}

	got, _, err := server.ReadPacket()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("unexpected payload: got %q want %q", got, want)
	}
}

func TestUDPTransportCloseMakesFurtherOperationsSafe(t *testing.T) {
	server, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := server.LocalAddr()
	if addr == nil {
		t.Fatal("expected local addr before close")
	}

	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("expected double close to be harmless, got %v", err)
	}

	if server.LocalAddr() != nil {
		t.Fatal("expected local addr to be nil after close")
	}

	if _, _, err := server.ReadPacket(); err == nil {
		t.Fatal("expected read after close to fail")
	}

	if err := server.WritePacket([]byte("x"), addr); err == nil {
		t.Fatal("expected write after close to fail")
	}

	if err := server.SetReadDeadline(time.Now().Add(time.Second)); err == nil {
		t.Fatal("expected SetReadDeadline after close to fail")
	}
}
