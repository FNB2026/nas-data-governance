//go:build darwin

package scanner

import (
	"strings"
	"syscall"
)

func physicalIdentityReliable(root string) bool {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(root, &stat); err != nil {
		return false
	}
	buf := make([]byte, 0, len(stat.Fstypename))
	for _, c := range stat.Fstypename {
		if c == 0 {
			break
		}
		buf = append(buf, byte(c))
	}
	fsType := strings.ToLower(string(buf))
	switch fsType {
	case "smbfs", "nfs", "afpfs", "webdav", "fusefs", "osxfuse", "macfuse":
		return false
	default:
		return fsType != ""
	}
}
