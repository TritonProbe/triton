package packet

import "testing"

func TestEncodeDecodePacketNumber(t *testing.T) {
	truncated, bits := EncodePacketNumber(0xabe8bc, 0xabe800)
	if bits != 16 {
		t.Fatalf("unexpected bits: %d", bits)
	}
	got := DecodePacketNumber(0xabe800, truncated, bits)
	if got != 0xabe8bc {
		t.Fatalf("unexpected packet number: got %#x", got)
	}
}
