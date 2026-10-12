//go:build !darwin && !linux

package masbackup

import "io/fs"

// Backup preflight is a macOS-focused prototype. Platforms where device
// identity is not implemented fail closed rather than skip the mount check.
func sameDevice(fs.FileInfo, fs.FileInfo) bool { return false }
