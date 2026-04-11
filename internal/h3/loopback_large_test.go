package h3

import (
	"strings"
	"testing"

	"github.com/tritonprobe/triton/internal/quic/stream"
)

func TestReadAllHandlesMultipleChunks(t *testing.T) {
	payload := strings.Repeat("x", 6000)
	s := stream.New(0, 1<<20, 1<<20)
	if err := s.PushRecv(0, []byte(payload[:3000]), false); err != nil {
		t.Fatal(err)
	}
	if err := s.PushRecv(3000, []byte(payload[3000:]), true); err != nil {
		t.Fatal(err)
	}
	got, err := readAll(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Fatalf("unexpected payload length: got %d want %d", len(got), len(payload))
	}
}
