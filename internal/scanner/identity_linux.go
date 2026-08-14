//go:build linux

package scanner

import "syscall"

// Linux statfs magic values used by filesystems NDG explicitly understands.
// Reliability is allowlisted: an unknown filesystem remains conservative.
const (
	linuxExtMagic     = 0xef53
	linuxXFSMagic     = 0x58465342
	linuxBtrfsMagic   = 0x9123683e
	linuxTmpfsMagic   = 0x01021994
	linuxOverlayMagic = 0x794c7630
	linuxZFSMagic     = 0x2fc12fc1
	linuxNFSMagic     = 0x6969
	linuxFuseMagic    = 0x65735546
	linuxCIFSMagic    = 0xff534d42
	linuxSMB2Magic    = 0xfe534d42
	linux9PMagic      = 0x01021997
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
	return profileForLinuxMagic(uint64(stat.Type))
}

func profileForLinuxMagic(magic uint64) (string, bool, bool) {
	switch magic {
	case linuxExtMagic:
		return "ext", false, true
	case linuxXFSMagic:
		return "xfs", false, true
	case linuxBtrfsMagic:
		return "btrfs", false, true
	case linuxTmpfsMagic:
		return "tmpfs", false, true
	case linuxOverlayMagic:
		return "overlay", false, true
	case linuxZFSMagic:
		return "zfs", false, true
	case linuxNFSMagic:
		return "nfs", true, false
	case linuxFuseMagic:
		return "fuse", true, false
	case linuxCIFSMagic, linuxSMB2Magic:
		return "smbfs", true, false
	case linux9PMagic:
		return "9p", true, false
	default:
		return "unknown", false, false
	}
}
