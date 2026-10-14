//go:build linux

package fingerprint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
)

func TestGuardedContextLinuxOpenat2RequiresNoXdevNoSymlinksAndBeneath(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("canonical synthetic root")
	}
	data := []byte("synthetic-openat2-boundary")
	path := filepath.Join(root, "candidate")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal("synthetic file setup")
	}
	var file domain.FileInstance
	if _, err := scanner.Scan(context.Background(), scanner.Options{Root: root, StorageID: "synthetic-openat2"}, func(item domain.FileInstance) error {
		file = item
		return nil
	}); err != nil {
		t.Fatal("synthetic indexing")
	}
	want := sha256.Sum256(data)
	got, err := GuardedContext(context.Background(), root, file)
	if err != nil || got != fmt.Sprintf("%x", want) {
		t.Fatalf("kernel openat2 read failed: %v", err)
	}

	for _, injectedErr := range []error{unix.ENOSYS, unix.EXDEV} {
		t.Run(injectedErr.Error(), func(t *testing.T) {
			called := false
			_, _, err := openRecoveredNoFollowRootWith(root, "candidate", func(_ int, gotPath string, how *unix.OpenHow) (int, error) {
				called = true
				wantResolve := uint64(unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV)
				if gotPath != "candidate" || how.Resolve != wantResolve || how.Flags&uint64(unix.O_NOFOLLOW) == 0 {
					t.Fatalf("unsafe openat2 request: path=%q flags=%#x resolve=%#x", gotPath, how.Flags, how.Resolve)
				}
				return -1, injectedErr
			})
			if !called || !errors.Is(err, injectedErr) {
				t.Fatalf("must fail closed on openat2 %v, got %v", injectedErr, err)
			}
			unchanged, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(unchanged, data) {
				t.Fatal("rejected rooted open modified synthetic source")
			}
		})
	}
}

func TestGuardedContextLinuxRejectsInnerSymlink(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("canonical synthetic root")
	}
	inside := filepath.Join(root, "inside")
	if err := os.Mkdir(inside, 0700); err != nil {
		t.Fatal("synthetic directory setup")
	}
	path := filepath.Join(inside, "candidate")
	data := []byte("before-index")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal("synthetic file setup")
	}
	var file domain.FileInstance
	if _, err := scanner.Scan(context.Background(), scanner.Options{Root: root, StorageID: "synthetic-openat2"}, func(item domain.FileInstance) error {
		file = item
		return nil
	}); err != nil {
		t.Fatal("synthetic indexing")
	}
	if err := os.Rename(inside, inside+"-moved"); err != nil {
		t.Fatal("move synthetic directory")
	}
	if err := os.Symlink(inside+"-moved", inside); err != nil {
		t.Fatal("replace synthetic directory with symlink")
	}
	if _, err := GuardedContext(context.Background(), root, file); err == nil {
		t.Fatal("openat2 followed a symlink under the registered root")
	}
}

func TestGuardedContextLinuxOpenat2FailureDoesNotFallback(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal("canonical synthetic root")
	}
	data := []byte("synthetic-openat2-fail-closed")
	path := filepath.Join(root, "candidate")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal("synthetic file setup")
	}
	for _, injectedErr := range []error{unix.ENOSYS, unix.EXDEV} {
		t.Run(injectedErr.Error(), func(t *testing.T) {
			called := 0
			_, _, openErr := openRecoveredNoFollowRootWith(root, "candidate", func(_ int, _ string, _ *unix.OpenHow) (int, error) {
				called++
				return -1, injectedErr
			})
			if called != 1 || !errors.Is(openErr, injectedErr) {
				t.Fatalf("no-xdev/no-symlink open must return kernel rejection without fallback: %v", openErr)
			}
			unchanged, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(unchanged, data) {
				t.Fatal("rejected open modified synthetic source")
			}
		})
	}
}
