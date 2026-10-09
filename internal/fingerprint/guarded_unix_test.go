//go:build darwin || linux

package fingerprint

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
)

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
