package connection

import (
	"testing"

	"github.com/tritonprobe/triton/internal/quic/frame"
)

func TestHandleFramesUpdatesStateAndStreams(t *testing.T) {
	c := New(RoleClient, 1, []byte{0x01, 0x02})
	if err := c.Transition(StateInitialSent); err != nil {
		t.Fatal(err)
	}
	if err := c.Transition(StateHandshake); err != nil {
		t.Fatal(err)
	}

	frames := []frame.Frame{
		frame.MaxDataFrame{MaximumData: 2048},
		frame.StreamFrame{StreamID: 1, Offset: 0, Data: []byte("hello"), Fin: true},
		frame.PathChallengeFrame{Data: [8]byte{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'}},
		frame.PathResponseFrame{Data: [8]byte{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'}},
		frame.HandshakeDoneFrame{},
	}
	if err := c.HandleFrames(frames); err != nil {
		t.Fatal(err)
	}

	if c.State() != StateConnected {
		t.Fatalf("expected connected state, got %v", c.State())
	}
	s, ok := c.Streams().GetStream(1)
	if !ok {
		t.Fatal("expected remote stream")
	}
	buf := make([]byte, 5)
	n, _ := s.Read(buf)
	if got := string(buf[:n]); got != "hello" {
		t.Fatalf("unexpected stream data: %q", got)
	}
	if c.PendingPathChallenges() != 0 {
		t.Fatalf("expected cleared path challenges, got %d", c.PendingPathChallenges())
	}
}

func TestHandleResetStream(t *testing.T) {
	c := New(RoleServer, 1, []byte{0x01})
	s, err := c.Streams().OpenStream(true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write([]byte("payload")); err != nil {
		t.Fatal(err)
	}
	if err := c.HandleFrames([]frame.Frame{frame.ResetStreamFrame{StreamID: s.ID(), ErrorCode: 42, FinalSize: 7}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Streams().GetStream(s.ID()); ok {
		t.Fatal("expected stream to be removed after reset")
	}
}
