package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveBenchListAndCleanup(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir, 1, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SaveBench("bench-1", map[string]any{"id": "bench-1"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := store.SaveBench("bench-2", map[string]any{"id": "bench-2"}); err != nil {
		t.Fatal(err)
	}

	items, err := store.List("benches")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "bench-2" {
		t.Fatalf("unexpected bench list after cleanup: %+v", items)
	}
}

func TestCleanupByRetentionAndLoadErrors(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir, 10, time.Nanosecond)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "probes", "old.json.gz")
	if err := os.WriteFile(path, []byte("not-gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	if err := store.cleanup("probes"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected expired file to be removed, got %v", err)
	}

	badPath := filepath.Join(dir, "probes", "bad.json.gz")
	if err := os.WriteFile(badPath, []byte("not-gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := store.Load("probes", "bad", &out); err == nil {
		t.Fatal("expected invalid gzip load to fail")
	}
}

func TestStoreRejectsInvalidCategoryAndID(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir, 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SaveProbe("../escape", map[string]any{"id": "bad"}); err == nil {
		t.Fatal("expected invalid id to fail")
	}
	if _, err := store.List("../escape"); err == nil {
		t.Fatal("expected invalid category to fail")
	}
	var out map[string]any
	err = store.Load("probes", `..\outside`, &out)
	if err == nil {
		t.Fatal("expected path traversal load to fail")
	}
	if !strings.Contains(err.Error(), "invalid result id") {
		t.Fatalf("expected invalid id error, got %v", err)
	}
}
