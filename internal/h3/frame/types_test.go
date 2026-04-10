package frame

import "testing"

func TestEncodeParseFrames(t *testing.T) {
	data, err := Encode([]Frame{
		HeadersFrame{Block: []byte("method:GET\npath:/")},
		DataFrame{Data: []byte("hello")},
	})
	if err != nil {
		t.Fatal(err)
	}
	frames, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 {
		t.Fatalf("unexpected frame count: %d", len(frames))
	}
	h, ok := frames[0].(HeadersFrame)
	if !ok || string(h.Block) != "method:GET\npath:/" {
		t.Fatalf("unexpected headers frame: %#v", frames[0])
	}
	d, ok := frames[1].(DataFrame)
	if !ok || string(d.Data) != "hello" {
		t.Fatalf("unexpected data frame: %#v", frames[1])
	}
}
