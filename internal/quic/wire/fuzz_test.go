package wire

import "testing"

func FuzzParsePacketFrames(f *testing.F) {
	f.Add([]byte{
		0xc1,
		0x00, 0x00, 0x00, 0x01,
		0x02, 0xde, 0xad,
		0x02, 0xca, 0xfe,
		0x04,
		0x01,
		0x01,
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

		header, frames, err := ParsePacketFrames(data, shortHeaderDCIDLen)
		if err != nil {
			return
		}
		if header == nil {
			t.Fatal("expected non-nil header on successful parse")
		}
		if frames == nil {
			t.Fatal("expected non-nil frames slice on successful parse")
		}
		_ = header.IsLongHeader()
	})
}
