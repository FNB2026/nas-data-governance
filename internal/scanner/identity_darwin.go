//go:build darwin

package scanner

import (
	"strings"
	"syscall"
)

func physicalIdentityReliable(root string) bool {
	_, _, reliable := filesystemProfile(root)
	return reliable
}

func filesystemProfile(root string) (string, bool, bool) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(root, &stat); err != nil {
		return "unknown", false, false
	}
	buf := make([]byte, 0, len(stat.Fstypename))
	for _, c := range stat.Fstypename {
		if c == 0 {
			break
		}
		buf = append(buf, byte(c))
	}
	return profileForFilesystem(strings.ToLower(string(buf)))
}
