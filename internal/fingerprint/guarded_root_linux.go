//go:build linux

package fingerprint

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

type rootedOpenat2 func(dirfd int, path string, how *unix.OpenHow) (int, error)

func guardedContextMountProofAvailable() bool { return true }

func openRecoveredNoFollowRoot(root, rel string) (*os.File, unix.Stat_t, error) {
	return openRecoveredNoFollowRootWith(root, rel, unix.Openat2)
}

func openRecoveredNoFollowRootWith(root, rel string, openat2 rootedOpenat2) (*os.File, unix.Stat_t, error) {
	dir, err := openRootNoFollow(root)
	if err != nil {
		return nil, unix.Stat_t{}, err
	}
	defer func() { _ = unix.Close(dir) }()
	var rootStat unix.Stat_t
	if err := unix.Fstat(dir, &rootStat); err != nil {
		return nil, unix.Stat_t{}, err
	}
	how := &unix.OpenHow{
		Flags:   uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK),
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	}
	fd, err := openat2(dir, rel, how)
	if err != nil {
		// Do not fall back to path opens or st_dev-only checks. A kernel without
		// openat2 or a mount/symlink rejection is an unverified source.
		return nil, unix.Stat_t{}, err
	}
	if fd < 0 {
		return nil, unix.Stat_t{}, errors.New("rooted source open returned an invalid descriptor")
	}
	return os.NewFile(uintptr(fd), "governance-freshness-source"), rootStat, nil
}
