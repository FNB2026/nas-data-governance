//go:build linux

package managedfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func fixture(t *testing.T) (string, ObjectProof) {
	t.Helper()
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Skip("non-root private reader fixture required")
	}
	root := filepath.Join(t.TempDir(), "managed")
	if os.Mkdir(root, 0700) != nil || os.Mkdir(filepath.Join(root, "objects"), 0700) != nil {
		t.Fatal("fixture creation failed")
	}
	var st unix.Statx_t
	if unix.Statx(unix.AT_FDCWD, root, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BASIC_STATS, &st) != nil {
		t.Fatal("fixture identity unavailable")
	}
	rp := RootProof{strings.Repeat("a", 64), strings.Repeat("b", 64), unix.Mkdev(st.Dev_major, st.Dev_minor), st.Ino}
	marker, _ := json.Marshal(struct {
		Version int    `json:"version"`
		RootID  string `json:"root_id"`
		StoreID string `json:"store_id"`
	}{1, rp.RootID, rp.StoreID})
	if os.WriteFile(filepath.Join(root, "root.json"), append(marker, '\n'), 0600) != nil {
		t.Fatal("fixture marker failed")
	}
	id := strings.Repeat("c", 64)
	raw := []byte("synthetic sealed private object")
	path := filepath.Join(root, "objects", id)
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("fixture object failed")
	}
	if unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BASIC_STATS, &st) != nil {
		t.Fatal("fixture object identity failed")
	}
	hash := sha256.Sum256(raw)
	op := ObjectProof{rp, id, unix.Mkdev(st.Dev_major, st.Dev_minor), st.Ino, int64(st.Size), st.Mtime.Sec, st.Mtime.Nsec, st.Ctime.Sec, st.Ctime.Nsec, hex.EncodeToString(hash[:])}
	r, err := OpenReader(root, rp)
	if errors.Is(err, ErrUnsupported) && os.Getenv("NDG_REQUIRE_MANAGED_READER_KERNEL") != "1" {
		t.Skip("fixture filesystem/kernel unsupported")
	}
	if err != nil {
		t.Fatal("valid fixture rejected", err)
	}
	if r.Close() != nil {
		t.Fatal("fixture reader close failed")
	}
	return root, op
}

func opened(t *testing.T, root string, proof RootProof) Reader {
	t.Helper()
	r, err := OpenReader(root, proof)
	if err != nil {
		t.Fatal("reader construction failed", err)
	}
	t.Cleanup(func() {
		if r.Close() != nil {
			t.Error("reader close failed")
		}
	})
	return r
}

func TestReaderMatchesStoredProofWithoutCreatingFiles(t *testing.T) {
	root, proof := fixture(t)
	r := opened(t, root, proof.Root)
	if err := r.Inspect(context.Background(), proof); err != nil {
		t.Fatal("valid stored proof rejected", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 2 {
		t.Fatal("reader changed root namespace")
	}
	entries, err = os.ReadDir(filepath.Join(root, "objects"))
	if err != nil || len(entries) != 1 || entries[0].Name() != proof.ObjectID {
		t.Fatal("reader changed object namespace")
	}
	for _, fd := range []int{r.(*reader).root, r.(*reader).objects} {
		flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		if err != nil || flags&unix.FD_CLOEXEC == 0 {
			t.Fatal("retained descriptor is not CLOEXEC")
		}
	}
}

func TestReaderRejectsChangedObjects(t *testing.T) {
	for _, kind := range []string{"replacement", "content", "hardlink", "symlink", "fifo", "directory", "mode", "hash", "size", "root-proof"} {
		t.Run(kind, func(t *testing.T) {
			root, proof := fixture(t)
			r := opened(t, root, proof.Root)
			path := filepath.Join(root, "objects", proof.ObjectID)
			switch kind {
			case "replacement":
				raw, _ := os.ReadFile(path)
				if os.Rename(path, path+"-original") != nil || os.WriteFile(path, raw, 0600) != nil {
					t.Fatal("replacement fixture failed")
				}
			case "content":
				if os.WriteFile(path, []byte("changed synthetic private object"), 0600) != nil {
					t.Fatal("content fixture failed")
				}
			case "hardlink":
				if os.Link(path, path+"-link") != nil {
					t.Fatal("hardlink fixture failed")
				}
			case "symlink", "fifo", "directory":
				if os.Rename(path, path+"-original") != nil {
					t.Fatal("special fixture failed")
				}
				var err error
				if kind == "symlink" {
					err = os.Symlink(path+"-original", path)
				}
				if kind == "fifo" {
					err = unix.Mkfifo(path, 0600)
				}
				if kind == "directory" {
					err = os.Mkdir(path, 0700)
				}
				if err != nil {
					t.Fatal("special object creation failed")
				}
			case "mode":
				if os.Chmod(path, 0644) != nil {
					t.Fatal("mode fixture failed")
				}
			case "hash":
				proof.SHA256 = strings.Repeat("0", 64)
			case "size":
				proof.Size++
			case "root-proof":
				proof.Root.StoreID = strings.Repeat("d", 64)
			}
			if err := r.Inspect(context.Background(), proof); !errors.Is(err, ErrRejected) {
				t.Fatal("changed object not rejected", err)
			}
		})
	}
}

func TestReaderRejectsNamesAndCancellation(t *testing.T) {
	root, proof := fixture(t)
	r := opened(t, root, proof.Root)
	for _, name := range []string{"", "../outside", "/etc/passwd", "objects/child", strings.Repeat("A", 64), ".", ".."} {
		p := proof
		p.ObjectID = name
		if err := r.Inspect(context.Background(), p); err != ErrRejected {
			t.Fatal("invalid object ID not rejected")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Inspect(ctx, proof); err != ErrCancelled {
		t.Fatal("cancelled inspection continued")
	}
}

func TestReaderRejectsRootReplacement(t *testing.T) {
	root, proof := fixture(t)
	r := opened(t, root, proof.Root)
	if os.Rename(root, root+"-original") != nil || os.Mkdir(root, 0700) != nil {
		t.Fatal("root replacement fixture failed")
	}
	if err := r.Inspect(context.Background(), proof); err != ErrRejected {
		t.Fatal("replacement root accepted", err)
	}
}

type pausedContext struct {
	context.Context
	entered chan struct{}
	release chan struct{}
}

func (c pausedContext) Err() error {
	select {
	case c.entered <- struct{}{}:
	default:
	}
	<-c.release
	return nil
}

func TestReaderBusyCloseNeverReleasesOrReusesDescriptors(t *testing.T) {
	root, proof := fixture(t)
	r := opened(t, root, proof.Root)
	c := pausedContext{context.Background(), make(chan struct{}, 1), make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- r.Inspect(c, proof) }()
	<-c.entered
	copyOfInterface := r
	if copyOfInterface.Close() != ErrBusy || copyOfInterface.Inspect(context.Background(), proof) != ErrBusy {
		t.Fatal("in-flight session lifetime was not protected")
	}
	for i := 0; i < 32; i++ {
		fd, err := unix.Open("/dev/null", unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal("FD reuse fixture failed")
		}
		if fd == r.(*reader).root || fd == r.(*reader).objects {
			t.Fatal("in-flight descriptor was reused")
		}
		_ = unix.Close(fd)
	}
	close(c.release)
	if err := <-done; err != nil {
		t.Fatal("protected inspection failed", err)
	}
	if r.Close() != nil || copyOfInterface.Close() != nil {
		t.Fatal("close was not exactly-once/idempotent")
	}
	if r.Inspect(context.Background(), proof) != ErrClosed {
		t.Fatal("closed session accepted another operation")
	}
}
