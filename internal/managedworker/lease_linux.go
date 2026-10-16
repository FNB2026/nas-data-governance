//go:build linux

package managedworker

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// LeaseName is the fixed single-instance lock file. It lives in the private
// run directory, never inside the managed root.
const LeaseName = "worker.lease"

// AcquireLease takes the exclusive non-blocking lease. A second worker is
// refused rather than queued, so two workers can never both believe they own
// the same managed root.
func AcquireLease(dir string, want Subject) (*Lease, error) {
	if err := RequireSubject(want); err != nil {
		return nil, err
	}
	if err := privateDirectory(dir, want); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, LeaseName)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, ErrIsolation
		}
	}
	fd, err := unix.Openat(unix.AT_FDCWD, path, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, ErrIsolation
	}
	f := os.NewFile(uintptr(fd), "managed worker lease")
	ok := false
	defer func() {
		if !ok {
			_ = f.Close()
		}
	}()
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil {
		return nil, ErrIsolation
	}
	if st.Uid != want.UID || st.Gid != want.GID || st.Mode&0o7777 != 0o600 {
		return nil, ErrIsolation
	}
	if unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil {
		return nil, ErrBusy
	}
	ok = true
	return &Lease{f: f, path: path}, nil
}

// Record stores the holder pid for diagnosis only. It never validates or
// transfers ownership and it is never used to reclaim a lease.
func (l *Lease) Record() error {
	if l == nil || l.f == nil {
		return ErrIsolation
	}
	raw := strconv.Itoa(os.Getpid())
	if err := l.f.Truncate(0); err != nil {
		return ErrIsolation
	}
	if _, err := l.f.Seek(0, 0); err != nil {
		return ErrIsolation
	}
	if _, err := l.f.WriteString(raw); err != nil {
		return ErrIsolation
	}
	return l.f.Sync()
}

// Release drops the lease. It does not delete the file: the fixed path must
// keep its identity and ownership across restarts.
func (l *Lease) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	_ = unix.Flock(int(l.f.Fd()), unix.LOCK_UN)
	return l.f.Close()
}

// LeaseHolder reports the recorded pid and whether it is currently alive.
// A lease whose holder is gone is still a lease: this package never reclaims
// it, because a vanished holder leaves an uncertain root that must be examined
// by an authorized recovery rather than silently reopened.
func LeaseHolder(dir string) (pid int, alive bool, err error) {
	path := filepath.Join(dir, LeaseName)
	fd, e := unix.Openat(unix.AT_FDCWD, path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return 0, false, ErrIsolation
	}
	f := os.NewFile(uintptr(fd), "managed worker lease holder")
	defer f.Close()
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG {
		return 0, false, ErrIsolation
	}
	raw, e := readAll(f)
	if e != nil {
		return 0, false, ErrIsolation
	}
	pid, e = strconv.Atoi(strings.TrimSpace(raw))
	if e != nil || pid <= 0 {
		return 0, false, ErrIsolation
	}
	err = unix.Kill(pid, 0)
	if err == nil {
		return pid, true, nil
	}
	if errors.Is(err, unix.EPERM) {
		return pid, true, nil
	}
	return pid, false, nil
}

func readAll(f *os.File) (string, error) {
	var out strings.Builder
	buffer := make([]byte, 64)
	for {
		n, err := f.Read(buffer)
		out.Write(buffer[:n])
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return "", err
		}
	}
	return out.String(), nil
}
