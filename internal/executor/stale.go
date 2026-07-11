// Package executor implements the M3 safe-operation pipeline. It is the
// only package in this codebase allowed to write to the user's file
// system, and only through the quarantine/copy/verify/delete sub-steps
// defined here. Per AGENTS rule 1, every other package stays read-only.
//
// The pipeline is deliberately split into sub-steps (Snapshot → Check →
// Execute → Verify → Audit) so that no step can be silently merged with
// another, per AGENTS rule 2 and white paper §36-41.
package executor

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
	"time"

	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/fingerprint"
)

// FileSnapshot captures the current filesystem state of one file for stale
// comparison. It is the minimal attribute set required by the white paper
// for pre-execution review: size, mtime, inode, device, and content hash.
type FileSnapshot struct {
	Size       int64
	ModifiedAt time.Time
	Device     uint64
	Inode      uint64
	Hash       string
}

// ErrFileMissing is returned by Snapshot when the path does not exist.
// Callers should treat this as StaleMissing rather than a hard error.
var ErrFileMissing = errors.New("executor: file missing")

// Snapshot reads the current filesystem state of path. When verifyHash is
// true the SHA-256 is recomputed; otherwise Hash is left empty and the
// stale check skips hash comparison. Use verifyHash=false for cheap
// metadata-only freshness checks, and true for the final pre-execution
// review where content integrity must be confirmed.
func Snapshot(path string, verifyHash bool) (FileSnapshot, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return FileSnapshot{}, ErrFileMissing
		}
		return FileSnapshot{}, err
	}
	dev, ino := deviceAndInode(info)
	snap := FileSnapshot{
		Size:       info.Size(),
		ModifiedAt: info.ModTime(),
		Device:     dev,
		Inode:      ino,
	}
	if verifyHash {
		h, err := fingerprint.Full(path)
		if err != nil {
			return snap, err
		}
		snap.Hash = h
	}
	return snap, nil
}

// StaleReason describes why a file no longer matches its plan-time state.
// Values are stable identifiers used in audit logs; they never include the
// file path or content (AGENTS rule 6).
type StaleReason string

const (
	StaleNone    StaleReason = ""
	StaleMissing StaleReason = "missing"
	StaleSize    StaleReason = "size_changed"
	StaleMtime   StaleReason = "mtime_changed"
	StaleInode   StaleReason = "inode_changed"
	StaleDevice  StaleReason = "device_changed"
	StaleHash    StaleReason = "hash_changed"
)

// Check compares a plan's expected file state against the current snapshot.
// Returns StaleNone when the file is fresh. The comparison order is
// deliberate: cheap metadata checks run before the expensive hash check,
// so we avoid hashing a file that is already obviously stale.
//
// Hash comparison runs only when both sides have a non-empty hash. This
// lets callers opt out of hash verification by passing verifyHash=false
// to Snapshot.
func Check(expected domain.FileInstance, current FileSnapshot) StaleReason {
	if current.Size != expected.Size {
		return StaleSize
	}
	if !current.ModifiedAt.Equal(expected.ModifiedAt) {
		return StaleMtime
	}
	if current.Inode != expected.Inode {
		return StaleInode
	}
	if current.Device != expected.Device {
		return StaleDevice
	}
	if current.Hash != "" && expected.ContentSHA256 != "" && current.Hash != expected.ContentSHA256 {
		return StaleHash
	}
	return StaleNone
}

// CheckStale is the composed form: it snapshots path, then compares against
// the expected state. Returns StaleMissing when the path does not exist.
// verifyHash controls whether SHA-256 is recomputed.
//
// This helper exists for convenience; the underlying Snapshot and Check
// are exported so callers can split the steps when needed (for example,
// to snapshot many files in one pass and compare them later).
func CheckStale(expected domain.FileInstance, verifyHash bool) (StaleReason, error) {
	snap, err := Snapshot(expected.Path, verifyHash)
	if errors.Is(err, ErrFileMissing) {
		return StaleMissing, nil
	}
	if err != nil {
		return StaleNone, err
	}
	return Check(expected, snap), nil
}

// deviceAndInode mirrors scanner.deviceAndInode but lives here so the
// executor has no dependency on the scanner package (which is read-only
// walk-only). Cross-package coupling would make it harder to reason about
// which packages touch the filesystem.
func deviceAndInode(info fs.FileInfo) (uint64, uint64) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0
	}
	return uint64(stat.Dev), uint64(stat.Ino)
}
