package h3

import "testing"

func TestEncodeDecodeHeaders(t *testing.T) {
	in := map[string]string{
		":method": "GET",
		":path":   "/ping",
		"x-test":  "1",
	}
	block := EncodeHeaders(in)
	out := DecodeHeaders(block)
	if out[":method"] != "GET" || out[":path"] != "/ping" || out["x-test"] != "1" {
		t.Fatalf("unexpected decoded headers: %#v", out)
	}
}
