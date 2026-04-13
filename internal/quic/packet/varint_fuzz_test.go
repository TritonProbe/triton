package packet

import (
	"bytes"
	"testing"
)

func FuzzVarIntRoundTrip(f *testing.F) {
	for _, seed := range []uint64{0, 1, 63, 64, 15293, 16383, 16384, 999999, 1073741823} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value uint64) {
		encoded, err := EncodeVarInt(value)
		if err != nil {
			if value > 4611686018427387903 {
				return
			}
			t.Fatalf("unexpected encode error for %d: %v", value, err)
		}

		decoded, err := DecodeVarInt(encoded)
		if err != nil {
			t.Fatalf("DecodeVarInt failed after successful encode: %v", err)
		}
		if decoded != value {
			t.Fatalf("round-trip mismatch: got %d want %d", decoded, value)
		}

		var buf bytes.Buffer
		n, err := WriteVarInt(&buf, value)
		if err != nil {
			t.Fatalf("WriteVarInt failed after successful encode: %v", err)
		}
		if n != len(encoded) {
			t.Fatalf("unexpected encoded length: got %d want %d", n, len(encoded))
		}

		read, consumed, err := ReadVarInt(&buf)
		if err != nil {
			t.Fatalf("ReadVarInt failed after successful write: %v", err)
		}
		if read != value {
			t.Fatalf("stream round-trip mismatch: got %d want %d", read, value)
		}
		if consumed != len(encoded) {
			t.Fatalf("unexpected consumed length: got %d want %d", consumed, len(encoded))
		}
	})
}
