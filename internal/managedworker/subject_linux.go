//go:build linux

package managedworker

import (
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// RequireSubject refuses to continue unless this process already is the
// dedicated unprivileged identity with no effective capability and no way to
// regain one. The worker never attempts to drop its own privileges: dropping
// privileges from root leaves the same address space in charge of the secrets.
func RequireSubject(want Subject) error {
	if want.UID == 0 || want.GID == 0 {
		return ErrIsolation
	}
	if os.Geteuid() != int(want.UID) || os.Getegid() != int(want.GID) {
		return ErrIsolation
	}
	status, err := kernelStatus()
	if err != nil {
		return err
	}
	caps, err := parseHexField(status, "CapEff:")
	if err != nil {
		return err
	}
	if caps != 0 {
		return ErrIsolation
	}
	return nil
}

// RequireNoNewPrivs is checked separately because it is a property of how the
// worker was launched, not something an ordinary test process can be expected
// to already have. The launcher must set it; the worker refuses to run without
// it so that no setuid helper can widen a worker that only appeared unprivileged.
func RequireNoNewPrivs() error {
	status, err := kernelStatus()
	if err != nil {
		return err
	}
	nnp, err := parseHexField(status, "NoNewPrivs:")
	if err != nil {
		return err
	}
	if nnp != 1 {
		return ErrIsolation
	}
	return nil
}

// SetNoNewPrivs is used by the worker's own startup path, before any private
// root is opened. It is irreversible for this process.
func SetNoNewPrivs() error {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return ErrUnsupported
	}
	return nil
}

// kernelStatus reads the process status only from a verified procfs regular
// file. A replaced /proc or a symlinked status file is refused.
func kernelStatus() ([]byte, error) {
	fd, err := unix.Open("/proc/self/status", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnsupported
	}
	f := os.NewFile(uintptr(fd), "kernel process status")
	defer f.Close()
	var fs unix.Statfs_t
	var st unix.Stat_t
	if unix.Fstatfs(fd, &fs) != nil || uint64(fs.Type) != unix.PROC_SUPER_MAGIC ||
		unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, ErrUnsupported
	}
	return io.ReadAll(io.LimitReader(f, 1<<20))
}

func parseHexField(status []byte, key string) (uint64, error) {
	for _, line := range strings.Split(string(status), "\n") {
		if !strings.HasPrefix(line, key) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, key))
		parsed, err := strconv.ParseUint(value, 16, 64)
		if err != nil {
			return 0, ErrUnsupported
		}
		return parsed, nil
	}
	return 0, ErrUnsupported
}

// privateDirectory requires an existing directory that is a regular directory,
// not a symbolic link, owned by want, mode 0700 and free of POSIX ACLs.
func privateDirectory(path string, want Subject) error {
	var st unix.Stat_t
	if unix.Lstat(path, &st) != nil {
		return ErrIsolation
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return ErrIsolation
	}
	if st.Uid != want.UID || st.Gid != want.GID {
		return ErrIsolation
	}
	if st.Mode&0o7777 != 0o700 {
		return ErrIsolation
	}
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default"} {
		_, err := unix.Getxattr(path, name, nil)
		if !isNoData(err) {
			return ErrIsolation
		}
	}
	return nil
}

func isNoData(err error) bool {
	return err == unix.ENODATA || err == unix.ENOTSUP || err == unix.EOPNOTSUPP
}

// RequireLayout refuses a deployment whose private directories are missing,
// shared, relocated or owned by another account. The socket is deliberately
// kept outside the managed root so that a stray socket cannot become an
// unknown entry inside it.
func RequireLayout(layout Layout, want Subject) error {
	if layout.Run == "" || layout.State == "" || layout.Run == layout.State {
		return ErrIsolation
	}
	if privateDirectory(layout.Run, want) != nil || privateDirectory(layout.State, want) != nil {
		return ErrIsolation
	}
	return nil
}
