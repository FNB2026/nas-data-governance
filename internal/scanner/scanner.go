package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"nas-data-governance/internal/domain"
)

type Options struct {
	Root          string
	StorageID     string
	CrossMounts   bool
	ExcludedNames map[string]bool
}

func Scan(opts Options, visit func(domain.FileInstance) error) error {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return err
	}
	rootDev, _ := deviceAndInode(rootInfo)

	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk %q: %w", path, walkErr)
		}
		if path != root && opts.ExcludedNames[entry.Name()] {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %q: %w", path, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil
		}
		dev, ino := deviceAndInode(info)
		if !opts.CrossMounts && dev != 0 && rootDev != 0 && dev != rootDev {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		return visit(domain.FileInstance{
			StorageID: opts.StorageID, Path: path, Name: entry.Name(), Size: info.Size(),
			Mode: uint32(info.Mode()), ModifiedAt: info.ModTime(), Device: dev, Inode: ino,
			DiscoveredAt: time.Now().UTC(),
		})
	})
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
