//go:build darwin || linux

package fingerprint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
)

type cancelAfterRead struct {
	cancel context.CancelFunc
	reads  int
}

func (r *cancelAfterRead) Read(p []byte) (int, error) {
	r.reads++
	if r.reads == 1 {
		copy(p, "x")
		r.cancel()
		return 1, nil
	}
	return 0, errors.New("underlying reader called after cancellation")
}

func TestContextReaderChecksCancellationBetweenReads(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	underlying := &cancelAfterRead{cancel: cancel}
	reader := contextReader{ctx: ctx, reader: underlying}
	buf := make([]byte, 1)
	if n, err := reader.Read(buf); n != 1 || !errors.Is(err, context.Canceled) || string(buf) != "x" {
		t.Fatal("cancellation during controlled read", n, err)
	}
	if n, err := reader.Read(buf); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled reader did not stop between chunks", n, err)
	}
	if underlying.reads != 1 {
		t.Fatal("underlying source was read again after cancellation")
	}
}

var _ io.Reader = (*cancelAfterRead)(nil)

func TestGuardedContextRejectsPreCanceledSourceRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "synthetic-cancel-file")
	data := []byte("synthetic-cancel-content")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal("synthetic file setup")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal("synthetic file stat")
	}
	h := sha256.Sum256(data)
	file := domain.FileInstance{Path: path, Size: int64(len(data)), ModifiedAt: info.ModTime()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GuardedContext(ctx, root, file); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled guarded read should fail closed: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) || sha256.Sum256(got) != h {
		t.Fatal("canceled verification changed synthetic source")
	}
}

func TestGuardedMatchesHashAndRejectsFileOrAncestorReplacement(t *testing.T) {
	for _, kind := range []string{"unchanged", "file-symlink", "parent-symlink", "changed", "outside"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			parent := filepath.Join(root, "inside")
			os.Mkdir(parent, 0700)
			path := filepath.Join(parent, "file")
			os.WriteFile(path, []byte("test"), 0600)
			var file domain.FileInstance
			if _, err := scanner.Scan(context.Background(), scanner.Options{Root: root, StorageID: "guarded"}, func(f domain.FileInstance) error { file = f; return nil }); err != nil {
				t.Fatal(err)
			}
			want, _ := Full(path)
			outside := t.TempDir()
			other := filepath.Join(outside, "file")
			os.WriteFile(other, []byte("secret outside contents"), 0600)
			switch kind {
			case "file-symlink":
				os.Remove(path)
				os.Symlink(other, path)
			case "parent-symlink":
				os.Rename(parent, parent+"-old")
				os.Symlink(outside, parent)
			case "changed":
				os.WriteFile(path, []byte("different size"), 0600)
			case "outside":
				file.Path = other
			}
			for _, full := range []bool{false, true} {
				h, err := Guarded(root, file, full)
				if kind == "unchanged" {
					if err != nil || h != want {
						t.Fatalf("guarded hash mismatch: %v", err)
					}
				} else if err == nil || h != "" {
					t.Fatal("read replaced/outside source")
				}
			}
		})
	}
}

func TestGuardedRejectsReplacementDuringProductionRead(t *testing.T) {
	for _, ancestor := range []bool{false, true} {
		t.Run(fmt.Sprint(ancestor), func(t *testing.T) {
			root := t.TempDir()
			parent := filepath.Join(root, "inside")
			if err := os.Mkdir(parent, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(parent, "file")
			content := bytes.Repeat([]byte("test"), sampleSize)
			if err := os.WriteFile(path, content, 0600); err != nil {
				t.Fatal(err)
			}
			var file domain.FileInstance
			if _, err := scanner.Scan(context.Background(), scanner.Options{Root: root, StorageID: "guarded"}, func(f domain.FileInstance) error { file = f; return nil }); err != nil {
				t.Fatal(err)
			}
			for _, full := range []bool{false, true} {
				got, err := Guarded(root, file, full)
				if err != nil {
					t.Fatal(err)
				}
				var want string
				if full {
					want, err = Full(path)
				} else {
					want, err = Quick(path, file.Size)
				}
				if err != nil || got != want {
					t.Fatal("large guarded fingerprint changed algorithm")
				}
			}
			h, err := guardedRead(root, file, func(f *os.File) (string, error) {
				if ancestor {
					if err := os.Rename(parent, parent+"-old"); err != nil {
						return "", err
					}
					if err := os.Mkdir(parent, 0700); err != nil {
						return "", err
					}
				} else {
					if err := os.Rename(path, path+"-old"); err != nil {
						return "", err
					}
				}
				if err := os.WriteFile(path, content, 0600); err != nil {
					return "", err
				}
				if err := os.Chtimes(path, file.ModifiedAt, file.ModifiedAt); err != nil {
					return "", err
				}
				return fullReader(f)
			})
			if err == nil || h != "" {
				t.Fatal("published old descriptor hash for replaced path")
			}
		})
	}
}
