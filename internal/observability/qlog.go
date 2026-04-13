package observability

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlog"
	"github.com/quic-go/quic-go/qlogwriter"
)

type bufferedWriteCloser struct {
	*bufio.Writer
	file *os.File
}

func (b *bufferedWriteCloser) Close() error {
	if err := b.Writer.Flush(); err != nil {
		_ = b.file.Close()
		return err
	}
	return b.file.Close()
}

func NewQLOGTracer(traceDir string) func(context.Context, bool, quic.ConnectionID) qlogwriter.Trace {
	if traceDir == "" {
		return nil
	}
	return func(_ context.Context, isClient bool, connID quic.ConnectionID) qlogwriter.Trace {
		if err := os.MkdirAll(traceDir, 0o750); err != nil {
			return nil
		}
		role := "server"
		if isClient {
			role = "client"
		}
		path := filepath.Join(traceDir, fmt.Sprintf("%s_%s.sqlog", connID, role))
		// #nosec G304 -- traceDir is explicit operator configuration and connID/role are generated locally.
		file, err := os.Create(path)
		if err != nil {
			return nil
		}
		trace := qlogwriter.NewConnectionFileSeq(
			&bufferedWriteCloser{Writer: bufio.NewWriter(file), file: file},
			isClient,
			connID,
			[]string{qlog.EventSchema},
		)
		go trace.Run()
		return trace
	}
}

func HasQLOGFiles(traceDir string) (bool, error) {
	files, err := ListQLOGFiles(traceDir)
	if err != nil {
		return false, err
	}
	return len(files) > 0, nil
}

func ListQLOGFiles(traceDir string) ([]string, error) {
	if traceDir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(traceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".sqlog") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

func DiffQLOGFiles(before, after []string) []string {
	seen := make(map[string]struct{}, len(before))
	for _, name := range before {
		seen[name] = struct{}{}
	}
	out := make([]string, 0, len(after))
	for _, name := range after {
		if _, ok := seen[name]; ok {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
