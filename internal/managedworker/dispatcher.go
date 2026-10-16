package managedworker

import (
	"context"
)

// Executable reports the operations this worker will actually perform. It is
// the authoritative answer for a caller that wants to know what it can do:
// SEAL and PURGE are deliberately absent until the business authorization,
// retention and keeper contracts exist and are enforced here.
func Executable() []string { return []string{"PING", "STATUS"} }

// Status describes the worker's own capability state. PurgeReleaseAllowed is
// reported from the managed-object package and is never overridable here: a
// caller that can reach the socket still cannot raise it.
type Status struct {
	Ready               bool `json:"ready"`
	PurgeReleaseAllowed bool `json:"purge_release_allowed"`
	Executable          int  `json:"executable_ops"`
}

// Dispatch is the only path from a request to an operation. Object operations
// are refused rather than deferred: there is no fallback branch that would
// perform them, and no flag, environment variable or request field that can
// change this until the authorization contract is implemented.
func Dispatch(ctx context.Context, request Request) (any, error) {
	if ctx.Err() != nil {
		return nil, ErrProtocol
	}
	switch request.Op {
	case "PING":
		return map[string]any{"ok": true}, nil
	case "STATUS":
		return Status{Ready: true, PurgeReleaseAllowed: false, Executable: len(Executable())}, nil
	case "SEAL", "PURGE", "INSPECT":
		// A complete, well-formed request must be refused here too, so the
		// gate is authorization rather than request validation.
		return nil, ErrAuthorization
	}
	return nil, ErrProtocol
}
