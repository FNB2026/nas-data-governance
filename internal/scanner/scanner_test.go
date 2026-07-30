package scanner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// writeFile creates a file with content, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// collectFiles runs a scan and returns the collected FileInstances.
func collectFiles(t *testing.T, opts Options) (Stats, []domain.FileInstance) {
	t.Helper()
	var files []domain.FileInstance
	stats, err := Scan(context.Background(), opts, func(f domain.FileInstance) error {
		files = append(files, f)
		return nil
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return stats, files
}

func TestScanFindsFilesInNestedDirs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "a")
	writeFile(t, filepath.Join(root, "sub", "b.txt"), "b")
	writeFile(t, filepath.Join(root, "sub", "deep", "c.txt"), "c")

	stats, files := collectFiles(t, Options{Root: root})

	if stats.FilesScanned != 3 {
		t.Fatalf("expected 3 files, got %d", stats.FilesScanned)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 collected files, got %d", len(files))
	}
	// All paths should be under root.
	for _, f := range files {
		if !strings.HasPrefix(f.Path, root) {
			t.Errorf("path %q not under root %q", f.Path, root)
		}
	}
}

func TestScanPopulatesPhysicalIdentity(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "a")

	_, files := collectFiles(t, Options{Root: root, StorageID: "s1"})
	if len(files) != 1 {
		t.Fatalf("expected one file, got %d", len(files))
	}
	f := files[0]
	if f.Device == 0 || f.Inode == 0 {
		t.Skip("filesystem does not expose device/inode identity")
	}
	wantReliable := physicalIdentityReliable(root)
	if f.Physical.Reliable != wantReliable {
		t.Fatalf("physical identity reliability = %t, want %t", f.Physical.Reliable, wantReliable)
	}
	if f.Physical.Device != f.Device || f.Physical.Inode != f.Inode {
		t.Fatalf("physical identity does not match legacy fields: %#v", f)
	}
}

func TestScanSkipsExcludedNames(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "keep.txt"), "keep")
	writeFile(t, filepath.Join(root, ".snapshot", "skip.txt"), "skip")
	writeFile(t, filepath.Join(root, "@eaDir", "thumb.jpg"), "thumb")
	writeFile(t, filepath.Join(root, "normal", "file.txt"), "normal")

	stats, files := collectFiles(t, Options{
		Root:          root,
		ExcludedNames: DefaultExclusions(),
	})

	// Should find keep.txt and normal/file.txt = 2 files.
	if stats.FilesScanned != 2 {
		t.Fatalf("expected 2 files (excluded dirs skipped), got %d", stats.FilesScanned)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 collected files, got %d", len(files))
	}
}

func TestScanSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "real.txt"), "real")

	// Create a symlink to the real file.
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(filepath.Join(root, "real.txt"), link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	stats, files := collectFiles(t, Options{Root: root})

	// Only real.txt should be found; the symlink must be skipped.
	if stats.FilesScanned != 1 {
		t.Fatalf("expected 1 file (symlink skipped), got %d", stats.FilesScanned)
	}
	if len(files) != 1 || files[0].Name != "real.txt" {
		t.Fatalf("expected only real.txt, got %v", files)
	}
}

func TestScanContextCancellation(t *testing.T) {
	root := t.TempDir()
	// Create enough files to ensure the scan isn't instant.
	for i := 0; i < 100; i++ {
		writeFile(t, filepath.Join(root, "file", "f"+padZero(i, 3)+".txt"), "x")
	}

	// Cancel before scanning.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var count int
	stats, err := Scan(ctx, Options{Root: root}, func(f domain.FileInstance) error {
		count++
		return nil
	})

	// Should return context error immediately (or after 0 files).
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context canceled, got %v", err)
	}
	_ = stats
	_ = count
}

func TestScanSingleDirFailureDoesNotAbort(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "good", "a.txt"), "a")
	writeFile(t, filepath.Join(root, "bad", "b.txt"), "b")

	// Remove read permission on the "bad" directory to cause ReadDir failure.
	// Skip on non-Unix or if running as root (root bypasses permissions).
	if runtime.GOOS == "windows" {
		t.Skip("permission test not applicable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root, permission test not applicable")
	}

	badDir := filepath.Join(root, "bad")
	if err := os.Chmod(badDir, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(badDir, 0o755) // restore so cleanup works

	stats, files := collectFiles(t, Options{Root: root})

	// The scan should continue past the "bad" directory error.
	// "good/a.txt" should still be found.
	found := false
	for _, f := range files {
		if f.Name == "a.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected to find a.txt despite bad dir error")
	}

	// The "bad" directory error should be recorded as non-fatal.
	if len(stats.Errors) == 0 {
		t.Fatal("expected at least 1 non-fatal error for the bad directory")
	}
}

func TestScanResumePathSkipsEarlierFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "a")
	writeFile(t, filepath.Join(root, "b.txt"), "b")
	writeFile(t, filepath.Join(root, "c.txt"), "c")

	// Resume from "b.txt": should skip "a.txt" and find "b.txt" + "c.txt".
	resumePath := filepath.Join(root, "b.txt")
	stats, files := collectFiles(t, Options{Root: root, ResumePath: resumePath})

	if stats.FilesScanned != 2 {
		t.Fatalf("expected 2 files after resume, got %d", stats.FilesScanned)
	}
	for _, f := range files {
		if f.Name == "a.txt" {
			t.Fatal("a.txt should have been skipped by resume path")
		}
	}
}

func TestScanReturnsStatsWithDirCount(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "a")
	writeFile(t, filepath.Join(root, "sub1", "b.txt"), "b")
	writeFile(t, filepath.Join(root, "sub1", "deep", "c.txt"), "c")
	writeFile(t, filepath.Join(root, "sub2", "d.txt"), "d")

	stats, _ := collectFiles(t, Options{Root: root})

	// root + sub1 + sub1/deep + sub2 = 4 directories.
	if stats.DirsVisited != 4 {
		t.Fatalf("expected 4 dirs visited, got %d", stats.DirsVisited)
	}
	if stats.FilesScanned != 4 {
		t.Fatalf("expected 4 files scanned, got %d", stats.FilesScanned)
	}
}

func TestScanPopulatesFileInstanceFields(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "test.txt")
	os.WriteFile(src, []byte("hello world"), 0o644)
	// Set a known mtime.
	mtime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	os.Chtimes(src, mtime, mtime)

	stats, files := collectFiles(t, Options{Root: root, StorageID: "test-storage"})
	_ = stats

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	f := files[0]
	if f.StorageID != "test-storage" {
		t.Errorf("StorageID: expected test-storage, got %s", f.StorageID)
	}
	if f.Name != "test.txt" {
		t.Errorf("Name: expected test.txt, got %s", f.Name)
	}
	if f.Size != 11 {
		t.Errorf("Size: expected 11, got %d", f.Size)
	}
	if !f.ModifiedAt.Equal(mtime) {
		t.Errorf("ModifiedAt: expected %v, got %v", mtime, f.ModifiedAt)
	}
	if f.Path != src {
		t.Errorf("Path: expected %s, got %s", src, f.Path)
	}
	// Device and Inode should be non-zero on Unix.
	if f.Device == 0 || f.Inode == 0 {
		// Could be zero on non-Unix; just log.
		t.Logf("Device=%d Inode=%d (may be zero on non-Unix)", f.Device, f.Inode)
	}
}

func TestScanVisitErrorStopsScan(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a.txt"), "a")
	writeFile(t, filepath.Join(root, "b.txt"), "b")

	// Return an error from visit; scan should stop.
	visitErr := context.DeadlineExceeded
	_, err := Scan(context.Background(), Options{Root: root}, func(f domain.FileInstance) error {
		return visitErr
	})
	if err == nil {
		t.Fatal("expected scan to return visit error")
	}
	if err != visitErr {
		t.Fatalf("expected %v, got %v", visitErr, err)
	}
}

func TestFormatErrorsEmpty(t *testing.T) {
	s := Stats{}
	if s.FormatErrors() != "" {
		t.Fatalf("expected empty string for no errors, got %q", s.FormatErrors())
	}
}

func TestFormatErrorsNonEmpty(t *testing.T) {
	s := Stats{Errors: []ErrorEntry{
		{Path: "/foo", Error: context.DeadlineExceeded},
	}}
	out := s.FormatErrors()
	if !strings.Contains(out, "1 non-fatal errors") {
		t.Fatalf("expected error count in output, got %q", out)
	}
	if strings.Contains(out, "/foo") || !strings.Contains(out, "paths omitted") {
		t.Fatalf("sensitive path leaked in output: %q", out)
	}
}

func padZero(n int, width int) string {
	s := string(rune('0' + n))
	for len(s) < width {
		s = "0" + s
	}
	return s
}

func TestMarkPathSeenUsesExactStoragePathPair(t *testing.T) {
	cases := []struct {
		name            string
		storageID, path string
		wantAlreadySeen bool
	}{
		{"first pair", "s1", "/a/b.txt", false},
		{"same pair again", "s1", "/a/b.txt", true},
		{"different path", "s1", "/a/c.txt", false},
		{"different storage", "s2", "/a/b.txt", false},
		{"empty values first", "", "", false},
		{"empty values again", "", "", true},
	}
	seen := make(map[scanPathKey]struct{})
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := markPathSeen(seen, c.storageID, c.path); got != c.wantAlreadySeen {
				t.Fatalf("markPathSeen(%q,%q)=%v want %v",
					c.storageID, c.path, got, c.wantAlreadySeen)
			}
		})
	}
}

// TestScanDoesNotMergeHardLinks verifies that hard links — same inode at
// different paths — are NOT deduplicated. They are distinct file instances
// and must each be visited. This guards against an earlier design that
// keyed on inode.
func TestScanDoesNotMergeHardLinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hard links not supported on Windows")
	}
	root := t.TempDir()
	original := filepath.Join(root, "original.txt")
	writeFile(t, original, "shared content")
	link := filepath.Join(root, "link.txt")
	if err := os.Link(original, link); err != nil {
		t.Skipf("hard link not supported: %v", err)
	}

	stats, files := collectFiles(t, Options{Root: root})

	if stats.FilesScanned != 2 {
		t.Fatalf("expected 2 files (hard link distinct), got %d", stats.FilesScanned)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 collected files, got %d", len(files))
	}
	// Both files must have the same inode but different paths.
	if files[0].Inode != files[1].Inode {
		t.Fatalf("expected same inode for hard link pair, got %d vs %d",
			files[0].Inode, files[1].Inode)
	}
	paths := map[string]bool{files[0].Path: true, files[1].Path: true}
	if !paths[original] || !paths[link] {
		t.Fatalf("expected both original and link paths, got %v", paths)
	}
}
