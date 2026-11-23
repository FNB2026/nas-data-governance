//go:build mas

package wails

const sandboxBuild = true

// The Mac App Store edition is a real file-governance product: user-selected
// source folders are authorized read-write and quarantine/restore execution is
// available through the same approval and stale-check pipeline as other
// desktop channels. Permanent purge remains independently gated by release
// policy.
const sandboxWrites = true
