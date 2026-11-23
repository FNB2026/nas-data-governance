//go:build darwin

package wails

// macOS uses descriptor-relative O_NOFOLLOW opens plus per-component device and
// statfs mount-identity equality. The proof covers the descriptor actually
// read and refuses same-device bind/nullfs-style mount crossings.
func governanceFreshnessMountProofAvailable() bool { return true }
