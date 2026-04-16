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

func TestListCacheInvalidatesOnExternalDelete(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir, 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SaveProbe("probe-1", map[string]any{"id": "probe-1"}); err != nil {
		t.Fatal(err)
	}
	items, err := store.List("probes")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one probe item, got %+v", items)
	}

	if err := os.Remove(items[0].Path); err != nil {
		t.Fatal(err)
	}

	items, err = store.List("probes")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected cache invalidation after delete, got %+v", items)
	}
}

func TestListReturnsIndependentSlice(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir, 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SaveBench("bench-1", map[string]any{"id": "bench-1"}); err != nil {
		t.Fatal(err)
	}
	items, err := store.List("benches")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one bench item, got %+v", items)
	}
	items[0].ID = "mutated"

	refetched, err := store.List("benches")
	if err != nil {
		t.Fatal(err)
	}
	if len(refetched) != 1 || refetched[0].ID != "bench-1" {
		t.Fatalf("expected cached list to be isolated from caller mutation, got %+v", refetched)
	}
}

func TestSaveLoadAndCleanupSummaries(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir, 1, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SaveProbe("probe-1", map[string]any{"id": "probe-1"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProbeSummary("probe-1", map[string]any{"id": "probe-1", "target": "https://example.com"}); err != nil {
		t.Fatal(err)
	}

	var summary map[string]any
	if err := store.LoadProbeSummary("probe-1", &summary); err != nil {
		t.Fatal(err)
	}
	if summary["target"] != "https://example.com" {
		t.Fatalf("unexpected summary payload: %#v", summary)
	}

	time.Sleep(10 * time.Millisecond)
	if err := store.SaveProbe("probe-2", map[string]any{"id": "probe-2"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveProbeSummary("probe-2", map[string]any{"id": "probe-2"}); err != nil {
		t.Fatal(err)
	}

	if err := store.LoadProbeSummary("probe-1", &summary); !os.IsNotExist(err) {
		t.Fatalf("expected cleaned-up summary for evicted probe, got %v", err)
	}
}

func TestLoadSummaryFallsBackToPersistedIndex(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir, 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SaveProbeSummary("probe-1", map[string]any{
		"id":     "probe-1",
		"target": "https://indexed.example.com",
	}); err != nil {
		t.Fatal(err)
	}

	path, err := store.summaryPath("probes", "probe-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewFileStore(dir, 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	var summary map[string]any
	if err := restarted.LoadProbeSummary("probe-1", &summary); err != nil {
		t.Fatal(err)
	}
	if summary["target"] != "https://indexed.example.com" {
		t.Fatalf("expected summary from persisted index, got %#v", summary)
	}
}
