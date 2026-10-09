package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestSelectedPathsPrunesUnrelatedTreesAndRejectsEscape(t *testing.T) {
	root := t.TempDir()
	selected := filepath.Join(root, "needed", "file")
	os.Mkdir(filepath.Dir(selected), 0700)
	os.WriteFile(selected, []byte("x"), 0600)
	unrelated := filepath.Join(root, "unrelated")
	os.Mkdir(unrelated, 0700)
	os.WriteFile(filepath.Join(unrelated, "file"), []byte("y"), 0600)
	calls := 0
	opts := Options{Root: root, StorageID: "selected", SelectedPaths: map[string]bool{selected: true}}
	stats, err := scan(context.Background(), opts, func(f domain.FileInstance) error {
		calls++
		if f.Path != selected {
			t.Fatal("unselected visit")
		}
		return nil
	}, func(path string) ([]os.DirEntry, error) {
		if path == unrelated {
			t.Fatal("enumerated unrelated tree")
		}
		return os.ReadDir(path)
	})
	if err != nil || calls != 1 || stats.FilesScanned != 1 || stats.DirsVisited != 2 {
		t.Fatalf("selected traversal: %+v %v", stats, err)
	}
	opts.SelectedPaths = map[string]bool{filepath.Join(filepath.Dir(root), "escape"): true}
	if _, err := Scan(context.Background(), opts, func(domain.FileInstance) error { t.Fatal("outside visit"); return nil }); err == nil {
		t.Fatal("accepted escape")
	}
}
