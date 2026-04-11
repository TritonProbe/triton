package stream

import "testing"

func TestManagerLimitsAndClose(t *testing.T) {
	m := NewManager(RoleServer)
	m.maxBidiLocal = 0
	if _, err := m.OpenStream(true); err == nil {
		t.Fatal("expected bidi stream limit error")
	}

	m.maxUniLocal = 0
	if _, err := m.OpenStream(false); err == nil {
		t.Fatal("expected uni stream limit error")
	}

	m = NewManager(RoleServer)
	s, err := m.OpenStream(true)
	if err != nil {
		t.Fatalf("OpenStream returned error: %v", err)
	}
	if got, ok := m.GetStream(s.ID()); !ok || got.ID() != s.ID() {
		t.Fatalf("expected open stream to be retrievable")
	}
	m.CloseStream(s.ID())
	if _, ok := m.GetStream(s.ID()); ok {
		t.Fatal("expected stream to be removed")
	}
}
