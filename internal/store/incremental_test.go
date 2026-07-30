package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func seedStorage(t *testing.T, st *SQLiteStore, id string) {
	t.Helper()
	if err := st.RegisterStorage(context.Background(), domain.Storage{
		ID: id, RootPath: "/vol", Kind: "filesystem", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("register storage: %v", err)
	}
}

func seedFile(t *testing.T, st *SQLiteStore, storageID, path string, size int64, hash string) {
	t.Helper()
	files := []domain.FileInstance{{
		StorageID: storageID, Path: path, Name: filepath.Base(path),
		Size: size, Mode: 0o644, ModifiedAt: time.Now().UTC(),
		QuickHash: hash, ContentSHA256: hash, DiscoveredAt: time.Now().UTC(),
	}}
	if _, err := st.UpsertFiles(context.Background(), files); err != nil {
		t.Fatalf("upsert file: %v", err)
	}
}

// TestListFileMetadataReturnsActiveFiles verifies that ListFileMetadata
// returns only active files (not missing ones).
func TestListFileMetadataReturnsActiveFiles(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")
	seedFile(t, st, "s1", "/vol/a.txt", 100, "hashA")
	seedFile(t, st, "s1", "/vol/b.txt", 200, "hashB")

	metas, err := st.ListFileMetadata(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("expected 2 active files, got %d", len(metas))
	}

	// Mark a.txt as missing.
	n, err := st.MarkFilesMissing(ctx, "s1", []string{"/vol/b.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 file marked missing, got %d", n)
	}

	// Now only b.txt should be active.
	metas, _ = st.ListFileMetadata(ctx, "s1")
	if len(metas) != 1 {
		t.Fatalf("expected 1 active file after missing, got %d", len(metas))
	}
	if metas[0].Path != "/vol/b.txt" {
		t.Fatalf("expected b.txt, got %s", metas[0].Path)
	}
}

// TestMarkFilesMissingIdempotent verifies that calling MarkFilesMissing
// twice with the same seen-paths doesn't re-mark files.
func TestMarkFilesMissingIdempotent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")
	seedFile(t, st, "s1", "/vol/a.txt", 100, "h")
	seedFile(t, st, "s1", "/vol/b.txt", 200, "h")

	// First scan: saw a.txt only → b.txt marked missing.
	n1, _ := st.MarkFilesMissing(ctx, "s1", []string{"/vol/a.txt"})
	if n1 != 1 {
		t.Fatalf("expected 1 missing, got %d", n1)
	}

	// Second scan: saw a.txt only again → no new missing files.
	n2, _ := st.MarkFilesMissing(ctx, "s1", []string{"/vol/a.txt"})
	if n2 != 0 {
		t.Fatalf("expected 0 new missing, got %d", n2)
	}
}

func TestListFilesAndFormatsExcludeMissingButPreserveAuditRow(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "active-snapshot.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.RegisterStorage(ctx, domain.Storage{ID: "s1", RootPath: "/vol", Kind: "filesystem", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "/vol/active.txt", Name: "active.txt", Size: 1, ModifiedAt: time.Now(), DiscoveredAt: time.Now()},
		{StorageID: "s1", Path: "/vol/missing.txt", Name: "missing.txt", Size: 1, ModifiedAt: time.Now(), DiscoveredAt: time.Now()},
	}
	ids, err := st.UpsertFiles(ctx, files)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err := st.SaveFormat(ctx, id, domain.FormatInfo{Format: "text", Category: domain.CategoryDocument}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.MarkFilesMissing(ctx, "s1", []string{"/vol/active.txt"}); err != nil {
		t.Fatal(err)
	}
	active, err := st.ListFiles(ctx, "s1")
	if err != nil || len(active) != 1 || active[0].Path != "/vol/active.txt" {
		t.Fatalf("active files = %#v err=%v", active, err)
	}
	formats, err := st.ListFormats(ctx, "s1")
	if err != nil || len(formats) != 1 || formats[0].Path != "/vol/active.txt" {
		t.Fatalf("active formats = %#v err=%v", formats, err)
	}
	var auditRows int
	if err := st.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM file_instances WHERE file_status='missing'").Scan(&auditRows); err != nil || auditRows != 1 {
		t.Fatalf("missing audit rows = %d err=%v", auditRows, err)
	}
}

func TestMarkFilesUnavailableUsesSeenTableAndCanRecoverActive(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "partial.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seedStorage(t, st, "partial")
	seedFile(t, st, "partial", "/vol/seen.txt", 1, "")
	seedFile(t, st, "partial", "/vol/unseen.txt", 1, "")
	count, err := st.MarkFilesUnavailable(ctx, "partial", []string{"/vol/seen.txt"})
	if err != nil || count != 1 {
		t.Fatalf("unavailable count=%d err=%v", count, err)
	}
	active, err := st.ListFiles(ctx, "partial")
	if err != nil || len(active) != 1 || active[0].Path != "/vol/seen.txt" {
		t.Fatalf("active snapshot=%#v err=%v", active, err)
	}
	if err := st.MarkFileActive(ctx, "partial", "/vol/unseen.txt"); err != nil {
		t.Fatal(err)
	}
	active, err = st.ListFiles(ctx, "partial")
	if err != nil || len(active) != 2 {
		t.Fatalf("reactivated snapshot=%#v err=%v", active, err)
	}
}

// TestMarkFileActive restores a previously-missing file.
func TestMarkFileActive(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")
	seedFile(t, st, "s1", "/vol/a.txt", 100, "h")

	// Mark as missing.
	st.MarkFilesMissing(ctx, "s1", []string{}) // a.txt is now missing

	// Restore.
	if err := st.MarkFileActive(ctx, "s1", "/vol/a.txt"); err != nil {
		t.Fatal(err)
	}

	metas, _ := st.ListFileMetadata(ctx, "s1")
	if len(metas) != 1 {
		t.Fatalf("expected 1 active file after restore, got %d", len(metas))
	}
}

// TestCheckpointCRUD verifies the checkpoint lifecycle.
func TestCheckpointCRUD(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	// Start checkpoint.
	id, err := st.StartCheckpoint(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero checkpoint ID")
	}

	// Update progress.
	if err := st.UpdateCheckpoint(ctx, id, "/vol/last.txt", 42); err != nil {
		t.Fatal(err)
	}

	// LastCheckpoint should return our running checkpoint.
	cp, err := st.LastCheckpoint(ctx, "s1")
	if err != nil {
		t.Fatalf("last checkpoint: %v", err)
	}
	if cp.ID != id {
		t.Fatalf("expected checkpoint %d, got %d", id, cp.ID)
	}
	if cp.LastScannedPath != "/vol/last.txt" {
		t.Fatalf("expected last path /vol/last.txt, got %s", cp.LastScannedPath)
	}
	if cp.ScannedCount != 42 {
		t.Fatalf("expected count 42, got %d", cp.ScannedCount)
	}
	if cp.Status != "running" {
		t.Fatalf("expected running, got %s", cp.Status)
	}

	// Complete it.
	if err := st.CompleteCheckpoint(ctx, id, "completed"); err != nil {
		t.Fatal(err)
	}

	// Now LastCheckpoint should return ErrNotFound (no running checkpoint).
	_, err = st.LastCheckpoint(ctx, "s1")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after completion, got %v", err)
	}
}

// TestLastCheckpointNotFound verifies ErrNotFound when no checkpoint exists.
func TestLastCheckpointNotFound(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	_, err := st.LastCheckpoint(ctx, "s1")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestCheckpointAbort verifies that 'aborted' status is handled same as
// 'completed' for LastCheckpoint purposes (not returned as running).
func TestCheckpointAbort(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")

	id, _ := st.StartCheckpoint(ctx, "s1")
	st.CompleteCheckpoint(ctx, id, "aborted")

	_, err := st.LastCheckpoint(ctx, "s1")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for aborted checkpoint, got %v", err)
	}
}
