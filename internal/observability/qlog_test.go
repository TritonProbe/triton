package observability

import (
	"context"
	"testing"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlog"
)

func TestNewQLOGTracerWritesFile(t *testing.T) {
	dir := t.TempDir()
	tracer := NewQLOGTracer(dir)
	if tracer == nil {
		t.Fatal("expected tracer")
	}

	connID := quic.ConnectionIDFromBytes([]byte{0xde, 0xad, 0xbe, 0xef})
	trace := tracer(context.Background(), true, connID)
	if trace == nil {
		t.Fatal("expected qlog trace")
	}
	rec := trace.AddProducer()
	if rec == nil {
		t.Fatal("expected recorder")
	}
	rec.RecordEvent(qlog.DebugEvent{Message: "hello"})
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	ok, err := HasQLOGFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected qlog file")
	}
}
