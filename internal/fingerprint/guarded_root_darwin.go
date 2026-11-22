//go:build darwin

package fingerprint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Darwin lacks Linux openat2 RESOLVE_NO_XDEV, but a descriptor-based proof is
// available: walk every component from the registered root with O_NOFOLLOW and
// require the opened directory/file to stay on the same device and the same
// statfs mount identity as the root. This rejects both ordinary mount crossings
// and same-device bind/nullfs-style mounts for the descriptor actually read.
func guardedContextMountProofAvailable() bool { return true }

func openRecoveredNoFollowRoot(root, rel string) (*os.File, unix.Stat_t, error) {
	dir, err := openRootNoFollow(root)
	if err != nil {
		// A sandboxed App may hold a security-scoped grant for the selected root
		// while being denied access to its ancestors. Only an explicit sandbox
		// denial may fall back to opening the already-canonical root directly
		// with O_NOFOLLOW; symlink or path-shape failures still fail closed.
		if !errors.Is(err, unix.EACCES) && !errors.Is(err, unix.EPERM) {
			return nil, unix.Stat_t{}, err
		}
		dir, err = unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, unix.Stat_t{}, err
		}
	}
	defer func() { _ = unix.Close(dir) }()

	var rootStat unix.Stat_t
	if err := unix.Fstat(dir, &rootStat); err != nil {
		return nil, unix.Stat_t{}, err
	}
	// Fstatfs of descriptor 0 is not meaningful after closing dir; capture the
	// mount identity while the root descriptor is still open.
	var rootFS unix.Statfs_t
	if err := unix.Fstatfs(dir, &rootFS); err != nil {
		return nil, unix.Stat_t{}, err
	}
	sameMount := func(fd int) error {
		var fs unix.Statfs_t
		if err := unix.Fstatfs(fd, &fs); err != nil {
			return err
		}
		if fs.Fsid != rootFS.Fsid {
			return ErrGuardedMountBoundaryUnverified
		}
		return nil
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
			return nil, unix.Stat_t{}, ErrGuardedMountBoundaryUnverified
		}
		if err := sameMount(child); err != nil {
			_ = unix.Close(child)
			return nil, unix.Stat_t{}, err
		}
		_ = unix.Close(dir)
		dir = child
	}

	fd, err := unix.Openat(dir, parts[len(parts)-1], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, unix.Stat_t{}, err
	}
	if err := sameMount(fd); err != nil {
		_ = unix.Close(fd)
		return nil, unix.Stat_t{}, err
	}
	return os.NewFile(uintptr(fd), "recovered-candidate"), rootStat, nil
}
