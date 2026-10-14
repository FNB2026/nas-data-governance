//go:build darwin

package fingerprint

import (
	"os"

	"golang.org/x/sys/unix"
)

func guardedContextMountProofAvailable() bool { return false }

// Darwin has no equivalent of Linux openat2 RESOLVE_NO_XDEV in this slice.
// Same-device nested mount points cannot be ruled out, so never open a source
// member for this freshness operation on Darwin.
func openRecoveredNoFollowRoot(_, _ string) (*os.File, unix.Stat_t, error) {
	return nil, unix.Stat_t{}, ErrGuardedMountBoundaryUnverified
}
