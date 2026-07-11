package executor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"nas-data-governance/internal/domain"
)

func writeSample(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotReturnsMissingForAbsentFile(t *testing.T) {
	_, err := Snapshot(filepath.Join(t.TempDir(), "absent"), false)
	if err != ErrFileMissing {
		t.Fatalf("expected ErrFileMissing, got %v", err)
	}
}

func TestSnapshotWithoutHashSkipsSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	writeSample(t, path, "hello")
	snap, err := Snapshot(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Hash != "" {
		t.Fatalf("expected empty hash without verifyHash, got %q", snap.Hash)
	}
	if snap.Size != 5 {
		t.Fatalf("expected size 5, got %d", snap.Size)
	}
}

func TestSnapshotWithHashComputesSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	writeSample(t, path, "hello")
	snap, err := Snapshot(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Hash == "" {
		t.Fatal("expected non-empty hash with verifyHash=true")
	}
}

func TestCheckNoneForFreshFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	writeSample(t, path, "hello")
	snap, err := Snapshot(path, true)
	if err != nil {
		t.Fatal(err)
	}
	expected := domain.FileInstance{
		Path: path, Size: snap.Size, ModifiedAt: snap.ModifiedAt,
		Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash,
	}
	if got := Check(expected, snap); got != StaleNone {
		t.Fatalf("expected StaleNone, got %q", got)
	}
}

func TestCheckDetectsSizeChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	writeSample(t, path, "hello")
	snap, err := Snapshot(path, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := domain.FileInstance{Size: snap.Size + 1, ModifiedAt: snap.ModifiedAt}
	if got := Check(expected, snap); got != StaleSize {
		t.Fatalf("expected StaleSize, got %q", got)
	}
}

func TestCheckDetectsMtimeChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	writeSample(t, path, "hello")
	snap, err := Snapshot(path, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := domain.FileInstance{Size: snap.Size, ModifiedAt: snap.ModifiedAt.Add(time.Hour)}
	if got := Check(expected, snap); got != StaleMtime {
		t.Fatalf("expected StaleMtime, got %q", got)
	}
}

func TestCheckDetectsInodeChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	writeSample(t, path, "hello")
	snap, err := Snapshot(path, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := domain.FileInstance{Size: snap.Size, ModifiedAt: snap.ModifiedAt, Inode: snap.Inode + 1}
	if got := Check(expected, snap); got != StaleInode {
		t.Fatalf("expected StaleInode, got %q", got)
	}
}

func TestCheckDetectsHashChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	writeSample(t, path, "hello")
	snap, err := Snapshot(path, true)
	if err != nil {
		t.Fatal(err)
	}
	expected := domain.FileInstance{
		Size: snap.Size, ModifiedAt: snap.ModifiedAt, Device: snap.Device, Inode: snap.Inode,
		ContentSHA256: "deadbeef",
	}
	if got := Check(expected, snap); got != StaleHash {
		t.Fatalf("expected StaleHash, got %q", got)
	}
}

func TestCheckSkipsHashWhenEitherSideEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	writeSample(t, path, "hello")
	snap, err := Snapshot(path, true)
	if err != nil {
		t.Fatal(err)
	}
	// Expected has empty ContentSHA256 → hash check is skipped even when
	// current has a hash. This lets quick-hash-only plans pass stale check.
	expected := domain.FileInstance{
		Size: snap.Size, ModifiedAt: snap.ModifiedAt, Device: snap.Device, Inode: snap.Inode,
	}
	if got := Check(expected, snap); got != StaleNone {
		t.Fatalf("expected StaleNone when expected hash is empty, got %q", got)
	}
}

func TestCheckStaleComposedForm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	writeSample(t, path, "hello")
	snap, err := Snapshot(path, true)
	if err != nil {
		t.Fatal(err)
	}
	fresh := domain.FileInstance{
		Path: path, Size: snap.Size, ModifiedAt: snap.ModifiedAt,
		Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash,
	}
	if got, err := CheckStale(fresh, true); err != nil || got != StaleNone {
		t.Fatalf("fresh file: got=%q err=%v", got, err)
	}
	if got, err := CheckStale(domain.FileInstance{Path: filepath.Join(t.TempDir(), "absent")}, false); err != nil || got != StaleMissing {
		t.Fatalf("missing file: got=%q err=%v", got, err)
	}
}
