package connection

import (
	"errors"
	"sync"
	"testing"

	"github.com/tritonprobe/triton/internal/quic/frame"
)

func TestConnectionConcurrentCloseWithErrorAndTransitionEndsClosed(t *testing.T) {
	c := New(RoleClient, []byte{0xaa})

	if err := c.Transition(StateInitialSent); err != nil {
		t.Fatal(err)
	}
	if err := c.Transition(StateHandshake); err != nil {
		t.Fatal(err)
	}

	closeErr := errors.New("boom")

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		c.CloseWithError(closeErr)
	}()

	go func() {
		defer wg.Done()
		_ = c.Transition(StateConnected)
	}()

	wg.Wait()

	if got := c.State(); got != StateClosed {
		t.Fatalf("expected closed state, got %v", got)
	}
	if got := c.CloseError(); !errors.Is(got, closeErr) {
		t.Fatalf("expected close error %v, got %v", closeErr, got)
	}
}

func TestHandshakeDoneFrameTransitionsToConnectedOnce(t *testing.T) {
	c := New(RoleClient, []byte{0xaa})

	if err := c.Transition(StateInitialSent); err != nil {
		t.Fatal(err)
	}
	if err := c.Transition(StateHandshake); err != nil {
		t.Fatal(err)
	}

	if err := c.HandleFrames([]frame.Frame{frame.HandshakeDoneFrame{}}); err != nil {
		t.Fatalf("HandleFrames returned error: %v", err)
	}
	if got := c.State(); got != StateConnected {
		t.Fatalf("expected connected state after handshake done, got %v", got)
	}

	if err := c.HandleFrames([]frame.Frame{frame.HandshakeDoneFrame{}}); err != nil {
		t.Fatalf("second HandleFrames returned error: %v", err)
	}
	if got := c.State(); got != StateConnected {
		t.Fatalf("expected connection to remain connected, got %v", got)
	}
}
