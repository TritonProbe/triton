package stream

import "testing"

func TestManagerOpenAndAcceptRemote(t *testing.T) {
	m := NewManager(RoleClient)
	local, err := m.OpenStream(true)
	if err != nil {
		t.Fatal(err)
	}
	if local.ID() != 0 {
		t.Fatalf("unexpected local stream id: %d", local.ID())
	}

	remote, created := m.GetOrCreateRemoteStream(1)
	if !created {
		t.Fatal("expected remote stream creation")
	}
	accepted, err := m.AcceptStream()
	if err != nil {
		t.Fatal(err)
	}
	if accepted.ID() != remote.ID() {
		t.Fatalf("unexpected accepted stream id: %d", accepted.ID())
	}
}
