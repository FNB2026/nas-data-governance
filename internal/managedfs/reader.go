// Package managedfs inspects already sealed objects in an exclusively owned
// private execution root. It grants no source mutation or business approval.
// Untrusted same-UID writers and privileged concurrent remounts are excluded by
// the deployment contract; owner/mode checks alone do not prove that contract.
package managedfs

import (
	"context"
	"errors"
)

var (
	ErrRejected    = errors.New("managed object verification rejected")
	ErrUnsupported = errors.New("managed object verification unavailable")
	ErrBusy        = errors.New("managed object session busy")
	ErrClosed      = errors.New("managed object session closed")
	ErrCancelled   = errors.New("managed object verification cancelled")
)

// RootProof comes from the authoritative initialization/catalog record. A
// fresh observation must never replace a stored, approved identity.
type RootProof struct {
	RootID  string `json:"root_id"`
	StoreID string `json:"store_id"`
	Device  uint64 `json:"device"`
	Inode   uint64 `json:"inode"`
}

// ObjectProof must be persisted by the future copy-in/catalog layer. Session mount IDs
// deliberately are not persisted as authority across process/host restarts.
type ObjectProof struct {
	Root      RootProof `json:"root"`
	ObjectID  string    `json:"object_id"`
	Device    uint64    `json:"device"`
	Inode     uint64    `json:"inode"`
	Size      int64     `json:"size"`
	MTimeSec  int64     `json:"mtime_sec"`
	MTimeNsec uint32    `json:"mtime_nsec"`
	CTimeSec  int64     `json:"ctime_sec"`
	CTimeNsec uint32    `json:"ctime_nsec"`
	SHA256    string    `json:"sha256"`
}

// Reader exposes no closable descriptor or copyable mutex-bearing concrete
// value. Copies of this interface share the same non-reentrant lifetime gate.
// Inspect only checks supplied evidence; it neither mints an approval nor
// establishes the catalog's pending/recovery state.
type Reader interface {
	Inspect(context.Context, ObjectProof) error
	Close() error
}
