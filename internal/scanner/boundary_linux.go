//go:build linux

package scanner

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// Boundary holds the Linux task root for traversal and subsequent file reads.
type Boundary struct {
	root string
	fd   int
	id   unix.Statx_t
}

var ErrMountBoundary = errors.New("scan mount boundary refused; path omitted")

func OpenBoundary(root string) (*Boundary, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, root, &unix.OpenHow{
		Flags: unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_NO_SYMLINKS,
	})
	if err != nil {
		return nil, errors.Join(ErrMountBoundary, err)
	}
	id, err := boundaryStat(fd)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return &Boundary{root: root, fd: fd, id: id}, nil
}

func boundaryStat(fd int) (unix.Statx_t, error) {
	var st unix.Statx_t
	const requested = unix.STATX_BASIC_STATS | unix.STATX_MNT_ID_UNIQUE
	if unix.Statx(fd, "", unix.AT_EMPTY_PATH, requested, &st) != nil || st.Mask&requested != requested {
		return st, ErrMountBoundary
	}
	return st, nil
}

func (b *Boundary) Close() error { return unix.Close(b.fd) }

func (b *Boundary) VerifyRoot() error {
	current, err := OpenBoundary(b.root)
	if err != nil {
		return err
	}
	defer current.Close()
	if !sameBoundaryIdentity(current.id, b.id) {
		return ErrMountBoundary
	}
	return nil
}

func (b *Boundary) open(path string, flags uint64) (*os.File, error) {
	rel, err := filepath.Rel(b.root, path)
	if err != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path ||
		rel == ".." || strings.HasPrefix(rel, "../") {
		return nil, ErrMountBoundary
	}
	fd, err := unix.Openat2(b.fd, rel, &unix.OpenHow{
		Flags:   flags | unix.O_CLOEXEC,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
	if errors.Is(err, unix.EXDEV) || errors.Is(err, unix.ELOOP) {
		return nil, ErrMountBoundary
	}
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), filepath.Base(path)), nil
}

// ReadFile only reads an already confined, identity-checked regular object.
func (b *Boundary) ReadFile(file domain.FileInstance, read func(*os.File) error) error {
	if err := b.VerifyRoot(); err != nil {
		return err
	}
	parentPath := filepath.Dir(file.Path)
	parent, err := b.open(parentPath, unix.O_PATH|unix.O_DIRECTORY)
	if err != nil {
		return err
	}
	defer parent.Close()
	parentID, err := boundaryStat(int(parent.Fd()))
	if err != nil {
		return err
	}

	meta, err := b.open(file.Path, unix.O_PATH)
	if err != nil {
		return err
	}
	defer meta.Close()
	before, err := boundaryStat(int(meta.Fd()))
	if err != nil || before.Mode&unix.S_IFMT != unix.S_IFREG ||
		before.Size != uint64(file.Size) || before.Mtime.Sec != file.ModifiedAt.Unix() ||
		before.Mtime.Nsec != uint32(file.ModifiedAt.Nanosecond()) ||
		(file.Device != 0 && unix.Mkdev(before.Dev_major, before.Dev_minor) != file.Device) ||
		(file.Inode != 0 && before.Ino != file.Inode) {
		return ErrMountBoundary
	}
	if err := b.verifyFilePath(file.Path, before, parentID); err != nil {
		return err
	}
	f, err := os.Open(fmt.Sprintf("/proc/self/fd/%d", meta.Fd()))
	if err != nil {
		return err
	}
	defer f.Close()
	opened, err := boundaryStat(int(f.Fd()))
	if err != nil || !sameBoundaryFile(before, opened) {
		return ErrMountBoundary
	}
	if err := read(f); err != nil {
		return err
	}
	return b.verifyFilePath(file.Path, before, parentID)
}

func (b *Boundary) verifyFilePath(path string, before, parentID unix.Statx_t) error {
	currentParent, err := b.open(filepath.Dir(path), unix.O_PATH|unix.O_DIRECTORY)
	if err != nil {
		return err
	}
	defer currentParent.Close()
	currentID, err := boundaryStat(int(currentParent.Fd()))
	if err != nil || !sameBoundaryIdentity(parentID, currentID) {
		return ErrMountBoundary
	}
	current, err := b.open(path, unix.O_PATH)
	if err != nil {
		return err
	}
	defer current.Close()
	after, err := boundaryStat(int(current.Fd()))
	if err != nil || !sameBoundaryFile(before, after) {
		return ErrMountBoundary
	}
	return b.VerifyRoot()
}

func sameBoundaryIdentity(a, b unix.Statx_t) bool {
	return a.Mnt_id == b.Mnt_id && a.Dev_major == b.Dev_major && a.Dev_minor == b.Dev_minor && a.Ino == b.Ino
}

func sameBoundaryFile(a, b unix.Statx_t) bool {
	return sameBoundaryIdentity(a, b) && a.Size == b.Size && a.Mode == b.Mode && a.Mtime == b.Mtime && a.Ctime == b.Ctime
}

func (b *Boundary) ReadDirectory(path string) ([]os.DirEntry, error) {
	f, err := b.open(path, unix.O_RDONLY|unix.O_DIRECTORY)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	for i, entry := range entries {
		entries[i] = boundaryEntry{DirEntry: entry, boundary: b, path: filepath.Join(path, entry.Name())}
	}
	return entries, nil
}

type boundaryEntry struct {
	os.DirEntry
	boundary *Boundary
	path     string
}

func (e boundaryEntry) Info() (fs.FileInfo, error) {
	f, err := e.boundary.open(e.path, unix.O_PATH)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}
