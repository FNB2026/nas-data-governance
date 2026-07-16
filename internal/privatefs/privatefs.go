// Package privatefs creates project-owned artifacts with owner-only access.
// These files can contain real paths, filenames, hashes, and directory
// structure, so permissive umask defaults are never relied upon.
package privatefs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DirectoryMode os.FileMode = 0o700
	FileMode      os.FileMode = 0o600
)

// EnsureDirectory creates path with owner-only access when needed and validates
// an existing directory without changing an arbitrary caller-owned parent.
// Symbolic links are refused.
func EnsureDirectory(path string) error {
	path = filepath.Clean(path)
	info, err := os.Lstat(path)
	created := false
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, DirectoryMode); err != nil {
			return fmt.Errorf("privatefs: create directory: %w", err)
		}
		created = true
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("privatefs: inspect directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("privatefs: directory must not be a symbolic link")
	}
	if !info.IsDir() {
		return fmt.Errorf("privatefs: path is not a directory")
	}
	if created {
		if err := os.Chmod(path, DirectoryMode); err != nil {
			return fmt.Errorf("privatefs: secure directory: %w", err)
		}
	}
	return nil
}

// SecureDirectory explicitly tightens a project-owned directory. Callers must
// not use this on shared locations such as /tmp or a repository root.
func SecureDirectory(path string) error {
	if err := EnsureDirectory(path); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Clean(path), DirectoryMode); err != nil {
		return fmt.Errorf("privatefs: secure directory: %w", err)
	}
	return nil
}

// Create creates or truncates a regular owner-only file. Existing symbolic
// links are refused, and existing files with broader permissions are tightened.
func Create(path string) (*os.File, error) {
	parent := filepath.Dir(path)
	var err error
	if filepath.Base(parent) == "var" {
		err = SecureDirectory(parent)
	} else {
		err = EnsureDirectory(parent)
	}
	if err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("privatefs: output must not be a symbolic link")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("privatefs: inspect output: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, FileMode)
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(FileMode); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("privatefs: secure output: %w", err)
	}
	return f, nil
}

// SecureFile tightens an existing regular file to owner-only access.
func SecureFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("privatefs: file must not be a symbolic link")
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("privatefs: path is not a regular file")
	}
	return os.Chmod(path, FileMode)
}

// SecureSQLiteFiles tightens the database and any existing WAL/SHM sidecars.
func SecureSQLiteFiles(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := SecureFile(candidate); err != nil {
			return err
		}
	}
	return nil
}
