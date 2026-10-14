//go:build darwin || linux

package fingerprint

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

var ErrGuardedSourceChanged = errors.New("guarded source changed; path omitted")
var ErrGuardedMountBoundaryUnverified = errors.New("guarded source mount boundary unverified; path omitted")

// Guarded hashes a recovered candidate through directory descriptors anchored
// at the task root. Every child open rejects symlinks and different devices;
// renaming a parent cannot redirect the read to another tree. The opened file's
// metadata must match the fresh scan both before and after hashing.
func Guarded(root string, file domain.FileInstance, full bool) (string, error) {
	return guardedReadWithOpener(root, file, func(f *os.File) (string, error) {
		if full {
			return fullReader(f)
		}
		return quickReader(f, file.Size)
	}, openRecovered)
}

// GuardedContext is Guarded with cancellation checks between file reads. A
// filesystem read already blocked in the kernel cannot be interrupted by Go;
// callers must retain the source lease until it returns.
func GuardedContext(ctx context.Context, root string, file domain.FileInstance) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !guardedContextMountProofAvailable() {
		return "", ErrGuardedMountBoundaryUnverified
	}
	if file.Size < 0 || file.Size == int64(^uint64(0)>>1) {
		return "", errors.New("recovered fingerprint size invalid; path omitted")
	}
	return guardedReadWithOpener(root, file, func(f *os.File) (string, error) {
		// Read at most expected size plus one byte. This bounds a file that
		// grows after its initial descriptor stat; guardedRead checks size again
		// after hashing and rejects the changed binding.
		return fullReader(io.LimitReader(contextReader{ctx: ctx, reader: f}, file.Size+1))
	}, openRecoveredNoFollowRoot)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	if ctxErr := r.ctx.Err(); ctxErr != nil {
		return n, ctxErr
	}
	return n, err
}

func guardedRead(root string, file domain.FileInstance, read func(*os.File) (string, error)) (string, error) {
	return guardedReadWithOpener(root, file, read, openRecovered)
}

type recoveredOpener func(root, rel string) (*os.File, unix.Stat_t, error)

func guardedReadWithOpener(root string, file domain.FileInstance, read func(*os.File) (string, error), open recoveredOpener) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, file.Path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || !filepath.IsAbs(file.Path) || filepath.Clean(file.Path) != file.Path {
		return "", errors.New("recovered fingerprint outside root; path omitted")
	}
	f, rootStat, err := open(root, rel)
	if err != nil {
		return "", err
	}
	defer f.Close()
	validate := func() error {
		info, err := f.Stat()
		if err != nil {
			return err
		}
		sys, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.Mode().IsRegular() || uint64(sys.Dev) != uint64(rootStat.Dev) || info.Size() != file.Size || !info.ModTime().Equal(file.ModifiedAt) || (file.Physical.Reliable && (uint64(sys.Dev) != file.Device || uint64(sys.Ino) != file.Inode)) {
			return ErrGuardedSourceChanged
		}
		return nil
	}
	if err := validate(); err != nil {
		return "", err
	}
	hash, err := read(f)
	if err != nil {
		return "", err
	}
	if err := validate(); err != nil {
		return "", err
	}
	// Reopen the complete current binding, including ancestors and root. The
	// original descriptor alone cannot detect rename/replacement of its path.
	current, currentRoot, err := open(root, rel)
	if err != nil {
		return "", err
	}
	defer current.Close()
	originalInfo, err := f.Stat()
	if err != nil {
		return "", err
	}
	currentInfo, err := current.Stat()
	if err != nil {
		return "", err
	}
	if rootStat.Dev != currentRoot.Dev || rootStat.Ino != currentRoot.Ino || !os.SameFile(originalInfo, currentInfo) || currentInfo.Size() != file.Size || !currentInfo.ModTime().Equal(file.ModifiedAt) {
		return "", ErrGuardedSourceChanged
	}
	return hash, nil
}

func openRecovered(root, rel string) (*os.File, unix.Stat_t, error) {
	dir, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, unix.Stat_t{}, err
	}
	return openRecoveredAtRoot(dir, rel)
}

// openRootNoFollow walks the absolute registered root from / using directory
// descriptors. Unlike O_NOFOLLOW on root alone, this rejects a symlink in any
// ancestor above the root without a check-then-open race. Mounts before the
// registered root are allowed; platform-specific member openers enforce the
// task-root boundary.
func openRootNoFollow(root string) (int, error) {
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		return -1, errors.New("recovered fingerprint root is not absolute; path omitted")
	}
	dir, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	if root != string(filepath.Separator) {
		for _, part := range strings.Split(strings.TrimPrefix(root, string(filepath.Separator)), string(filepath.Separator)) {
			if part == "" || part == "." || part == ".." {
				_ = unix.Close(dir)
				return -1, errors.New("recovered fingerprint root is invalid; path omitted")
			}
			next, openErr := unix.Openat(dir, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if openErr != nil {
				_ = unix.Close(dir)
				return -1, openErr
			}
			_ = unix.Close(dir)
			dir = next
		}
	}
	return dir, nil
}

func openRecoveredAtRoot(dir int, rel string) (*os.File, unix.Stat_t, error) {
	defer func() { _ = unix.Close(dir) }()
	var rootStat unix.Stat_t
	if err := unix.Fstat(dir, &rootStat); err != nil {
		return nil, unix.Stat_t{}, err
	}
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		child, err := unix.Openat(dir, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, unix.Stat_t{}, err
		}
		var info unix.Stat_t
		if err := unix.Fstat(child, &info); err != nil {
			_ = unix.Close(child)
			return nil, unix.Stat_t{}, err
		}
		if info.Dev != rootStat.Dev {
			_ = unix.Close(child)
			return nil, unix.Stat_t{}, errors.New("recovered fingerprint crossed mount; path omitted")
		}
		_ = unix.Close(dir)
		dir = child
	}
	fd, err := unix.Openat(dir, parts[len(parts)-1], unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, unix.Stat_t{}, err
	}
	f := os.NewFile(uintptr(fd), "recovered-candidate")
	return f, rootStat, nil
}
