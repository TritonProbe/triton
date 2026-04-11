package connection

import (
	"errors"
	"io"
	"testing"

	"github.com/tritonprobe/triton/internal/quic/frame"
)

type unknownFrame struct{}

func (unknownFrame) Type() frame.FrameType   { return 0xff }
func (unknownFrame) Length() int             { return 1 }
func (unknownFrame) Serialize(io.Writer) error { return nil }

func TestConnectionTransitionAndCloseErrors(t *testing.T) {
	c := New(RoleClient, 1, []byte{0x01, 0x02})
	if err := c.Transition(StateConnected); err == nil {
		t.Fatal("expected invalid connected transition")
	}
	if err := c.Transition(StateInitialSent); err != nil {
		t.Fatalf("unexpected initial transition error: %v", err)
	}
	if err := c.Transition(StateInitialReceived); err == nil {
		t.Fatal("expected invalid second initial transition")
	}
	c.CloseWithError(errors.New("boom"))
	if c.State() != StateClosed {
		t.Fatalf("expected closed state, got %v", c.State())
	}
	if got := c.CloseError(); got == nil || got.Error() != "boom" {
		t.Fatalf("unexpected close error: %v", got)
	}
	if err := c.Transition(StateClosed); err == nil {
		t.Fatal("expected transition on closed connection to fail")
	}
}

func TestConnectionHandleAdditionalFrames(t *testing.T) {
	c := New(RoleServer, 1, []byte{0x09})
	if err := c.HandleFrames([]frame.Frame{
		frame.PaddingFrame{Count: 2},
		frame.PingFrame{},
		frame.ACKFrame{},
		frame.NewTokenFrame{Token: []byte("abc")},
		frame.NewConnectionIDFrame{ConnectionID: []byte{0xaa, 0xbb}},
		frame.RetireConnectionIDFrame{SequenceNumber: 1},
		frame.PathChallengeFrame{Data: [8]byte{'1'}},
		frame.PathResponseFrame{Data: [8]byte{'1'}},
	}); err != nil {
		t.Fatalf("HandleFrames returned error: %v", err)
	}
	if cid, ok := c.PrimaryRemoteCID(); !ok || len(cid) != 2 {
		t.Fatalf("expected stored remote CID, got %v %v", cid, ok)
	}
	if c.PendingPathChallenges() != 0 {
		t.Fatalf("expected path challenge to be cleared, got %d", c.PendingPathChallenges())
	}
}

func TestConnectionHandleFramesErrors(t *testing.T) {
	c := New(RoleClient, 1, []byte{0x01})
	c.StoreRemoteCID([]byte{0x10})
	if cid, ok := c.PrimaryRemoteCID(); !ok || cid[0] != 0x10 {
		t.Fatalf("unexpected primary remote cid: %v %v", cid, ok)
	}

	if err := c.RecordSend(c.remoteMaxData + 1); err == nil {
		t.Fatal("expected send flow control error")
	}
	if err := c.RecordReceive(c.localMaxData + 1); err == nil {
		t.Fatal("expected receive flow control error")
	}
	if err := c.HandleFrames([]frame.Frame{frame.CryptoFrame{Data: make([]byte, c.localMaxData+1)}}); err == nil {
		t.Fatal("expected crypto receive flow control error")
	}
	if err := c.HandleFrames([]frame.Frame{frame.StreamFrame{StreamID: 1, Data: make([]byte, (1 << 20) + 1)}}); err == nil {
		t.Fatal("expected stream receive flow control error")
	}
	if err := c.handleFrame(unknownFrame{}); err == nil {
		t.Fatal("expected unsupported frame dispatch error")
	}
}
