package packet

import "testing"

func TestParseInitialLongHeader(t *testing.T) {
	data := []byte{
		0xc1,
		0x00, 0x00, 0x00, 0x01,
		0x04, 0xde, 0xad, 0xbe, 0xef,
		0x04, 0xca, 0xfe, 0xba, 0xbe,
		0x00,
		0x07,
		0x12, 0x34,
		0xaa, 0xbb, 0xcc, 0xdd, 0xee,
	}

	h, payload, err := ParseHeader(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	lh, ok := h.(*LongHeader)
	if !ok {
		t.Fatalf("expected long header, got %T", h)
	}
	if lh.Type != PacketTypeInitial {
		t.Fatalf("unexpected packet type: %v", lh.Type)
	}
	if lh.VersionNum != 1 {
		t.Fatalf("unexpected version: %d", lh.VersionNum)
	}
	if lh.PacketNumber != 0x1234 {
		t.Fatalf("unexpected packet number: %#x", lh.PacketNumber)
	}
	if string(payload) != string([]byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee}) {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestParseShortHeader(t *testing.T) {
	data := []byte{
		0x41,
		0xde, 0xad, 0xbe, 0xef,
		0x37, 0x99,
	}
	h, payload, err := ParseHeader(data, 4)
	if err != nil {
		t.Fatal(err)
	}
	sh, ok := h.(*ShortHeader)
	if !ok {
		t.Fatalf("expected short header, got %T", h)
	}
	if sh.PacketNumber != 0x3799 {
		t.Fatalf("unexpected packet number: %#x", sh.PacketNumber)
	}
	if len(payload) != 0 {
		t.Fatalf("unexpected payload length: %d", len(payload))
	}
}

func TestParseInitialLongHeaderRejectsOversizedTokenVarint(t *testing.T) {
	data := []byte{
		0xc1,
		0x00, 0x00, 0x00, 0x01,
		0x01, 0xde,
		0x01, 0xad,
		0x40, 0x10,
	}
	if _, _, err := ParseHeader(data, 0); err == nil {
		t.Fatal("expected oversized token varint to fail")
	}
}
