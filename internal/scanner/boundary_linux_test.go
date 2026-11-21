//go:build linux

package scanner

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestBoundaryReadRejectsConcurrentNamespaceChange(t *testing.T) {
	for _, change := range []string{"file", "parent", "root", "mount"} {
		t.Run(change, func(t *testing.T) {
			temp := t.TempDir()
			root, outside := filepath.Join(temp, "root"), filepath.Join(temp, "outside")
			parent := filepath.Join(root, "nested")
			for _, dir := range []string{parent, outside} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			name := filepath.Join(parent, "file")
			const inside = "in-scope synthetic file"
			if err := os.WriteFile(name, []byte(inside), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(outside, "file"), []byte("outside synthetic sentinel"), 0600); err != nil {
				t.Fatal(err)
			}
			var files []domain.FileInstance
			if _, err := Scan(context.Background(), Options{Root: root}, func(file domain.FileInstance) error {
				files = append(files, file)
				return nil
			}); err != nil || len(files) != 1 || files[0].Path != name {
				t.Fatal("fixture scan failed", err)
			}
			b, err := OpenBoundary(root)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			called := false
			err = b.ReadFile(files[0], func(opened *os.File) error {
				called = true
				switch change {
				case "file":
					if err := os.Rename(name, name+".original"); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(name, []byte("replacement"), 0600); err != nil {
						t.Fatal(err)
					}
				case "parent", "root":
					replaced := parent
					if change == "root" {
						replaced = root
					}
					if err := os.Rename(replaced, replaced+".original"); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outside, replaced); err != nil {
						t.Fatal(err)
					}
				case "mount":
					if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" {
						t.Skip("mount fixture requires a disposable GitHub runner")
					}
					if out, err := exec.Command("sudo", "-n", "mount", "--bind", "--", outside, parent).CombinedOutput(); err != nil {
						t.Fatal("fixture mount failed", err, string(out))
					}
					t.Cleanup(func() {
						if out, err := exec.Command("sudo", "-n", "umount", "--", parent).CombinedOutput(); err != nil {
							t.Error("fixture unmount failed", err, string(out))
						}
					})
				}
				raw, err := io.ReadAll(opened)
				if err != nil || string(raw) != inside {
					t.Fatal("pinned descriptor returned replacement bytes", err)
				}
				return nil
			})
			if !called || err == nil {
				t.Fatal("namespace changed but read result remained eligible", err)
			}
		})
	}
}
