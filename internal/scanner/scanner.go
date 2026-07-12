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

func DefaultExclusions() map[string]bool {
	names := []string{".data-governance-trash", "@eaDir", ".snapshot", "#recycle"}
	m := make(map[string]bool, len(names))
	for _, name := range names {
		m[strings.TrimSpace(name)] = true
	}
	return m
}

// FormatErrors returns a human-readable summary of non-fatal scan errors.
// Returns an empty string when there are no errors.
func (s Stats) FormatErrors() string {
	if len(s.Errors) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d non-fatal errors:\n", len(s.Errors))
	for i, e := range s.Errors {
		if i >= 10 {
			fmt.Fprintf(&b, "  ... and %d more\n", len(s.Errors)-10)
			break
		}
		fmt.Fprintf(&b, "  %s: %v\n", e.Path, e.Error)
	}
	return b.String()
}
