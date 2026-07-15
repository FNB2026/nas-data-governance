package scanner

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"nas-data-governance/internal/domain"
)

// Options configures a scan.
type Options struct {
	Root          string
	StorageID     string
	CrossMounts   bool
	ExcludedNames map[string]bool
	// ResumePath, if non-empty, skips files whose path sorts strictly
	// before this value. Directories are still entered so that files
	// at the same prefix level are discovered. Used by incremental scan
	// to resume from a checkpoint.
	ResumePath string
}

// Stats holds scan statistics returned by Scan.
type Stats struct {
	FilesScanned int
	DirsVisited  int
	// Errors collects non-fatal errors (e.g., permission denied on one
	// directory). The scan continues past these. Fatal errors are
	// returned as the error result of Scan itself.
	Errors []ErrorEntry
}

// ErrorEntry records a non-fatal error encountered during scanning.
type ErrorEntry struct {
	Path  string
	Error error
}

type scanPathKey struct {
	storageID string
	path      string
}

// Scan walks the filesystem tree starting at opts.Root and calls visit
// for each regular file. It accepts a context for cancellation.
//
// Key behaviors:
//   - Context cancellation: returns ctx.Err() immediately.
//   - Single directory failure: records the error in Stats.Errors and
//     continues scanning other directories. Does NOT abort the whole scan.
//   - Symlinks: skipped (AGENTS rule 4).
//   - Cross-mount: when CrossMounts is false, subdirectories on a
//     different device are skipped (AGENTS rule 4).
//   - Resume: when ResumePath is set, files whose path sorts before
//     ResumePath are skipped (for checkpoint recovery).
//   - Path idempotency: a (storage_id, path) pair is visited at most once.
//     This defensively contains duplicate enumeration without assigning the
//     cause to a filesystem or protocol. Hard links — same inode at a different
//     path — are distinct instances and are NOT deduplicated, because the key
//     excludes inode by design.
func Scan(ctx context.Context, opts Options, visit func(domain.FileInstance) error) (Stats, error) {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return Stats{}, err
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return Stats{}, err
	}
	rootDev, _ := deviceAndInode(rootInfo)

	stats := Stats{}
	// seen guards (storage_id, path) idempotency across the whole scan.
	// Sized for typical directory volumes; grows as needed.
	seen := make(map[scanPathKey]struct{}, 4096)
	type dirEntry struct {
		path string
		dev  uint64
	}
	queue := []dirEntry{{path: root, dev: rootDev}}

	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return stats, err
		}

		dir := queue[0]
		queue = queue[1:]
		stats.DirsVisited++

		entries, err := os.ReadDir(dir.path)
		if err != nil {
			stats.Errors = append(stats.Errors, ErrorEntry{Path: dir.path, Error: err})
			continue // single dir failure does not abort the scan
		}

		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return stats, err
			}

			path := filepath.Join(dir.path, entry.Name())

			// Exclusion check (root itself is never excluded).
			if path != root && opts.ExcludedNames[entry.Name()] {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				stats.Errors = append(stats.Errors, ErrorEntry{Path: path, Error: err})
				continue
			}

			// Skip symlinks (AGENTS rule 4).
			if info.Mode()&fs.ModeSymlink != 0 {
				continue
			}

			dev, ino := deviceAndInode(info)

			// Cross-mount protection.
			if !opts.CrossMounts && dev != 0 && dir.dev != 0 && dev != dir.dev {
				continue
			}

			if entry.IsDir() {
				queue = append(queue, dirEntry{path: path, dev: dev})
				continue
			}

			// Resume checkpoint: skip files before the resume path.
			if opts.ResumePath != "" && path < opts.ResumePath {
				continue
			}

			// Idempotency: skip if this (storage_id, path) was already visited.
			// Key excludes inode so hard links at different paths remain distinct.
			if markPathSeen(seen, opts.StorageID, path) {
				continue
			}

			stats.FilesScanned++
			if err := visit(domain.FileInstance{
				StorageID: opts.StorageID, Path: path, Name: entry.Name(), Size: info.Size(),
				Mode: uint32(info.Mode()), ModifiedAt: info.ModTime(), Device: dev, Inode: ino,
				DiscoveredAt: time.Now().UTC(),
			}); err != nil {
				return stats, err
			}
		}
	}

	return stats, nil
}

func deviceAndInode(info fs.FileInfo) (uint64, uint64) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0
	}
	return uint64(stat.Dev), uint64(stat.Ino)
}

// markPathSeen records a (storage_id, path) pair and reports whether it was
// already present. Inode is intentionally excluded: hard links (same inode at
// a different path) are distinct file instances and must NOT be merged.
func markPathSeen(seen map[scanPathKey]struct{}, storageID, path string) bool {
	key := scanPathKey{storageID: storageID, path: path}
	if _, ok := seen[key]; ok {
		return true
	}
	seen[key] = struct{}{}
	return false
}

func DefaultExclusions() map[string]bool {
	names := []string{".data-governance-trash", "@eaDir", ".snapshot", "#recycle"}
	m := make(map[string]bool, len(names))
	for _, name := range names {
		m[strings.TrimSpace(name)] = true
	}
	return m
}

// FormatErrors returns a path-free summary suitable for ordinary logs.
// Detailed paths remain available only in Stats.Errors for an explicitly
// private diagnostic layer. Returns an empty string when there are no errors.
func (s Stats) FormatErrors() string {
	if len(s.Errors) == 0 {
		return ""
	}
	return fmt.Sprintf("%d non-fatal errors; paths omitted", len(s.Errors))
}
