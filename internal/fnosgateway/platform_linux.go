//go:build linux

package fnosgateway

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// LoadConfig reads only an absolute, root-owned path with no writable or
// symlink ancestors. An ACL write grant would also set the POSIX group mask.
func LoadConfig(path string) (Config, error) {
	var c Config
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return c, ErrRejected
	}
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return c, ErrRejected
	}
	defer func() { _ = unix.Close(fd) }()
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, p := range parts {
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || st.Uid != 0 || st.Mode&0022 != 0 {
			return c, ErrRejected
		}
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, err := unix.Openat2(fd, p, &unix.OpenHow{Flags: uint64(flags), Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
		if err != nil {
			return c, ErrRejected
		}
		_ = unix.Close(fd)
		fd = next
	}
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || st.Uid != 0 || st.Mode&0022 != 0 || st.Mode&unix.S_IFMT != unix.S_IFREG || st.Size > 4096 {
		return c, ErrRejected
	}
	f := os.NewFile(uintptr(fd), "gateway-config")
	fd = -1
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 4097))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF {
		return c, ErrRejected
	}
	return c, nil
}

// AllThreadsSyscall covers Go's existing OS threads under a stop-the-world
// barrier. It returns ENOTSUP with cgo: this executable MUST use CGO_ENABLED=0.
// All future threads inherit the now irreversible flag from their parents.
func LockPrivileges() error {
	_, _, e := syscall.AllThreadsSyscall6(unix.SYS_PRCTL, unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0, 0)
	if e != 0 {
		return ErrRejected
	}
	return nil
}
func CheckIdentity(c Config) error {
	u, e := user.Lookup(c.PackageUser)
	if e != nil {
		return ErrRejected
	}
	g, e := user.LookupGroup(c.PackageGroup)
	if e != nil {
		return ErrRejected
	}
	uid, e := strconv.Atoi(u.Uid)
	if e != nil || uid <= 0 {
		return ErrRejected
	}
	gid, e := strconv.Atoi(g.Gid)
	if e != nil || gid <= 0 {
		return ErrRejected
	}
	tasks, e := os.ReadDir("/proc/self/task")
	if e != nil || len(tasks) == 0 {
		return ErrRejected
	}
	for _, t := range tasks {
		if _, e := strconv.Atoi(t.Name()); e != nil {
			return ErrRejected
		}
		b, e := os.ReadFile("/proc/self/task/" + t.Name() + "/status")
		if e != nil {
			return ErrRejected
		}
		if !validStatus(b, uid, gid) {
			return ErrRejected
		}
	}
	return nil
}
func validStatus(b []byte, uid, gid int) bool {
	fields := map[string][]string{}
	for _, line := range bytes.Split(b, []byte{'\n'}) {
		k, v, ok := bytes.Cut(line, []byte{':'})
		if !ok {
			continue
		}
		fields[string(k)] = strings.Fields(string(v))
	}
	for k, n := range map[string]int{"Uid": uid, "Gid": gid} {
		if len(fields[k]) != 4 {
			return false
		}
		for _, s := range fields[k] {
			v, e := strconv.Atoi(s)
			if e != nil || v != n {
				return false
			}
		}
	}
	for _, s := range fields["Groups"] {
		v, e := strconv.Atoi(s)
		if e != nil || v != gid {
			return false
		}
	}
	if _, ok := fields["Groups"]; !ok {
		return false
	}
	for _, k := range []string{"CapInh", "CapPrm", "CapEff", "CapAmb"} {
		if len(fields[k]) != 1 {
			return false
		}
		v, e := strconv.ParseUint(fields[k][0], 16, 64)
		if e != nil || v != 0 {
			return false
		}
	}
	return len(fields["NoNewPrivs"]) == 1 && fields["NoNewPrivs"][0] == "1"
}
