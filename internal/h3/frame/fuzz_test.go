package frame

import (
	"bytes"
	"testing"
)

func FuzzParse(f *testing.F) {
	f.Add([]byte{
		0x01, 0x11,
		'm', 'e', 't', 'h', 'o', 'd', ':', 'G', 'E', 'T', '\n',
		'p', 'a', 't', 'h', ':', '/',
	})
	f.Add([]byte{
		0x00, 0x05, 'h', 'e', 'l', 'l', 'o',
	})
	f.Add([]byte{0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		frames, err := Parse(data)
		if err != nil {
			return
		}
		if len(frames) == 0 && len(data) > 0 {
			t.Fatal("expected parsed frames or an error")
		}

		var buf bytes.Buffer
		for _, frame := range frames {
			if frame == nil {
				t.Fatal("unexpected nil frame")
			}
			if err := frame.Serialize(&buf); err != nil {
				t.Fatalf("Serialize(%T) returned error after successful parse: %v", frame, err)
			}
		}
	})
}
