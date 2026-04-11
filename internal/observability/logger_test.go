package observability

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLoggerToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "access.jsonl")
	logger, err := NewLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	logger.Logger.Info("hello", slog.String("component", "test"))

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"component":"test"`) {
		t.Fatalf("unexpected log contents: %q", string(data))
	}
}

func TestManagedLoggerCloseNilSafe(t *testing.T) {
	var buf bytes.Buffer
	logger := newManagedLogger(&buf, nil)
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewLoggerStdoutAndCloseError(t *testing.T) {
	logger, err := NewLogger("")
	if err != nil {
		t.Fatalf("NewLogger returned error: %v", err)
	}
	if logger == nil || logger.Logger == nil {
		t.Fatal("expected stdout logger")
	}

	wantErr := errors.New("close boom")
	custom := newManagedLogger(&bytes.Buffer{}, func() error { return wantErr })
	if err := custom.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("expected close error %v, got %v", wantErr, err)
	}
}
