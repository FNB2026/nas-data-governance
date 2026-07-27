package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProgressReporterWritesPrivateAggregateSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "progress.json")
	r, err := startProgressReporter(path, time.Minute, func() progressSnapshot {
		return progressSnapshot{Command: "scan", Stage: "hash", Status: "running", Processed: 42, Total: 100}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got progressSnapshot
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Processed != 42 || got.Stage != "hash" || got.UpdatedAt.IsZero() {
		t.Fatalf("unexpected snapshot: %#v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
}
