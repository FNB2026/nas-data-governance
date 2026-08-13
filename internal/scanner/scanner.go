package scanner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// Options configures a scan.
type Options struct {
	Root          string
	StorageID     string
	CrossMounts   bool
	ExcludedNames map[string]bool
	// ResumePath, if non-empty, skips files whose path sorts at or before
	// this value (path <= ResumePath). Directories are still entered so that
	// files beyond the boundary are discovered. Used by incremental scan to
	// resume from a checkpoint.
	//
	// Correctness depends on Scan visiting files in ascending global
	// dictionary order of their full paths: that way every file with
	// path <= ResumePath has already been scanned, so skipping them on
	// resume never misses a file. Scan enforces this ordering via a
	// deterministic pre-order DFS (see dirSortKey).
	ResumePath string
	// NetworkSource enables conservative classification of filesystem errors
	// that indicate a temporarily disconnected remote source.
	NetworkSource bool
	// OnDirectoryCompleted is called after a directory subtree has been fully
	// enumerated. LastScannedPath is the greatest file path visited so far and
	// FilesScanned is the session-local count. Callers may use this boundary to
	// persist a durable resume checkpoint before traversal advances.
	OnDirectoryCompleted func(DirectoryCheckpoint) error
}

// DirectoryCheckpoint describes a deterministic, completed traversal prefix.
type DirectoryCheckpoint struct {
	LastScannedPath string
	FilesScanned    int
}

// Stats holds scan statistics returned by Scan.
type Stats struct {
	FilesScanned int
	DirsVisited  int
	// Errors collects non-fatal errors (e.g., permission denied on one
	// directory). The scan continues past these. Fatal errors are
	// returned as the error result of Scan itself.
	Errors []ErrorEntry
	// SourceUnavailable is true only when a network source produced an error
	// consistent with a disconnected or stale mount. Permission errors remain
	// ordinary partial-coverage errors.
	SourceUnavailable bool
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
//   - Traversal order: deterministic pre-order depth-first. Entries within
//     each directory are sorted by dirSortKey so that files are visited in
//     ascending global dictionary order of their full paths. This ordering
//     invariant is what makes checkpoint resume (path <= ResumePath)
//     correct: BFS or any non-dictionary order would skip files that were
//     never actually scanned.
//   - Resume: when ResumePath is set, files with path <= ResumePath are
//     skipped (for checkpoint recovery).
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
		if opts.NetworkSource && (isNetworkInterruption(err) || errors.Is(err, os.ErrNotExist)) {
			return Stats{
				Errors:            []ErrorEntry{{Path: root, Error: err}},
				SourceUnavailable: true,
			}, nil
		}
		return Stats{}, err
	}
	rootDev, _ := deviceAndInode(rootInfo)
	identityReliable := physicalIdentityReliable(root)

	stats := Stats{}
	// seen guards (storage_id, path) idempotency across the whole scan.
	// Sized for typical directory volumes; grows as needed.
	seen := make(map[scanPathKey]struct{}, 4096)
	lastScannedPath := ""
	checkpointBlocked := false

	// walkDir performs a pre-order depth-first traversal of dirPath. Entries
	// are sorted with dirSortKey so that the visit order matches a global
	// ascending dictionary order of file paths. Recursing into a subdirectory
	// before its next sibling guarantees that directory's entire subtree is
	// processed first, which is exactly what keeps ResumePath (path <=
	// boundary) consistent with the actual visit order.
	var walkDir func(dirPath string, dirDev uint64) error
	walkDir = func(dirPath string, dirDev uint64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		stats.DirsVisited++

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			stats.Errors = append(stats.Errors, ErrorEntry{Path: dirPath, Error: err})
			checkpointBlocked = true
			if opts.NetworkSource && (isNetworkInterruption(err) || (dirPath == root && errors.Is(err, os.ErrNotExist))) {
				stats.SourceUnavailable = true
			}
			return nil // single dir failure does not abort the scan
		}

		// os.ReadDir already returns entries sorted by name, but plain name
		// ordering does NOT equal global path ordering when a directory name
		// is a prefix of a sibling file name (e.g. dir "a" vs file "a.txt":
		// globally "root/a.txt" < "root/a/..."). Re-sort with dirSortKey so
		// DFS visits files in true ascending dictionary order.
		sort.Slice(entries, func(i, j int) bool {
			return dirSortKey(entries[i]) < dirSortKey(entries[j])
		})

		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}

			path := filepath.Join(dirPath, entry.Name())

			// Exclusion check (root itself is never excluded).
			if path != root && opts.ExcludedNames[entry.Name()] {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				stats.Errors = append(stats.Errors, ErrorEntry{Path: path, Error: err})
				checkpointBlocked = true
				if opts.NetworkSource && isNetworkInterruption(err) {
					stats.SourceUnavailable = true
				}
				continue
			}

			// Skip symlinks (AGENTS rule 4).
			if info.Mode()&fs.ModeSymlink != 0 {
				continue
			}

			dev, ino := deviceAndInode(info)

			// Cross-mount protection.
			if !opts.CrossMounts && dev != 0 && dirDev != 0 && dev != dirDev {
				continue
			}

			if entry.IsDir() {
				// Recurse depth-first so this directory's entire subtree is
				// processed before the next sibling, preserving global
				// dictionary order.
				if err := walkDir(path, dev); err != nil {
					return err
				}
				continue
			}

			// Resume checkpoint: skip files at or before the resume path.
			// The checkpoint's last_scanned_path is the most recently
			// scanned file; resume continues strictly after it.
			if opts.ResumePath != "" && path <= opts.ResumePath {
				continue
			}

			// Idempotency: skip if this (storage_id, path) was already visited.
			// Key excludes inode so hard links at different paths remain distinct.
			if markPathSeen(seen, opts.StorageID, path) {
				continue
			}

			stats.FilesScanned++
			lastScannedPath = path
			physical := physicalIdentity(info, identityReliable)
			if err := visit(domain.FileInstance{
				StorageID: opts.StorageID, Path: path, Name: entry.Name(), Size: info.Size(),
				Mode: uint32(info.Mode()), ModifiedAt: info.ModTime(), Device: dev, Inode: ino,
				Physical:     physical,
				DiscoveredAt: time.Now().UTC(),
			}); err != nil {
				return err
			}
		}
		if opts.OnDirectoryCompleted != nil && lastScannedPath != "" && !checkpointBlocked {
			if err := opts.OnDirectoryCompleted(DirectoryCheckpoint{
				LastScannedPath: lastScannedPath,
				FilesScanned:    stats.FilesScanned,
			}); err != nil {
				return err
			}
		}
		return nil
	}

	if err := walkDir(root, rootDev); err != nil {
		return stats, err
	}
	return stats, nil
}

// IsNetworkUnavailableError reports whether an error from a known network
// filesystem is consistent with a disconnected/stale source. os.ErrNotExist
// is included because an already-enumerated mount disappears as ENOENT when
// a worker later opens the file for hashing.
func IsNetworkUnavailableError(err error) bool {
	return errors.Is(err, os.ErrNotExist) || isNetworkInterruption(err)
}

func isNetworkInterruption(err error) bool {
	return errors.Is(err, syscall.ENOTCONN) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.EHOSTDOWN) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETDOWN) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.ESTALE) ||
		errors.Is(err, syscall.EIO)
}

// dirSortKey returns a sort key for a directory entry that makes a pre-order
// DFS visit files in ascending global dictionary order of their full paths.
//
// Directory names are suffixed with "/" so that a directory sorts relative to
// its siblings exactly as its descendants (whose paths begin with "name/")
// do in global path order. For example, dir "a" (key "a/") sorts AFTER file
// "a.txt" (key "a.txt") because '.' (0x2E) < '/' (0x2F), matching the global
// order "root/a.txt" < "root/a/...". A plain name sort would place dir "a"
// before file "a.txt" and break the resume-skip invariant.
func dirSortKey(e os.DirEntry) string {
	if e.IsDir() {
		return e.Name() + "/"
	}
	return e.Name()
}

func physicalIdentity(info fs.FileInfo, filesystemReliable bool) domain.PhysicalIdentity {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return domain.PhysicalIdentity{}
	}
	device, inode := uint64(stat.Dev), uint64(stat.Ino)
	return domain.PhysicalIdentity{
		Device:    device,
		Inode:     inode,
		LinkCount: uint64(stat.Nlink),
		Reliable:  filesystemReliable && device != 0 && inode != 0,
	}
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
