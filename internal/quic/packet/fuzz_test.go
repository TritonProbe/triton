package packet

import "testing"

func FuzzParseHeader(f *testing.F) {
	f.Add([]byte{
		0xc1,
		0x00, 0x00, 0x00, 0x01,
		0x04, 0xde, 0xad, 0xbe, 0xef,
		0x04, 0xca, 0xfe, 0xba, 0xbe,
		0x00,
		0x07,
		0x12, 0x34,
		0xaa, 0xbb, 0xcc, 0xdd, 0xee,
	}, 0)
	f.Add([]byte{
		0x41,
		0xde, 0xad, 0xbe, 0xef,
		0x37, 0x99,
	}, 4)
	f.Add([]byte{0x00}, 0)

	f.Fuzz(func(t *testing.T, data []byte, shortHeaderDCIDLen int) {
		if shortHeaderDCIDLen < 0 {
			shortHeaderDCIDLen = 0
		}
		if shortHeaderDCIDLen > 32 {
			shortHeaderDCIDLen = shortHeaderDCIDLen % 33
		}

		h, payload, err := ParseHeader(data, shortHeaderDCIDLen)
		if err != nil {
			return
		}
		if h == nil {
			t.Fatal("expected non-nil header on successful parse")
		}
		_ = h.IsLongHeader()
		_ = h.PacketType()
		_ = h.Version()
		_ = h.DestConnectionID()
		_ = h.SrcConnectionID()
		if h.PacketNumberLength() < 1 || h.PacketNumberLength() > 4 {
			t.Fatalf("unexpected packet number length: %d", h.PacketNumberLength())
		}
		if payload == nil {
			t.Fatal("expected non-nil payload slice on successful parse")
		}
	})
}
