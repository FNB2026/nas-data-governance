//go:build darwin

package fingerprint

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestGuardedContextDarwinFailsClosedBeforeSourceRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "synthetic")
	content := []byte("disposable-macos-boundary")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal("synthetic source setup")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal("synthetic source stat")
	}
	file := domain.FileInstance{Path: path, Size: int64(len(content)), ModifiedAt: info.ModTime()}
	got, err := GuardedContext(context.Background(), root, file)
	if !errors.Is(err, ErrGuardedMountBoundaryUnverified) || got != "" {
		t.Fatalf("Darwin freshness read must remain unverified: hash=%q err=%v", got, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, content) {
		t.Fatal("refused verification changed synthetic source")
	}
}
