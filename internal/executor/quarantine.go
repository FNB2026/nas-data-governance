package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// QuarantineStructure controls how files are laid out inside the quarantine
// root. The white paper requires that isolated files remain traceable to
// their original location, so every destination also carries the source
// path in audit logs (not in the filename itself).
type QuarantineStructure string

const (
	// QuarantineFlat places all files directly under Root. Use when the
	// quarantine root is per-task or per-run.
	QuarantineFlat QuarantineStructure = "flat"
	// QuarantineDated groups files by YYYY-MM subdirectory under Root.
	// Use for long-lived quarantine roots that accumulate across runs.
	QuarantineDated QuarantineStructure = "dated"
)

// QuarantineConfig describes where and how isolated files are stored.
// Root must be an absolute path on the same volume as the source files
// whenever possible; cross-volume quarantine forces a copy instead of a
// rename and requires the full copy-verify-delete pipeline (§41).
type QuarantineConfig struct {
	Root      string
	Structure QuarantineStructure
}

// PathFor computes the nominal quarantine destination for a source file.
// It never touches the filesystem; the caller is expected to review and
// persist this path before any write. When Structure is "dated", the path
// includes a YYYY-MM subdirectory derived from `now`.
//
// The returned path preserves the source filename so humans can recognize
// it during review. Collision resolution (when two same-named files land in
// the same directory) is handled by ResolveCollision, which is a separate
// step so the path can be audited before the file is created.
func (c QuarantineConfig) PathFor(sourcePath string, now time.Time) string {
	base := filepath.Base(sourcePath)
	switch c.Structure {
	case QuarantineDated:
		return filepath.Join(c.Root, now.Format("2006-01"), base)
	default:
		return filepath.Join(c.Root, base)
	}
}

// ResolveCollision returns a non-existing destination path derived from
// the nominal path. When the nominal path already exists, a numeric suffix
// is inserted before the extension (e.g., "report.pdf" → "report.1.pdf").
// The function only stats; it never creates or writes.
//
// Note: this is a best-effort check. The caller must still use O_EXCL when
// creating the file to close the TOCTOU window between this check and the
// actual write.
func ResolveCollision(nominal string) (string, error) {
	if _, err := os.Stat(nominal); err != nil {
		if !os.IsExist(err) {
			return nominal, nil
		}
	}
	dir, base := filepath.Split(nominal)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 1; i < 10000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s.%d%s", stem, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("executor: could not resolve collision for %q after 10000 attempts", base)
}

// Validate checks that the quarantine root is configured and absolute.
// It does not verify the directory exists on disk — that is the caller's
// responsibility at execution time, not at plan-review time.
func (c QuarantineConfig) Validate() error {
	if c.Root == "" {
		return fmt.Errorf("executor: quarantine root is empty")
	}
	if !filepath.IsAbs(c.Root) {
		return fmt.Errorf("executor: quarantine root must be absolute")
	}
	switch c.Structure {
	case QuarantineFlat, QuarantineDated:
		return nil
	default:
		return fmt.Errorf("executor: unknown quarantine structure %q", c.Structure)
	}
}
