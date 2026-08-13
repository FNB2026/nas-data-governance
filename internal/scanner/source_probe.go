package scanner

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SourceStatus is a privacy-safe, protocol-neutral source availability state.
type SourceStatus string

const (
	SourceOnline      SourceStatus = "online"
	SourceUnavailable SourceStatus = "unavailable"
)

// SourceProfile describes the filesystem properties that affect safe scanning.
// It intentionally contains no path, host, account, or share name so callers
// may include it in ordinary UI state and aggregate diagnostics.
type SourceProfile struct {
	Status                   SourceStatus
	FilesystemType           string
	Network                  bool
	PhysicalIdentityReliable bool
	Latency                  time.Duration
	RecommendedWorkers       int
}

// ProbeError reports a path-free source preflight failure.
type ProbeError struct {
	Code string
}

func (e *ProbeError) Error() string {
	switch e.Code {
	case "timeout":
		return "scanner: source preflight timed out; path omitted"
	case "not_found":
		return "scanner: source is unavailable; path omitted"
	case "permission_denied":
		return "scanner: source permission denied; path omitted"
	case "not_directory":
		return "scanner: source is not a directory; path omitted"
	case "symlink_rejected":
		return "scanner: source root must not be a symbolic link; path omitted"
	default:
		return "scanner: source preflight failed; path omitted"
	}
}

type sourceProbeResult struct {
	filesystemType string
	network        bool
	reliable       bool
	err            error
}

// ProbeSource performs a bounded, read-only source preflight. It stats the
// root, opens the directory, and requests at most one entry. It never opens a
// file, follows a symlink, traverses a child directory, or writes to the source.
//
// Some network filesystem syscalls are not cancellable by Go. The inspection
// therefore runs behind a buffered result channel so the caller can stop
// waiting when ctx expires. At most one in-flight syscall remains for a timed
// out probe, and its result is discarded when it eventually returns.
func ProbeSource(ctx context.Context, root string) (SourceProfile, error) {
	if err := ctx.Err(); err != nil {
		return SourceProfile{Status: SourceUnavailable}, &ProbeError{Code: "timeout"}
	}
	absRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil || strings.TrimSpace(root) == "" {
		return SourceProfile{Status: SourceUnavailable}, &ProbeError{Code: "unavailable"}
	}

	started := time.Now()
	resultCh := make(chan sourceProbeResult, 1)
	go func() {
		resultCh <- inspectSource(absRoot)
	}()

	select {
	case <-ctx.Done():
		return SourceProfile{Status: SourceUnavailable, Latency: time.Since(started)}, &ProbeError{Code: "timeout"}
	case result := <-resultCh:
		latency := time.Since(started)
		if result.err != nil {
			return SourceProfile{Status: SourceUnavailable, Latency: latency}, result.err
		}
		return SourceProfile{
			Status:                   SourceOnline,
			FilesystemType:           result.filesystemType,
			Network:                  result.network,
			PhysicalIdentityReliable: result.reliable,
			Latency:                  latency,
			RecommendedWorkers:       recommendedWorkers(result.network, latency),
		}, nil
	}
}

func inspectSource(root string) sourceProbeResult {
	info, err := os.Lstat(root)
	if err != nil {
		return sourceProbeResult{err: classifyProbeError(err)}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return sourceProbeResult{err: &ProbeError{Code: "symlink_rejected"}}
	}
	if !info.IsDir() {
		return sourceProbeResult{err: &ProbeError{Code: "not_directory"}}
	}

	dir, err := os.Open(root)
	if err != nil {
		return sourceProbeResult{err: classifyProbeError(err)}
	}
	_, readErr := dir.Readdirnames(1)
	closeErr := dir.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return sourceProbeResult{err: classifyProbeError(readErr)}
	}
	if closeErr != nil {
		return sourceProbeResult{err: &ProbeError{Code: "unavailable"}}
	}

	fsType, network, reliable := filesystemProfile(root)
	return sourceProbeResult{filesystemType: fsType, network: network, reliable: reliable}
}

func classifyProbeError(err error) error {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return &ProbeError{Code: "not_found"}
	case errors.Is(err, os.ErrPermission):
		return &ProbeError{Code: "permission_denied"}
	default:
		return &ProbeError{Code: "unavailable"}
	}
}

func recommendedWorkers(network bool, latency time.Duration) int {
	if !network {
		return 4
	}
	if latency >= 250*time.Millisecond {
		return 1
	}
	return 2
}

func profileForFilesystem(fsType string) (normalized string, network, reliable bool) {
	normalized = strings.ToLower(strings.TrimSpace(fsType))
	switch normalized {
	case "smbfs", "nfs", "afpfs", "webdav", "fusefs", "osxfuse", "macfuse":
		return normalized, true, false
	case "":
		return "unknown", false, false
	default:
		return normalized, false, true
	}
}
