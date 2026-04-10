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
