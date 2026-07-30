package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

	// Simulate an aborted scan that got through a.txt and b.txt (2 files).
	// The checkpoint's last_scanned_path is b.txt — the most recently
	// scanned file. Resume uses path <= ResumePath, so b.txt (and a.txt)
	// are skipped, and scanning continues from c.txt onward.
	cpID, err := st.StartCheckpoint(ctx, storageID)
	if err != nil {
		t.Fatalf("StartCheckpoint: %v", err)
	}
	resumePath := filepath.Join(root, "b.txt")
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

	// Scanner skips files whose path sorts at or before ResumePath (<=).
	// a.txt and b.txt are skipped; c.txt, d.txt, e.txt are discovered (3 files).
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

// checkpointUpdateRecord captures a single UpdateCheckpoint call's arguments
// so tests can verify that scanned_count accumulates across resume cycles.
type checkpointUpdateRecord struct {
	checkpointID int64
	lastPath     string
	scannedCount int
}

// countingScanStore wraps a ScanStore and records every UpdateCheckpoint
// invocation. It is used by TestScanResumeCumulativeCountNoRegression to
// prove that scanned_count = ResumedCount + int(count) rather than just
// int(count), which would regress across multiple interrupt→resume cycles.
type countingScanStore struct {
	ScanStore
	mu      sync.Mutex
	updates []checkpointUpdateRecord
}

func (c *countingScanStore) UpdateCheckpoint(ctx context.Context, checkpointID int64, lastPath string, scannedCount int) error {
	c.mu.Lock()
	c.updates = append(c.updates, checkpointUpdateRecord{checkpointID, lastPath, scannedCount})
	c.mu.Unlock()
	return c.ScanStore.UpdateCheckpoint(ctx, checkpointID, lastPath, scannedCount)
}

func (c *countingScanStore) snapshot() []checkpointUpdateRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]checkpointUpdateRecord, len(c.updates))
	copy(out, c.updates)
	return out
}

// TestScanResumeCumulativeCountNoRegression verifies that the checkpoint's
// scanned_count accumulates monotonically across a full
// interrupt→resume→interrupt→resume cycle. Before the fix, UpdateCheckpoint
// used int(count) (the session-local counter that resets to 0 on each Scan
// call), which caused the stored count to regress to ~1000 on every resume
// instead of growing. The fix uses result.ResumedCount + int(count), so the
// count grows: 100 → 1100 → 2100.
//
// The test creates 2101 files and uses a cancelling hash function to
// interrupt the first resume after exactly 1000 hashes, then lets the second
// resume complete. The countingScanStore captures every UpdateCheckpoint call
// so we can assert the cumulative count at each boundary.
func TestScanResumeCumulativeCountNoRegression(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "source")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	// 2101 files: f0000.txt through f2100.txt.
	// Layout: 100 initial (f0000-f0099) + 1001 for resume #1 (f0100-f1100)
	// + 1001 for resume #2 (f1100-f2100).
	const totalFiles = 2101
	for i := 0; i < totalFiles; i++ {
		name := fmt.Sprintf("f%04d.txt", i)
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	st, err := store.Open(ctx, filepath.Join(tmp, "project.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	storageID := "cumulative-count-test"
	if err := st.RegisterStorage(ctx, domain.Storage{
		ID: storageID, RootPath: root, Kind: "filesystem",
	}); err != nil {
		t.Fatal(err)
	}

	// --- Step 1: simulate first interrupt after 100 files ---
	cpID, err := st.StartCheckpoint(ctx, storageID)
	if err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(root, "f0099.txt")
	if err := st.UpdateCheckpoint(ctx, cpID, firstPath, 100); err != nil {
		t.Fatal(err)
	}
	if err := st.CompleteCheckpoint(ctx, cpID, "aborted"); err != nil {
		t.Fatal(err)
	}

	// Wrap the store to capture UpdateCheckpoint calls.
	cs := &countingScanStore{ScanStore: st}

	// --- Step 2: resume scan #1 — should accumulate from 100 ---
	// Use a hash function that cancels the context after 1000 calls,
	// interrupting the scan after the 1000-file checkpoint boundary.
	scan1Ctx, scan1Cancel := context.WithCancel(ctx)
	var hashCount1 int32
	hashFunc1 := func(path string, size int64) (string, error) {
		if atomic.AddInt32(&hashCount1, 1) >= 1000 {
			scan1Cancel()
		}
		return "fake", nil
	}
	svc1 := NewScanServiceWithHashFunc(cs, hashFunc1, hashFunc1)
	_, err = svc1.Scan(scan1Ctx, ScanInput{
		Root: root, StorageID: storageID, Resume: true,
		Workers: 1, HashAttempts: 1, HashRetryDelay: 0,
	})
	// Context cancellation is the expected interrupt mechanism.
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("resume scan #1: unexpected error: %v", err)
	}

	updates1 := cs.snapshot()
	// At the 1000-file boundary, UpdateCheckpoint must have been called
	// with 100 + 1000 = 1100, NOT just 1000 (which would be a regression).
	found1100 := false
	for _, u := range updates1 {
		if u.scannedCount == 1100 {
			found1100 = true
		}
		// No UpdateCheckpoint call should have a count below the
		// ResumedCount (100). If it does, the fix is not applied.
		if u.scannedCount < 100 {
			t.Errorf("UpdateCheckpoint count %d < ResumedCount 100 (regression): %+v", u.scannedCount, u)
		}
	}
	if !found1100 {
		t.Errorf("expected UpdateCheckpoint with count 1100 (100 resumed + 1000 new), got: %+v", updates1)
	}

	// The checkpoint in the store should now have count=1100.
	cp1, err := st.LastCheckpoint(ctx, storageID)
	if err != nil {
		t.Fatalf("LastCheckpoint after scan #1: %v", err)
	}
	if cp1.ScannedCount != 1100 {
		t.Errorf("checkpoint count after scan #1 = %d, want 1100", cp1.ScannedCount)
	}

	// --- Step 3: resume scan #2 — should accumulate from 1100 ---
	cs2 := &countingScanStore{ScanStore: st}
	hashFunc2 := func(path string, size int64) (string, error) {
		return "fake", nil // no cancellation; let it complete
	}
	svc2 := NewScanServiceWithHashFunc(cs2, hashFunc2, hashFunc2)
	result2, err := svc2.Scan(ctx, ScanInput{
		Root: root, StorageID: storageID, Resume: true,
		Workers: 1, HashAttempts: 1, HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("resume scan #2: %v", err)
	}

	// ResumedCount must be 1100 (from the checkpoint left by scan #1).
	if result2.ResumedCount != 1100 {
		t.Errorf("scan #2 ResumedCount = %d, want 1100", result2.ResumedCount)
	}

	updates2 := cs2.snapshot()
	// At the 1000-file boundary, UpdateCheckpoint must have been called
	// with 1100 + 1000 = 2100, NOT just 1000.
	found2100 := false
	for _, u := range updates2 {
		if u.scannedCount == 2100 {
			found2100 = true
		}
		// No UpdateCheckpoint call should have a count below the
		// ResumedCount (1100). If it does, the fix is not applied.
		if u.scannedCount < 1100 {
			t.Errorf("UpdateCheckpoint count %d < ResumedCount 1100 (regression): %+v", u.scannedCount, u)
		}
	}
	if !found2100 {
		t.Errorf("expected UpdateCheckpoint with count 2100 (1100 resumed + 1000 new), got: %+v", updates2)
	}
}
