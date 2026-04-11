package wire

import (
	"bytes"
	"io"
	"testing"

	"github.com/tritonprobe/triton/internal/quic/frame"
)

type badFrame struct{}

func (badFrame) Type() frame.FrameType     { return frame.FrameTypePing }
func (badFrame) Length() int               { return 1 }
func (badFrame) Serialize(io.Writer) error { return nil }

func TestPacketNumberHelpersAndErrors(t *testing.T) {
	cases := []struct {
		in   uint64
		want int
	}{
		{0xff, 1},
		{0x100, 2},
		{0x10000, 3},
		{0x1000000, 4},
	}
	for _, tc := range cases {
		if got := packetNumberLen(tc.in); got != tc.want {
			t.Fatalf("packetNumberLen(%d)=%d want %d", tc.in, got, tc.want)
		}
	}

	var buf bytes.Buffer
	writePacketNumber(&buf, 0x01020304, 4)
	if got := buf.Bytes(); !bytes.Equal(got, []byte{0x01, 0x02, 0x03, 0x04}) {
		t.Fatalf("unexpected packet number bytes: %v", got)
	}
}

func TestParsePacketFramesError(t *testing.T) {
	if _, _, err := ParsePacketFrames([]byte{0x40}, 4); err == nil {
		t.Fatal("expected packet parse error")
	}
}
