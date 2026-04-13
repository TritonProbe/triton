package wire

import (
	"testing"

	"github.com/tritonprobe/triton/internal/quic/frame"
)

func TestBuildAndParseInitialPacket(t *testing.T) {
	data, err := BuildInitialPacket(1, []byte{0xde, 0xad}, []byte{0xca, 0xfe}, 1, []frame.Frame{
		frame.PingFrame{},
		frame.CryptoFrame{Offset: 0, Data: []byte("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	h, frames, err := ParsePacketFrames(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !h.IsLongHeader() {
		t.Fatal("expected long header")
	}
	if len(frames) != 2 {
		t.Fatalf("unexpected frame count: %d", len(frames))
	}
}

func TestBuildPacketRejectsOversizedConnectionIDs(t *testing.T) {
	oversized := make([]byte, 256)
	if _, err := BuildInitialPacket(1, oversized, []byte{0x01}, 1, []frame.Frame{frame.PingFrame{}}); err == nil {
		t.Fatal("expected oversized initial packet CID to fail")
	}
	if _, err := BuildShortPacket(oversized, 1, []frame.Frame{frame.PingFrame{}}); err == nil {
		t.Fatal("expected oversized short packet CID to fail")
	}
}
