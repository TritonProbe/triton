package packet

import (
	"strings"
	"testing"
)

func TestParseHeaderErrorsAndRetry(t *testing.T) {
	if _, _, err := ParseHeader(nil, 0); err == nil {
		t.Fatal("expected empty header error")
	}
	if _, _, err := ParseHeader([]byte{0x40}, 0); err == nil || !strings.Contains(err.Error(), "short header DCID length must be positive") {
		t.Fatalf("expected short header dcid error, got %v", err)
	}

	retry := []byte{
		0xf0,
		0x00, 0x00, 0x00, 0x01,
		0x01, 0xaa,
		0x01, 0xbb,
		0x99, 0x98,
		0, 1, 2, 3, 4, 5, 6, 7,
		8, 9, 10, 11, 12, 13, 14, 15,
	}
	h, payload, err := ParseHeader(retry, 0)
	if err != nil {
		t.Fatalf("ParseHeader retry returned error: %v", err)
	}
	lh, ok := h.(*LongHeader)
	if !ok || lh.Type != PacketTypeRetry {
		t.Fatalf("expected retry long header, got %#v", h)
	}
	if len(lh.Token) != 2 {
		t.Fatalf("expected retry token, got %v", lh.Token)
	}
	if len(payload) != 16 {
		t.Fatalf("expected retry integrity tail, got %d bytes", len(payload))
	}

	short := &ShortHeader{DCID: []byte{0xaa}, PacketNumber: 5, KeyPhase: true, SpinBit: true, PNLength: 2}
	if short.PacketType() != PacketTypeOneRTT || short.Version() != 0 || short.PacketNumberLength() != 2 {
		t.Fatalf("unexpected short header accessors: %#v", short)
	}
	if got := short.SrcConnectionID(); got != nil {
		t.Fatalf("expected nil source cid, got %v", got)
	}
	if pn := readPacketNumber([]byte{0x01, 0x02, 0x03}); pn != 0x010203 {
		t.Fatalf("unexpected packet number decode: %#x", pn)
	}
}
