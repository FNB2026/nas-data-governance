//go:build darwin || linux

package fingerprint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// Guarded hashes a recovered candidate through directory descriptors anchored
// at the task root. Every child open rejects symlinks and different devices;
// renaming a parent cannot redirect the read to another tree. The opened file's
// metadata must match the fresh scan both before and after hashing.
func Guarded(root string, file domain.FileInstance, full bool) (string, error) {
	return guardedRead(root, file, func(f *os.File) (string, error) {
		if full {
			return fullReader(f)
		}
		return quickReader(f, file.Size)
	})
}

func guardedRead(root string, file domain.FileInstance, read func(*os.File) (string, error)) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, file.Path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !filepath.IsAbs(file.Path) || filepath.Clean(file.Path) != file.Path {
		return "", errors.New("recovered fingerprint outside root; path omitted")
	}
	f, rootStat, err := openRecovered(root, rel)
	if err != nil {
		return "", err
	}
	defer f.Close()
	validate := func() error {
		info, err := f.Stat()
		if err != nil {
			return err
		}
		sys, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.Mode().IsRegular() || uint64(sys.Dev) != uint64(rootStat.Dev) || info.Size() != file.Size || !info.ModTime().Equal(file.ModifiedAt) || (file.Physical.Reliable && (uint64(sys.Dev) != file.Device || uint64(sys.Ino) != file.Inode)) {
			return errors.New("recovered fingerprint metadata changed; path omitted")
		}
		return nil
	}
	if err := validate(); err != nil {
		return "", err
	}
	hash, err := read(f)
	if err != nil {
		return "", err
	}
	if err := validate(); err != nil {
		return "", err
	}
	// Reopen the complete current binding, including ancestors and root. The
	// original descriptor alone cannot detect rename/replacement of its path.
	current, currentRoot, err := openRecovered(root, rel)
	if err != nil {
		return "", err
	}
	defer current.Close()
	originalInfo, err := f.Stat()
	if err != nil {
		return "", err
	}
	currentInfo, err := current.Stat()
	if err != nil {
		return "", err
	}
	if rootStat.Dev != currentRoot.Dev || rootStat.Ino != currentRoot.Ino || !os.SameFile(originalInfo, currentInfo) || currentInfo.Size() != file.Size || !currentInfo.ModTime().Equal(file.ModifiedAt) {
		return "", errors.New("recovered fingerprint path binding changed; path omitted")
	}
	return hash, nil
}

func openRecovered(root, rel string) (*os.File, unix.Stat_t, error) {
	dir, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, unix.Stat_t{}, err
	}
	defer func() { _ = unix.Close(dir) }()
	var rootStat unix.Stat_t
	if err := unix.Fstat(dir, &rootStat); err != nil {
		return nil, unix.Stat_t{}, err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		child, err := unix.Openat(dir, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, unix.Stat_t{}, err
		}
		var info unix.Stat_t
		if err := unix.Fstat(child, &info); err != nil {
			_ = unix.Close(child)
			return nil, unix.Stat_t{}, err
		}
		if info.Dev != rootStat.Dev {
			_ = unix.Close(child)
			return nil, unix.Stat_t{}, errors.New("recovered fingerprint crossed mount; path omitted")
		}
		_ = unix.Close(dir)
		dir = child
	}
	fd, err := unix.Openat(dir, parts[len(parts)-1], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, unix.Stat_t{}, err
	}
	f := os.NewFile(uintptr(fd), "recovered-candidate")
	return f, rootStat, nil
}
