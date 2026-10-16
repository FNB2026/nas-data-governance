//go:build linux

package managedfs

import (
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const maxMountInfoBytes = 4 << 20

func decimal(value string, bits int) (uint64, bool) {
	n, err := strconv.ParseUint(value, 10, bits)
	return n, err == nil && strconv.FormatUint(n, 10) == value
}

// mountProfile identifies a current filesystem type; it does not certify
// durability or replace kernel NO_XDEV lookup. STATX_MNT_ID (not UNIQUE) is
// session metadata and is never persisted as cross-restart authority.
func mountProfile(raw []byte, mount uint64, major, minor uint32, magic uint64) (string, error) {
	if len(raw) == 0 || len(raw) > maxMountInfoBytes || raw[len(raw)-1] != '\n' || mount == 0 {
		return "", ErrUnsupported
	}
	seen := map[uint64]bool{}
	found := ""
	for _, line := range strings.Split(string(raw[:len(raw)-1]), "\n") {
		fields := strings.Fields(line)
		separator := -1
		for index, value := range fields {
			if value == "-" {
				if separator != -1 {
					return "", ErrUnsupported
				}
				separator = index
			}
		}
		if separator < 6 || len(fields)-separator != 4 {
			return "", ErrUnsupported
		}
		id, ok := decimal(fields[0], 64)
		if !ok || id == 0 || seen[id] {
			return "", ErrUnsupported
		}
		seen[id] = true
		if _, ok := decimal(fields[1], 64); !ok {
			return "", ErrUnsupported
		}
		device := strings.Split(fields[2], ":")
		if len(device) != 2 || !strings.HasPrefix(fields[3], "/") || !strings.HasPrefix(fields[4], "/") {
			return "", ErrUnsupported
		}
		deviceMajor, majorOK := decimal(device[0], 32)
		deviceMinor, minorOK := decimal(device[1], 32)
		if !majorOK || !minorOK {
			return "", ErrUnsupported
		}
		if id != mount {
			continue
		}
		if deviceMajor != uint64(major) || deviceMinor != uint64(minor) {
			return "", ErrUnsupported
		}
		kind := fields[separator+1]
		if !(kind == "ext4" && magic == 0xef53 || kind == "btrfs" && magic == 0x9123683e || kind == "xfs" && magic == 0x58465342) {
			return "", ErrUnsupported
		}
		found = kind
	}
	if found == "" {
		return "", ErrUnsupported
	}
	return found, nil
}

func writerFilesystem(root int) (string, error) {
	st, err := snapshot(root)
	if err != nil {
		return "", ErrUnsupported
	}
	var filesystem unix.Statfs_t
	if unix.Fstatfs(root, &filesystem) != nil {
		return "", ErrUnsupported
	}
	// This fixed kernel metadata API intentionally uses proc's self link. No
	// caller-supplied source path is followed. A fake ordinary mountinfo file
	// fails procfs admission; hostile privileged proc remounts are excluded.
	fd, err := unix.Open("/proc/self/mountinfo", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", ErrUnsupported
	}
	f := os.NewFile(uintptr(fd), "kernel mount metadata")
	defer f.Close()
	var proc unix.Statfs_t
	var info unix.Stat_t
	if unix.Fstatfs(fd, &proc) != nil || uint64(proc.Type) != unix.PROC_SUPER_MAGIC || unix.Fstat(fd, &info) != nil || info.Mode&unix.S_IFMT != unix.S_IFREG {
		return "", ErrUnsupported
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxMountInfoBytes+1))
	if err != nil {
		return "", ErrUnsupported
	}
	return mountProfile(raw, st.Mnt_id, st.Dev_major, st.Dev_minor, uint64(filesystem.Type))
}
