package authz

import (
	"sync"
	"sync/atomic"

	"github.com/FNB2026/nas-data-governance/internal/store"
)

// ExecutionLockScope is the ownership truth of one real execution-lock
// reservation. It closes the A6 window: instead of a caller-supplied callback
// that can keep reporting "I own the lock" after the lock is gone, ownership
// is managed by the same release handle the lock was acquired with.
//
// Invariants:
//   - The only way to obtain a scope is AcquireExecutionLockScope, which
//     performs the real flock. An owner claim that was never acquired cannot
//     be constructed (a zero-value scope reports Owns() == false).
//   - Owns() flips to false, atomically and permanently, at the first
//     Release() — including a release that races with concurrent readers.
//   - The scope is bound to the canonical path of the store it was acquired
//     from. A scope from one database is never ownership evidence for
//     another: the flock lives on that path's ".execution.lock" sibling.
//   - Release is idempotent; every call after the first is a no-op.
//
// Production code must pass scopes only to NewStoreFactsWithScope; there is
// no constructor anywhere that accepts a caller-supplied owns-callback.
type ExecutionLockScope struct {
	mu      sync.Mutex
	owned   atomic.Bool
	boundDB string
	release func()
}

// AcquireExecutionLockScope reserves the execution lock of st and wraps the
// real release handle in a scope whose ownership flips truthfully. The store
// path is captured at acquisition, so the binding cannot drift afterwards.
func AcquireExecutionLockScope(st *store.SQLiteStore) (*ExecutionLockScope, error) {
	release, err := st.AcquireExecutionLock()
	if err != nil {
		return nil, err
	}
	s := &ExecutionLockScope{
		boundDB: st.Path(),
		release: release,
	}
	s.owned.Store(true)
	return s, nil
}

// Owns reports whether this scope currently holds the execution lock. It is
// safe for concurrent use. After Release — however raced — it is false
// forever.
func (s *ExecutionLockScope) Owns() bool {
	if s == nil {
		return false
	}
	return s.owned.Load()
}

// BoundStorePath reports the canonical database path the scope was acquired
// against. A scope is ownership evidence only for a fact source built on the
// same path.
func (s *ExecutionLockScope) BoundStorePath() string {
	if s == nil {
		return ""
	}
	return s.boundDB
}

// Covers reports that this scope is currently a truthful ownership claim for
// the given store: same database identity and still owned. This is the only
// predicate the fact source consults.
func (s *ExecutionLockScope) Covers(st *store.SQLiteStore) bool {
	if s == nil || st == nil {
		return false
	}
	return s.owned.Load() && s.boundDB == st.Path()
}

// Release releases the underlying flock exactly once. Later calls are no-ops.
// After the first call Owns() is permanently false, even if the underlying
// unlock itself failed (the reservation is considered gone either way: the
// flock may already be lost, and a caller must never re-verify on the strength
// of a scope it released).
func (s *ExecutionLockScope) Release() {
	if s == nil {
		return
	}
	if !s.owned.CompareAndSwap(true, false) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.release != nil {
		s.release()
	}
}
