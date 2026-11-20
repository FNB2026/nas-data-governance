package managedfs

import (
	"context"
	"errors"
	"io"
)

// ErrRecoveryRequired never implies that a mutation did or did not happen.
// It prohibits another operation; this prototype has no recovery/clear method.
var ErrRecoveryRequired = errors.New("managed object recovery required")

// CopySpec describes only the managed output. AuthorizationSHA256 binds an
// upstream decision for audit; the primitive cannot validate business approval,
// keeper policy, retention, or source identity and must not be exposed to a UI.
type CopySpec struct {
	Size                int64
	SHA256              string
	AuthorizationSHA256 string
}

// Writer is a dormant execution-layer primitive. SealCopy never removes its
// input. Purge addresses only a previously registered private object. A product
// integration must separately enforce every approval and retention requirement.
// Interface copies share a single non-reentrant lifetime and flock lease.
type Writer interface {
	SealCopy(context.Context, io.Reader, CopySpec) (ObjectProof, error)
	Inspect(context.Context, ObjectProof) error
	Purge(context.Context, ObjectProof, string) error
	Close() error
}
