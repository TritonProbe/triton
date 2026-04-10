package packet

import (
	"bytes"
	"testing"
)

func TestVarIntRoundTrip(t *testing.T) {
	cases := []uint64{0, 10, 63, 64, 15293, 16383, 16384, 999999, 1073741823}
	for _, tc := range cases {
		buf, err := EncodeVarInt(tc)
		if err != nil {
			t.Fatalf("encode %d: %v", tc, err)
		}
		got, err := DecodeVarInt(buf)
		if err != nil {
			t.Fatalf("decode %d: %v", tc, err)
		}
		if got != tc {
			t.Fatalf("round trip mismatch: got %d want %d", got, tc)
		}

		var stream bytes.Buffer
		if _, err := WriteVarInt(&stream, tc); err != nil {
			t.Fatalf("write %d: %v", tc, err)
		}
		read, n, err := ReadVarInt(&stream)
		if err != nil {
			t.Fatalf("read %d: %v", tc, err)
		}
		if read != tc {
			t.Fatalf("stream round trip mismatch: got %d want %d", read, tc)
		}
		if n != len(buf) {
			t.Fatalf("unexpected encoded length: got %d want %d", n, len(buf))
		}
	}
}
