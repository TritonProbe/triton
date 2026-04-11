package stream

import (
	"io"
	"testing"
)

func TestStreamStateAndFlowControlPaths(t *testing.T) {
	s := New(1, 3, 3)
	if got := s.ID(); got != 1 {
		t.Fatalf("unexpected stream id: %d", got)
	}
	if got := s.MaxSendData(); got != 3 {
		t.Fatalf("unexpected max send data: %d", got)
	}
	s.SetMaxSendData(5)
	if got := s.MaxSendData(); got != 5 {
		t.Fatalf("unexpected updated max send data: %d", got)
	}
	if _, err := s.Write([]byte("hello")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if got := s.BufferedSendBytes(); got != 5 {
		t.Fatalf("unexpected buffered bytes: %d", got)
	}
	if buf := s.SendBuffer(); string(buf) != "hello" {
		t.Fatalf("unexpected send buffer: %q", buf)
	}
	if err := s.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite returned error: %v", err)
	}
	if _, err := s.Write([]byte("x")); err == nil {
		t.Fatal("expected write after fin to fail")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if s.State() != StateClosed {
		t.Fatalf("expected closed state, got %v", s.State())
	}
}

func TestStreamReceiveEOFAndReset(t *testing.T) {
	s := New(2, 10, 4)
	if err := s.PushRecv(0, []byte("abcd"), true); err != nil {
		t.Fatalf("PushRecv returned error: %v", err)
	}
	buf := make([]byte, 4)
	n, err := s.Read(buf)
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
	if n != 4 || string(buf[:n]) != "abcd" {
		t.Fatalf("unexpected read: n=%d data=%q", n, buf[:n])
	}
	if _, err := s.Read(buf); err != io.EOF {
		t.Fatalf("expected EOF on second read, got %v", err)
	}

	s2 := New(3, 10, 3)
	if err := s2.PushRecv(0, []byte("toolong"), false); err == nil {
		t.Fatal("expected receive flow control error")
	}
	s2.Reset(42)
	if s2.State() != StateClosed {
		t.Fatalf("expected reset stream to be closed, got %v", s2.State())
	}
}

func TestRecvBufferMergeAndAdvance(t *testing.T) {
	r := newRecvBuffer()
	if err := r.Insert(0, []byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := r.Insert(2, []byte("cde")); err != nil {
		t.Fatal(err)
	}
	if err := r.Insert(0, nil); err != nil {
		t.Fatal(err)
	}
	if got := r.Readable(0); got != 5 {
		t.Fatalf("unexpected readable bytes: %d", got)
	}

	buf := make([]byte, 2)
	n, err := r.ReadAtOffset(0, buf)
	if err != nil || n != 2 || string(buf[:n]) != "ab" {
		t.Fatalf("unexpected read at offset: n=%d err=%v data=%q", n, err, buf[:n])
	}
	if got := r.Readable(2); got != 3 {
		t.Fatalf("unexpected readable after partial read: %d", got)
	}
}
