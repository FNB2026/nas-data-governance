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

// ExecutionLockHeld probes whether a writer currently holds the
// execution/recovery lock, without acquiring it. It never blocks and never
// persists a lock. Three outcomes are distinguished and callers MUST treat
// them differently:
//
//	(false, nil)  — confirmed free: the lock file does not exist (acquisition
//	                always creates it) or the probe took and released the lock;
//	(true, nil)   — confirmed held: the probe was refused with EWOULDBLOCK/EAGAIN;
//	(false, err)  — unknown: the lock file exists but is not accessible (wrong
//	                type, permissions, symlink, unexpected flock error). The
//	                caller must refuse, never proceed as if the lock were free.
func (s *SQLiteStore) ExecutionLockHeld() (bool, error) {
	fd, e := unix.Open(s.path+".execution.lock", unix.O_RDWR|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		if errors.Is(e, unix.ENOENT) {
			// No lock file: acquisition creates one before taking the flock,
			// so no writer can hold the lock.
			return false, nil
		}
		return false, errors.New("store: execution lock state unknown")
	}
	f := os.NewFile(uintptr(fd), "execution lock probe")
	defer f.Close()
	if e := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); e != nil {
		if errors.Is(e, unix.EWOULDBLOCK) || errors.Is(e, unix.EAGAIN) {
			return true, nil // a writer holds it
		}
		return false, errors.New("store: execution lock state unknown")
	}
	_ = unix.Flock(fd, unix.LOCK_UN)
	return false, nil
}
