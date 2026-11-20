//go:build linux

package managedfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

const maxObjectBytes int64 = 1 << 40

type directoryIdentity struct {
	device, inode, mount uint64
}

type reader struct {
	gate      sync.Mutex
	closed    bool
	path      string
	proof     RootProof
	root      int
	objects   int
	rootID    directoryIdentity
	objectsID directoryIdentity
}

func opaque(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'f' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func aclAbsent(fd int) bool {
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		if _, err := unix.Fgetxattr(fd, name, nil); !errors.Is(err, unix.ENODATA) {
			return false
		}
	}
	return true
}

func snapshot(fd int) (unix.Statx_t, error) {
	var st unix.Statx_t
	mask := unix.STATX_BASIC_STATS | unix.STATX_MNT_ID
	if unix.Statx(fd, "", unix.AT_EMPTY_PATH|unix.AT_STATX_SYNC_AS_STAT, mask, &st) != nil || st.Mask&uint32(mask) != uint32(mask) {
		return st, ErrUnsupported
	}
	return st, nil
}

func directory(fd int, private bool) (directoryIdentity, error) {
	st, err := snapshot(fd)
	if err != nil {
		return directoryIdentity{}, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR || !aclAbsent(fd) {
		return directoryIdentity{}, ErrRejected
	}
	uid, gid := uint32(os.Geteuid()), uint32(os.Getegid())
	if private {
		if st.Uid != uid || st.Gid != gid || st.Mode&07777 != 0700 {
			return directoryIdentity{}, ErrRejected
		}
	} else if st.Uid != 0 && st.Uid != uid || st.Mode&0022 != 0 && !(st.Uid == 0 && st.Mode&07777 == 01777) {
		// A system-owned sticky temporary parent cannot let another UID rename
		// the immediately following private, caller-owned component.
		return directoryIdentity{}, ErrRejected
	}
	return directoryIdentity{unix.Mkdev(st.Dev_major, st.Dev_minor), st.Ino, st.Mnt_id}, nil
}

// Explicit configured root mounts are allowed. Descendants always use NO_XDEV;
// no device-number comparison replaces the kernel lookup restriction.
func absolute(path string) (int, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return -1, ErrRejected
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, ErrRejected
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if _, err = directory(fd, false); err != nil {
			_ = unix.Close(fd)
			return -1, err
		}
		next, openErr := unix.Openat2(fd, part, &unix.OpenHow{
			Flags:   unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC,
			Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
		})
		_ = unix.Close(fd)
		if openErr != nil {
			return -1, ErrRejected
		}
		fd = next
	}
	return fd, nil
}

func rooted(fd int, name string, flags int) (int, error) {
	next, err := unix.Openat2(fd, name, &unix.OpenHow{
		Flags:   uint64(flags | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
	if err != nil {
		return -1, ErrRejected
	}
	return next, nil
}

func regular(fd int) (unix.Statx_t, error) {
	st, err := snapshot(fd)
	if err != nil {
		return st, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Mode&07777 != 0600 || st.Uid != uint32(os.Geteuid()) || st.Gid != uint32(os.Getegid()) || st.Nlink != 1 || !aclAbsent(fd) {
		return st, ErrRejected
	}
	return st, nil
}

func marker(fd int, proof RootProof) error {
	n, err := rooted(fd, "root.json", unix.O_RDONLY)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(n), "managed root identity")
	defer file.Close()
	st, err := regular(n)
	if err != nil || st.Size > 512 {
		return ErrRejected
	}
	expected, err := json.Marshal(struct {
		Version int    `json:"version"`
		RootID  string `json:"root_id"`
		StoreID string `json:"store_id"`
	}{1, proof.RootID, proof.StoreID})
	if err != nil {
		return ErrRejected
	}
	expected = append(expected, '\n')
	raw, err := io.ReadAll(io.LimitReader(file, 513))
	if err != nil || !bytes.Equal(raw, expected) {
		return ErrRejected
	}
	return nil
}

func OpenReader(path string, proof RootProof) (Reader, error) {
	if os.Geteuid() == 0 || os.Getegid() == 0 || !opaque(proof.RootID) || !opaque(proof.StoreID) || proof.Inode == 0 {
		return nil, ErrRejected
	}
	fd, err := absolute(path)
	if err != nil {
		return nil, err
	}
	r := &reader{path: path, proof: proof, root: fd, objects: -1}
	keep := false
	defer func() {
		if !keep {
			_ = r.Close()
		}
	}()
	r.rootID, err = directory(fd, true)
	if err != nil || r.rootID.device != proof.Device || r.rootID.inode != proof.Inode {
		return nil, ErrRejected
	}
	var fs unix.Statfs_t
	if unix.Fstatfs(fd, &fs) != nil {
		return nil, ErrUnsupported
	}
	switch uint64(fs.Type) {
	case 0xef53, 0x9123683e, 0x58465342: // ext4, Btrfs, XFS: still need per-FS execution proof.
	default:
		return nil, ErrUnsupported
	}
	if marker(fd, proof) != nil {
		return nil, ErrRejected
	}
	// A root still carrying the bootstrap partial marker is incomplete by
	// construction. The canonical marker alone cannot prove completion.
	partial, err := rooted(fd, bootstrapPartialName, unix.O_RDONLY)
	if err == nil {
		_ = unix.Close(partial)
		return nil, ErrRecoveryRequired
	}
	r.objects, err = rooted(fd, "objects", unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		return nil, err
	}
	r.objectsID, err = directory(r.objects, true)
	if err != nil {
		return nil, err
	}
	keep = true
	return r, nil
}

func (r *reader) bound() error {
	current, err := absolute(r.path)
	if err != nil {
		return err
	}
	defer unix.Close(current)
	identity, err := directory(current, true)
	if err != nil || identity != r.rootID || marker(current, r.proof) != nil {
		return ErrRejected
	}
	fresh, err := rooted(current, "objects", unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		return err
	}
	defer unix.Close(fresh)
	identity, err = directory(fresh, true)
	if err != nil || identity != r.objectsID {
		return ErrRejected
	}
	return nil
}

func matches(st unix.Statx_t, proof ObjectProof) bool {
	return unix.Mkdev(st.Dev_major, st.Dev_minor) == proof.Device && st.Ino == proof.Inode && st.Size == uint64(proof.Size) &&
		st.Mtime.Sec == proof.MTimeSec && st.Mtime.Nsec == proof.MTimeNsec && st.Ctime.Sec == proof.CTimeSec && st.Ctime.Nsec == proof.CTimeNsec
}

func (r *reader) Inspect(ctx context.Context, proof ObjectProof) error {
	if !r.gate.TryLock() {
		return ErrBusy
	}
	defer r.gate.Unlock()
	if r.closed {
		return ErrClosed
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	if proof.Root != r.proof || !opaque(proof.ObjectID) || !opaque(proof.SHA256) || proof.Size < 0 || proof.Size > maxObjectBytes || proof.Inode == 0 {
		return ErrRejected
	}
	if err := r.bound(); err != nil {
		return err
	}
	if err := inspectFile(ctx, r.objects, proof.ObjectID, proof); err != nil {
		return err
	}
	return r.bound()
}

func inspectFile(ctx context.Context, parent int, name string, proof ObjectProof) error {
	fd, err := rooted(parent, name, unix.O_RDONLY)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), "managed sealed object")
	defer file.Close()
	st, err := regular(fd)
	if err != nil || !matches(st, proof) {
		return ErrRejected
	}
	h := sha256.New()
	buffer := make([]byte, 64*1024)
	var total int64
	for {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		n, readErr := file.Read(buffer)
		total += int64(n)
		if total > proof.Size {
			return ErrRejected
		}
		_, _ = h.Write(buffer[:n])
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return ErrRejected
		}
	}
	st, err = regular(fd)
	if err != nil || !matches(st, proof) || total != proof.Size || hex.EncodeToString(h.Sum(nil)) != proof.SHA256 {
		return ErrRejected
	}
	// A pinned content FD is not a fresh pathname binding. Reopen the name
	// under the same kernel restrictions before reporting an observed match.
	fresh, err := rooted(parent, name, unix.O_RDONLY)
	if err != nil {
		return err
	}
	defer unix.Close(fresh)
	st, err = regular(fresh)
	if err != nil || !matches(st, proof) {
		return ErrRejected
	}
	return nil
}

func (r *reader) Close() error {
	if !r.gate.TryLock() {
		return ErrBusy
	}
	defer r.gate.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	result := error(nil)
	for _, fd := range []int{r.objects, r.root} {
		if fd >= 0 && unix.Close(fd) != nil {
			result = ErrRejected
		}
	}
	r.root, r.objects = -1, -1
	return result
}
