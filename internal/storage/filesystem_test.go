package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoadProbe(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(dir, 10, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	input := map[string]any{"id": "probe-1", "value": 42}
	if err := store.SaveProbe("probe-1", input); err != nil {
		t.Fatal(err)
	}

	var output map[string]any
	if err := store.Load("probes", "probe-1", &output); err != nil {
		t.Fatal(err)
	}
	if output["id"] != "probe-1" {
		t.Fatalf("unexpected id: %#v", output["id"])
	}
	if _, err := filepath.Abs(dir); err != nil {
		t.Fatal(err)
	}
}
