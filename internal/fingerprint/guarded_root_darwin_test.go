//go:build darwin

package fingerprint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestGuardedContextDarwinReadsWithinRegisteredRoot(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "synthetic")
	content := []byte("disposable-macos-boundary")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal("synthetic source setup")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal("synthetic source stat")
	}
	sum := sha256.Sum256(content)
	file := domain.FileInstance{Path: path, Size: int64(len(content)), ModifiedAt: info.ModTime(), ContentSHA256: hex.EncodeToString(sum[:])}
	got, err := GuardedContext(context.Background(), root, file)
	if err != nil || got != file.ContentSHA256 {
		t.Fatalf("Darwin freshness read failed: hash=%q err=%v", got, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, content) {
		t.Fatal("verification changed synthetic source")
	}
}

func TestGuardedContextDarwinRejectsInnerSymlink(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	target := filepath.Join(outside, "outside.bin")
	if err := os.WriteFile(target, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	file := domain.FileInstance{Path: link, Size: 7, ModifiedAt: info.ModTime()}
	if got, err := GuardedContext(context.Background(), root, file); err == nil || got != "" {
		t.Fatalf("symlink below registered root was followed: hash=%q err=%v", got, err)
	}
}
