package observability

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type ManagedLogger struct {
	Logger *slog.Logger
	close  func() error
}

func (l *ManagedLogger) Close() error {
	if l == nil || l.close == nil {
		return nil
	}
	return l.close()
}

func NewLogger(path string) (*ManagedLogger, error) {
	if path == "" {
		return &ManagedLogger{Logger: slog.New(slog.NewJSONHandler(os.Stdout, nil))}, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return newManagedLogger(file, file.Close), nil
}

func newManagedLogger(writer io.Writer, closeFn func() error) *ManagedLogger {
	return &ManagedLogger{
		Logger: slog.New(slog.NewJSONHandler(writer, nil)),
		close:  closeFn,
	}
}
