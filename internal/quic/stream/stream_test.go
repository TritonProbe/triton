package stream

import (
	"sync"
	"testing"
)

func TestStreamReadWriteAndReassembly(t *testing.T) {
	s := New(0, 1024, 1024)
	if _, err := s.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := s.PushRecv(3, []byte("def"), true); err != nil {
		t.Fatal(err)
	}
	if err := s.PushRecv(0, []byte("abc"), false); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 6)
	n, err := s.Read(buf)
	if err != nil && err.Error() != "EOF" && err != nil {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "abcdef" {
		t.Fatalf("unexpected read: %q", got)
	}
}

func TestRecvBufferOutOfOrder(t *testing.T) {
	r := newRecvBuffer()
	if err := r.Insert(5, []byte("world")); err != nil {
		t.Fatal(err)
	}
	if err := r.Insert(0, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if got := r.Readable(0); got != 10 {
		t.Fatalf("unexpected readable bytes: %d", got)
	}
}

func TestStreamConcurrentCloseWriteAndPushRecv(t *testing.T) {
	s := New(0, 1024, 1024)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := s.CloseWrite(); err != nil {
			t.Errorf("CloseWrite returned error: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		if err := s.PushRecv(0, []byte("abc"), true); err != nil {
			t.Errorf("PushRecv returned error: %v", err)
		}
	}()

	wg.Wait()

	if got := s.State(); got != StateClosed {
		t.Fatalf("unexpected stream state after concurrent local/remote close: %v", got)
	}

	buf := make([]byte, 3)
	n, err := s.Read(buf)
	if n != 3 || err == nil {
		t.Fatalf("expected final read with EOF, got n=%d err=%v", n, err)
	}
	if got := string(buf[:n]); got != "abc" {
		t.Fatalf("unexpected payload: %q", got)
	}
}

func TestStreamConcurrentCloseAndResetEndsClosed(t *testing.T) {
	s := New(4, 1024, 1024)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		if err := s.Close(); err != nil {
			t.Errorf("Close returned error: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		s.Reset(99)
	}()

	wg.Wait()

	if got := s.State(); got != StateClosed {
		t.Fatalf("expected closed state after concurrent close/reset, got %v", got)
	}
}
