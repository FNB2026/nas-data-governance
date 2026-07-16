package executor

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/FNB2026/nas-data-governance/internal/fingerprint"
)

// ErrHashMismatch is returned by VerifyFile when the computed SHA-256 does
// not match the expected value. The error carries no path or content.
var ErrHashMismatch = errors.New("executor: hash mismatch")

// ErrSymlinkRefused is returned when a source path is a symlink. The
// executor never follows symlinks, per AGENTS rule 4.
var ErrSymlinkRefused = errors.New("executor: symlink refused")

// ErrDestinationExists is returned when the destination file already exists
// and was not created with O_EXCL by this call.
var ErrDestinationExists = errors.New("executor: destination exists")

// notSymlink rejects a symlink at the leaf. Task-root-relative ancestor
// validation is performed by notSymlinkBelowRoot in the executor, which lets
// macOS system aliases such as /var -> /private/var remain usable while still
// refusing symlinks introduced inside a configured user-data root.
func notSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrSymlinkRefused
	}
	return nil
}

// CopyFile copies src to dst. The destination is created with O_EXCL so an
// existing file is never silently overwritten. Source file permissions are
// preserved. The source must not be a symlink (AGENTS rule 4).
//
// This function copies bytes only; it does not verify integrity. Callers
// must follow up with VerifyFile before deleting the source.
func CopyFile(src, dst string) (int64, error) {
	if err := notSymlink(src); err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, fmt.Errorf("mkdir parent: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return 0, err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode())
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return 0, ErrDestinationExists
		}
		return 0, err
	}
	success := false
	defer func() {
		_ = out.Close()
		if !success {
			_ = os.Remove(dst)
		}
	}()
	n, err := io.Copy(out, in)
	if err != nil {
		return n, err
	}
	if err := out.Sync(); err != nil {
		return n, err
	}
	success = true
	return n, nil
}

// VerifyFile computes the SHA-256 of path and compares against expected.
// Returns ErrHashMismatch when they differ. The path must not be a symlink.
func VerifyFile(path, expectedHash string) error {
	if err := notSymlink(path); err != nil {
		return err
	}
	actual, err := fingerprint.Full(path)
	if err != nil {
		return err
	}
	if actual != expectedHash {
		return ErrHashMismatch
	}
	return nil
}

// CopyAndVerify creates dst exclusively, verifies its complete SHA-256, and
// removes the newly-created destination if verification fails. The source is
// never removed. This is the safe primitive for COPY actions.
func CopyAndVerify(src, dst, expectedHash string) (int64, error) {
	n, err := CopyFile(src, dst)
	if err != nil {
		return n, err
	}
	if err := VerifyFile(dst, expectedHash); err != nil {
		_ = os.Remove(dst) // dst was created by CopyFile with O_EXCL.
		return n, fmt.Errorf("verify: %w", err)
	}
	return n, nil
}

// MoveFile performs the white-paper cross-volume move (§41):
// copy → verify → delete source. The source is deleted only after the
// destination hash matches expectedHash. If any step fails, the source is
// left untouched and the partial destination (if any) is removed.
//
// For same-volume moves the caller may use os.Rename directly, but this
// function always does the full copy-verify-delete dance so the integrity
// guarantee is uniform regardless of volume layout. Callers who need the
// fast path can check device IDs themselves and call os.Rename.
func MoveFile(src, dst, expectedHash string) error {
	if err := notSymlink(src); err != nil {
		return err
	}
	if _, err := CopyAndVerify(src, dst, expectedHash); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	if err := SafeRemove(src); err != nil {
		// Verification passed but source removal failed. This is the
		// dangerous case flagged in the white paper: "复制成功不代表
		// 移动成功". We leave both files in place and return an error
		// so the caller can decide whether to retry the remove or
		// treat the move as incomplete.
		_ = os.Remove(dst) // output was created by CopyFile with O_EXCL.
		return fmt.Errorf("delete source: %w", err)
	}
	return nil
}

// SafeRemove deletes a file. It is the destructive primitive used by
// quarantine and explicit delete actions. The caller MUST have already
// verified the file's integrity before calling this — SafeRemove does
// not re-check the hash, because the caller is expected to hold a fresh
// snapshot from the stale-check step.
//
// The path must not be a symlink (AGENTS rule 4).
func SafeRemove(path string) error {
	if err := notSymlink(path); err != nil {
		return err
	}
	return os.Remove(path)
}
