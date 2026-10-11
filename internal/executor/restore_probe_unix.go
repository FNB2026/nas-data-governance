//go:build darwin || linux

package executor

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// A restore probe separates confirmed absence from unreadable or changed data.
// It is read-only and descriptor-anchored; symlinks and different-device
// descendants are rejected. Dev alone cannot identify same-device bind mounts.
func probeRestorePath(root, path, hash string, size int64) (exists, matches bool, err error) {
	return probeRestorePathWithReader(root, path, hash, size, func(f *os.File) (string, error) {
		h := sha256.New()
		_, err := io.Copy(h, f)
		return hex.EncodeToString(h.Sum(nil)), err
	})
}

// The reader parameter permits deterministic identity-race tests without globals.
func probeRestorePathWithReader(root, path, hash string, size int64, read func(*os.File) (string, error)) (exists, matches bool, err error) {
	fail := errors.New("restore executor: path evidence unavailable; path omitted")
	root = filepath.Clean(root)
	rel, e := filepath.Rel(root, path)
	if e != nil || !filepath.IsAbs(root) || !filepath.IsAbs(path) || filepath.Clean(path) != path || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, false, fail
	}
	openParent := func() (int, unix.Stat_t, error) {
		fd, e := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e != nil {
			return -1, unix.Stat_t{}, fail
		}
		var rs unix.Stat_t
		if e = unix.Fstat(fd, &rs); e != nil {
			_ = unix.Close(fd)
			return -1, rs, fail
		}
		parts := strings.Split(rel, string(filepath.Separator))
		for _, part := range parts[:len(parts)-1] {
			next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if e != nil {
				_ = unix.Close(fd)
				return -1, rs, fail
			}
			var ns unix.Stat_t
			e = unix.Fstat(next, &ns)
			_ = unix.Close(fd)
			fd = next
			if e != nil || ns.Dev != rs.Dev {
				_ = unix.Close(fd)
				return -1, rs, fail
			}
		}
		return fd, rs, nil
	}
	parent, rs, e := openParent()
	if e != nil {
		return false, false, fail
	}
	defer unix.Close(parent)
	leaf := filepath.Base(rel)
	var before unix.Stat_t
	e = unix.Fstatat(parent, leaf, &before, unix.AT_SYMLINK_NOFOLLOW)
	absent := errors.Is(e, unix.ENOENT)
	if e != nil && !absent {
		return false, false, fail
	}
	if !absent && (before.Mode&unix.S_IFMT != unix.S_IFREG || before.Dev != rs.Dev) {
		return true, false, fail
	}
	var observed os.FileInfo
	if !absent {
		fd, e := unix.Openat(parent, leaf, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if e != nil {
			return true, false, fail
		}
		f := os.NewFile(uintptr(fd), "restore-evidence")
		defer f.Close()
		info, e := f.Stat()
		if e != nil {
			return true, false, fail
		}
		dev, ino := deviceAndInode(info)
		if !info.Mode().IsRegular() || dev != uint64(before.Dev) || ino != uint64(before.Ino) {
			return true, false, fail
		}
		observedHash, e := read(f)
		if e != nil {
			return true, false, fail
		}
		after, e := f.Stat()
		if e != nil || !os.SameFile(info, after) || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
			return true, false, fail
		}
		observed = after
		matches = after.Size() == size && observedHash == hash
	}
	// Reopen the entire live root/ancestor binding. A renamed parent or root
	// cannot make an old descriptor's evidence authorize a different path.
	current, cr, e := openParent()
	if e != nil {
		return !absent, false, fail
	}
	defer unix.Close(current)
	if cr.Dev != rs.Dev || cr.Ino != rs.Ino {
		return !absent, false, fail
	}
	var originalParent, currentParent unix.Stat_t
	if unix.Fstat(parent, &originalParent) != nil || unix.Fstat(current, &currentParent) != nil || originalParent.Dev != currentParent.Dev || originalParent.Ino != currentParent.Ino {
		return !absent, false, fail
	}
	var now unix.Stat_t
	e = unix.Fstatat(current, leaf, &now, unix.AT_SYMLINK_NOFOLLOW)
	if absent {
		if !errors.Is(e, unix.ENOENT) {
			return true, false, fail
		}
		return false, false, nil
	}
	if e != nil || now.Mode&unix.S_IFMT != unix.S_IFREG || now.Dev != before.Dev || now.Ino != before.Ino || now.Size != before.Size {
		return true, false, fail
	}
	fd, e := unix.Openat(current, leaf, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return true, false, fail
	}
	live := os.NewFile(uintptr(fd), "restore-evidence")
	defer live.Close()
	liveInfo, e := live.Stat()
	if e != nil || !os.SameFile(observed, liveInfo) || observed.Size() != liveInfo.Size() || !observed.ModTime().Equal(liveInfo.ModTime()) {
		return true, false, fail
	}
	return true, matches, nil
}
