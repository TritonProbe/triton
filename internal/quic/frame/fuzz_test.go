package frame

import (
	"bytes"
	"testing"
)

func FuzzParseFrames(f *testing.F) {
	f.Add([]byte{0x01})
	f.Add([]byte{
		0x04, 0x04, 0x09, 0x0c,
	})
	f.Add([]byte{
		0x1b, 1, 2, 3, 4, 5, 6, 7, 8,
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		frames, err := ParseFrames(data)
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
			if frame.Length() < 0 {
				t.Fatalf("unexpected negative frame length for %T", frame)
			}
			if err := frame.Serialize(&buf); err != nil {
				t.Fatalf("Serialize(%T) returned error after successful parse: %v", frame, err)
			}
		}
	})
}
