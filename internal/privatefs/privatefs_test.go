package privatefs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCreateTightensDirectoryAndFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode assertion")
	}
	dir := filepath.Join(t.TempDir(), "var")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plan.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	dirInfo, _ := os.Stat(dir)
	fileInfo, _ := os.Stat(path)
	if got := dirInfo.Mode().Perm(); got != DirectoryMode {
		t.Fatalf("directory mode = %o, want %o", got, DirectoryMode)
	}
	if got := fileInfo.Mode().Perm(); got != FileMode {
		t.Fatalf("file mode = %o, want %o", got, FileMode)
	}
}

func TestCreateRejectsOutputSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics")
	}
	dir := filepath.Join(t.TempDir(), "var")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "index.jsonl")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(link); err == nil {
		t.Fatal("expected symlink refusal")
	}
}
