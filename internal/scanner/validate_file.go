package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// ValidateFile checks a recovered file against its freshly scanned metadata
// without following child symlinks or crossing a mount boundary. Underlying I/O
// errors stay private to the service so network interruption can be classified.
func ValidateFile(root string, file domain.FileInstance) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, file.Path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !filepath.IsAbs(file.Path) || filepath.Clean(file.Path) != file.Path {
		return errors.New("recovered file outside root; path omitted")
	}
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	rootDev, _ := deviceAndInode(info)
	path := root
	parts := strings.Split(rel, string(filepath.Separator))
	for i, part := range parts {
		path = filepath.Join(path, part)
		info, err = os.Lstat(path)
		if err != nil {
			return err
		}
		dev, ino := deviceAndInode(info)
		if info.Mode()&os.ModeSymlink != 0 || (dev != 0 && rootDev != 0 && dev != rootDev) {
			return errors.New("recovered file boundary changed; path omitted")
		}
		if i < len(parts)-1 {
			if !info.IsDir() {
				return errors.New("recovered file parent changed; path omitted")
			}
			continue
		}
		if !info.Mode().IsRegular() || info.Size() != file.Size || !info.ModTime().Equal(file.ModifiedAt) ||
			(file.Physical.Reliable && (dev != file.Device || ino != file.Inode)) {
			return errors.New("recovered file metadata changed; path omitted")
		}
	}
	return nil
}
