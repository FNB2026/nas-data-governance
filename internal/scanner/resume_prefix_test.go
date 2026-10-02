package scanner

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

type observedEntry struct {
	os.DirEntry
	info func() (fs.FileInfo, error)
}

func (e observedEntry) Info() (fs.FileInfo, error) { return e.info() }

// Every boundary must produce exactly the full scan's suffix, while issuing
// zero ReadDir/Info operations for already completed subtrees and files.
func TestResumePrunesDurablePrefixBeforeIO(t *testing.T) {
	root := t.TempDir()
	names := []string{"a.txt", "a/early/x", "a/mid/first", "a/mid/last", "a/z", "a0/x", "b/one", "b/two", "照片/一", "照片/二"}
	for _, name := range names {
		writeFile(t, filepath.Join(root, name), "test")
	}
	_, full := collectFiles(t, Options{Root: root})
	boundaries := []string{filepath.Join(root, "a/mid/deleted"), filepath.Join(root, "a/mid/"), filepath.Join(root, "0")}
	for _, f := range full {
		boundaries = append(boundaries, f.Path)
	}
	for _, boundary := range boundaries {
		t.Run(filepath.Base(boundary), func(t *testing.T) {
			var got, want []string
			for _, f := range full {
				if f.Path > boundary {
					want = append(want, f.Path)
				}
			}
			read := func(dir string) ([]os.DirEntry, error) {
				prefix := dir + string(filepath.Separator)
				if dir != root && prefix <= boundary && !strings.HasPrefix(boundary, prefix) {
					t.Fatalf("entered completed subtree %q", dir)
				}
				entries, err := os.ReadDir(dir)
				for i, entry := range entries {
					entry := entry
					path := filepath.Join(dir, entry.Name())
					entries[i] = observedEntry{DirEntry: entry, info: func() (fs.FileInfo, error) {
						prefix := path + string(filepath.Separator)
						skipped := !entry.IsDir() && path <= boundary || entry.IsDir() && prefix <= boundary && !strings.HasPrefix(boundary, prefix)
						if skipped {
							t.Fatalf("metadata read in completed prefix: %q", path)
						}
						return entry.Info()
					}}
				}
				return entries, err
			}
			stats, err := scan(context.Background(), Options{Root: root, ResumePath: boundary}, func(f domain.FileInstance) error {
				got = append(got, f.Path)
				return nil
			}, read)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("suffix mismatch: got %v want %v", got, want)
			}
			if stats.FilesScanned != len(want) {
				t.Fatalf("count %d want %d", stats.FilesScanned, len(want))
			}
		})
	}
}

func TestResumeReadsOnlyBoundaryAncestorsAndLaterSubtrees(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "a", "deep", "done"), "done")
	writeFile(t, filepath.Join(root, "b", "next"), "next")
	reads := 0
	stats, err := scan(context.Background(), Options{Root: root, ResumePath: filepath.Join(root, "a", "deep", "done")}, func(domain.FileInstance) error { return nil }, func(dir string) ([]os.DirEntry, error) {
		reads++
		return os.ReadDir(dir)
	})
	if err != nil {
		t.Fatal(err)
	}
	// The boundary's ancestor chain is required, even though its final file
	// was already processed. The following sibling must still be traversed.
	if stats.FilesScanned != 1 || reads != 4 {
		t.Fatalf("files=%d directories=%d", stats.FilesScanned, reads)
	}
	// With a boundary after the whole subtree, only root and b are read.
	reads = 0
	stats, err = scan(context.Background(), Options{Root: root, ResumePath: filepath.Join(root, "a0")}, func(domain.FileInstance) error { return nil }, func(dir string) ([]os.DirEntry, error) {
		reads++
		if strings.HasPrefix(dir, filepath.Join(root, "a")+string(filepath.Separator)) || dir == filepath.Join(root, "a") {
			t.Fatal("read completed subtree")
		}
		return os.ReadDir(dir)
	})
	if err != nil || reads != 2 || stats.FilesScanned != 1 {
		t.Fatalf("err=%v directories=%d files=%d", err, reads, stats.FilesScanned)
	}
}
