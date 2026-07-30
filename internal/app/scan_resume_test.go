package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// TestScanResumeFromAbortedCheckpoint verifies that when Resume=true and
// an aborted checkpoint exists, the scan reuses it: ResumedFrom is set to
// the checkpoint's last scanned path, and files before that path are skipped.
func TestScanResumeFromAbortedCheckpoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "source")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create 5 files: a.txt through e.txt
	for _, name := range []string{"a.txt", "b.txt", "c.txt", "d.txt", "e.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	st, err := store.Open(ctx, filepath.Join(tmp, "project.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	storageID := "resume-test"

	// Register storage so the checkpoint FK is satisfied.
	if err := st.RegisterStorage(ctx, domain.Storage{
		ID: storageID, RootPath: root, Kind: "filesystem",
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate an aborted scan that got through a.txt and b.txt (2 files),
	// checkpointing at c.txt as the resume boundary.
	cpID, err := st.StartCheckpoint(ctx, storageID)
	if err != nil {
		t.Fatalf("StartCheckpoint: %v", err)
	}
	resumePath := filepath.Join(root, "c.txt")
	if err := st.UpdateCheckpoint(ctx, cpID, resumePath, 2); err != nil {
		t.Fatalf("UpdateCheckpoint: %v", err)
	}
	if err := st.CompleteCheckpoint(ctx, cpID, "aborted"); err != nil {
		t.Fatalf("CompleteCheckpoint: %v", err)
	}

	// Run scan with Resume=true.
	svc := NewScanService(st)
	result, err := svc.Scan(ctx, ScanInput{
		Root: root, StorageID: storageID, Resume: true,
		Workers: 1, HashAttempts: 1, HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("Scan with resume: %v", err)
	}

	// The scan should have reused the aborted checkpoint.
	if result.ResumedFrom != resumePath {
		t.Errorf("ResumedFrom = %q, want %q", result.ResumedFrom, resumePath)
	}
	if result.ResumedCount != 2 {
		t.Errorf("ResumedCount = %d, want 2", result.ResumedCount)
	}
	if result.CheckpointID != cpID {
		t.Errorf("CheckpointID = %d, want %d (reused aborted checkpoint)", result.CheckpointID, cpID)
	}

	// Scanner skips files whose path sorts strictly before ResumePath.
	// c.txt, d.txt, e.txt should be discovered (3 files).
	if len(result.Files) != 3 {
		names := make([]string, len(result.Files))
		for i, f := range result.Files {
			names[i] = f.Name
		}
		t.Fatalf("expected 3 files after resume (c,d,e), got %d: %v", len(result.Files), names)
	}
	for _, f := range result.Files {
		if f.Name == "a.txt" || f.Name == "b.txt" {
			t.Errorf("file %q should have been skipped by resume path", f.Name)
		}
	}
}

// TestScanResumeIgnoresCompletedCheckpoint verifies that a 'completed'
// checkpoint is NOT resumable. After a successful scan, Resume=true should
// start a fresh checkpoint with empty ResumedFrom.
func TestScanResumeIgnoresCompletedCheckpoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "source")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	st, err := store.Open(ctx, filepath.Join(tmp, "project.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	storageID := "completed-test"

	// First scan: complete successfully (checkpoint → "completed").
	svc := NewScanService(st)
	_, err = svc.Scan(ctx, ScanInput{
		Root: root, StorageID: storageID,
		Workers: 1, HashAttempts: 1, HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}

	// Verify no resumable checkpoint exists (LastCheckpoint only returns
	// running/aborted, not completed).
	_, err = st.LastCheckpoint(ctx, storageID)
	if err == nil {
		t.Fatal("expected ErrNotFound from LastCheckpoint after completed scan, got nil")
	}
	if !os.IsNotExist(err) && err != store.ErrNotFound {
		// Some stores may return a different not-found error; the key
		// point is that NO checkpoint is returned.
		t.Logf("LastCheckpoint returned non-nil error (expected): %v", err)
	}

	// Second scan with Resume=true: should NOT resume (completed checkpoint
	// is not resumable). A fresh checkpoint should be started.
	result, err := svc.Scan(ctx, ScanInput{
		Root: root, StorageID: storageID, Resume: true,
		Workers: 1, HashAttempts: 1, HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("resume scan after completed: %v", err)
	}

	if result.ResumedFrom != "" {
		t.Errorf("ResumedFrom = %q, want empty (completed checkpoint not resumable)", result.ResumedFrom)
	}
	if result.ResumedCount != 0 {
		t.Errorf("ResumedCount = %d, want 0", result.ResumedCount)
	}
	// All files should be discovered (no skip).
	if len(result.Files) != 2 {
		t.Fatalf("expected 2 files (no resume skip), got %d", len(result.Files))
	}
}

// TestScanResumeWithoutCheckpointStartsFresh verifies that Resume=true
// when no checkpoint exists at all starts a fresh scan (no error).
func TestScanResumeWithoutCheckpointStartsFresh(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "source")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(ctx, filepath.Join(tmp, "project.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	svc := NewScanService(st)
	result, err := svc.Scan(ctx, ScanInput{
		Root: root, StorageID: "no-checkpoint-test", Resume: true,
		Workers: 1, HashAttempts: 1, HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("Scan with resume but no checkpoint: %v", err)
	}

	if result.ResumedFrom != "" {
		t.Errorf("ResumedFrom = %q, want empty (no checkpoint existed)", result.ResumedFrom)
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}
}
