//go:build darwin

package wails

// macOS currently has no implementation in this slice that proves a rooted
// read cannot cross a same-device nested mount. Governance decision previews
// therefore remain UNVERIFIED until an equivalent native boundary exists.
func governanceFreshnessMountProofAvailable() bool { return false }
