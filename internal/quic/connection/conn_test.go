package connection

import (
	"testing"
	"time"
)

func TestConnectionTransitions(t *testing.T) {
	c := New(RoleClient, []byte{0xaa, 0xbb})
	if err := c.Transition(StateInitialSent); err != nil {
		t.Fatal(err)
	}
	if err := c.Transition(StateHandshake); err != nil {
		t.Fatal(err)
	}
	if err := c.Transition(StateConnected); err != nil {
		t.Fatal(err)
	}
	if c.State() != StateConnected {
		t.Fatalf("unexpected state: %v", c.State())
	}
}

func TestConnectionFlowControlAndIdle(t *testing.T) {
	c := New(RoleServer, []byte{0x01})
	if err := c.RecordReceive(128); err != nil {
		t.Fatal(err)
	}
	if err := c.RecordSend(128); err != nil {
		t.Fatal(err)
	}
	if !c.IdleExpired(time.Now().Add(31 * time.Second)) {
		t.Fatal("expected idle timeout")
	}
}
