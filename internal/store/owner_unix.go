//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/FNB2026/nas-data-governance/internal/scanner"
)

var ErrWriterUnavailable = errors.New("store: project writer unavailable")

func databaseScopeBinding(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return ""
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d:%d", canonical, st.Dev, st.Ino)))
	return hex.EncodeToString(h[:])
}

// This lease covers updated product writers only; older binaries and external
// SQLite clients do not participate. Operator-owned stable local ancestry is
// required. It does not replace the independent execution/recovery lease.
func acquireOwner(path string) (func(), error) {
	profile, err := scanner.ProbeSource(context.Background(), filepath.Dir(path))
	if err != nil || profile.Network || profile.FilesystemType == "unknown" {
		return nil, ErrWriterUnavailable
	}
	if info, err := os.Lstat(path); err == nil {
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.Mode().IsRegular() || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || info.Mode().Perm() != 0600 {
			return nil, ErrWriterUnavailable
		}
	} else if !os.IsNotExist(err) {
		return nil, ErrWriterUnavailable
	}
	fd, err := unix.Open(path+".owner.lock", unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, ErrWriterUnavailable
	}
	f := os.NewFile(uintptr(fd), "project-owner")
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, ErrWriterUnavailable
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 || info.Mode().Perm() != 0600 || unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB) != nil {
		f.Close()
		return nil, ErrWriterUnavailable
	}
	var once sync.Once
	return func() { once.Do(func() { _ = unix.Flock(fd, unix.LOCK_UN); _ = f.Close() }) }, nil
}
