//go:build darwin || linux

package executor

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreProbeBoundaryEvidence(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	payload := []byte("private-disposable-content")
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	path := filepath.Join(root, "file")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if exists, matches, err := probeRestorePath(root, path, hash, int64(len(payload))); err != nil || !exists || !matches {
		t.Fatal("complete regular file not confirmed")
	}
	if exists, _, err := probeRestorePath(root, filepath.Join(root, "missing"), hash, int64(len(payload))); err != nil || exists {
		t.Fatal("strict absence not confirmed")
	}
	if _, _, err := probeRestorePath(root, filepath.Join(outside, "file"), hash, 1); err == nil {
		t.Fatal("outside accepted")
	}
	if err := os.Symlink(outside, filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := probeRestorePath(root, filepath.Join(root, "alias", "missing"), hash, 1); err == nil {
		t.Fatal("missing leaf under symlink treated absent")
	}
	if _, _, err := probeRestorePath(root, filepath.Join(root, "missing-parent", "missing"), hash, 1); err == nil {
		t.Fatal("unavailable parent treated absent")
	}
	// /dev is an existing independent mounted filesystem on supported hosts.
	// Only ancestor metadata is inspected, no device contents are read.
	a, _ := os.Stat("/")
	b, _ := os.Stat("/dev")
	ad, _ := deviceAndInode(a)
	bd, _ := deviceAndInode(b)
	if ad != bd {
		if _, _, err := probeRestorePath("/", "/dev/null", hash, 1); err == nil {
			t.Fatal("nested mount accepted")
		}
	} else {
		t.Log("nested mount assertion unavailable on this host")
	}
}

func TestRestoreProbeRejectsBindingReplacementDuringRead(t *testing.T) {
	for _, level := range []string{"root", "ancestor", "leaf"} {
		t.Run(level, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "root")
			parent := filepath.Join(root, "parent")
			path := filepath.Join(parent, "file")
			payload := []byte("disposable-identical-replacement")
			if err := os.MkdirAll(parent, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, payload, 0600); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(payload)
			expected := hex.EncodeToString(sum[:])
			invoked := false
			_, matches, err := probeRestorePathWithReader(root, path, expected, int64(len(payload)), func(f *os.File) (string, error) {
				invoked = true
				b, err := io.ReadAll(f)
				if err != nil {
					return "", err
				}
				target := path
				if level == "root" {
					target = root
				}
				if level == "ancestor" {
					target = parent
				}
				if err := os.Rename(target, target+"-preserved"); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(parent, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, payload, 0600); err != nil {
					t.Fatal(err)
				}
				h := sha256.Sum256(b)
				return hex.EncodeToString(h[:]), nil
			})
			if !invoked || err == nil || matches {
				t.Fatal("replaced live binding authorized by old descriptor")
			}
			if strings.Contains(err.Error(), root) {
				t.Fatal("binding refusal leaked path")
			}
		})
	}
}
