//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package store

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// AcquireExecutionLock excludes recovery while an execution owns this DB,
// across store handles and processes. Never wait behind an active writer.
func (s *SQLiteStore) AcquireExecutionLock() (func(), error) {
	fd, e := unix.Open(s.path+".execution.lock", unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if e != nil {
		return nil, errors.New("store: cannot acquire execution lock")
	}
	f := os.NewFile(uintptr(fd), "execution lock")
	info, statErr := f.Stat()
	if statErr != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, errors.New("store: invalid execution lock")
	}
	if e = f.Chmod(0600); e != nil {
		_ = f.Close()
		return nil, errors.New("store: cannot secure execution lock")
	}
	if e = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); e != nil {
		_ = f.Close()
		return nil, errors.New("store: execution or recovery already active")
	}
	return func() { _ = unix.Flock(fd, unix.LOCK_UN); _ = f.Close() }, nil
}
