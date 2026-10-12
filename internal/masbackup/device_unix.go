//go:build darwin || linux

package masbackup

import (
	"io/fs"
	"syscall"
)

func sameDevice(a, b fs.FileInfo) bool {
	left, leftOK := a.Sys().(*syscall.Stat_t)
	right, rightOK := b.Sys().(*syscall.Stat_t)
	return leftOK && rightOK && left.Dev == right.Dev
}
