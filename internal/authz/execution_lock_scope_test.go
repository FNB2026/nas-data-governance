package authz

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/store"
)

// The A6 mutation and fault suite. Every scenario models one way a lock-
// ownership claim could diverge from the real flock, and proves the
// scope-based design refuses it. There is no constructor anywhere that
// accepts a caller-supplied owns-callback, so a claim that outlives the lock
// cannot be built in production code — each test below demonstrates the
// refusal the design guarantees rather than a defect that still exists.

// openScopeStore opens one isolated store with its synthetic roots, returning
// the store and an approved-purge request for the named scenario.
func openScopeStore(t *testing.T, ctx context.Context, dir, db, name string, elapsed, now time.Time) (*store.SQLiteStore, Request) {
	t.Helper()
	sourceRoot := filepath.Join(dir, "source")
	quarantineRoot := filepath.Join(dir, "quarantine")
	for _, d := range []string{sourceRoot, quarantineRoot} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(ctx, filepath.Join(dir, db))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, req := approvedPurge(t, ctx, st, sourceRoot, quarantineRoot, name,
		elapsed, now.Add(-12*time.Hour), elapsed, true)
	return st, req
}

// TestExecutionLockScope_OwnershipTruth is the direct unit contract of the
// scope: ownership begins only with a real acquisition, ends at the first
// release, never resumes, and is bound to exactly one store identity.
func TestExecutionLockScope_OwnershipTruth(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	stA, err := store.Open(ctx, filepath.Join(dir, "unit-a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer stA.Close()
	stB, err := store.Open(ctx, filepath.Join(dir, "unit-b.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer stB.Close()

	// A zero-value scope is not an owner: ownership cannot be asserted into
	// existence without the real flock.
	var forged ExecutionLockScope
	if forged.Owns() {
		t.Fatal("a zero-value scope must not report ownership")
	}
	if forged.Covers(stA) {
		t.Fatal("a zero-value scope must not cover any store")
	}
	var nilScope *ExecutionLockScope
	if nilScope.Owns() || nilScope.Covers(stA) {
		t.Fatal("a nil scope must not report ownership")
	}

	scope, err := AcquireExecutionLockScope(stA)
	if err != nil {
		t.Fatal(err)
	}
	if !scope.Owns() {
		t.Fatal("a freshly acquired scope must own the lock")
	}
	if !scope.Covers(stA) {
		t.Fatal("the scope must cover the store it was acquired from")
	}
	if scope.Covers(stB) {
		t.Fatal("the scope must not cover a different store")
	}
	if scope.BoundStorePath() != stA.Path() {
		t.Fatalf("scope bound to %q, store path %q", scope.BoundStorePath(), stA.Path())
	}

	// Release is idempotent and permanent.
	scope.Release()
	scope.Release()
	if scope.Owns() {
		t.Fatal("ownership must end at the first release")
	}
	if scope.Covers(stA) {
		t.Fatal("a released scope must not cover its own store")
	}

	// The lock is really free after release: the same store can reserve it
	// again through a fresh scope.
	scope2, err := AcquireExecutionLockScope(stA)
	if err != nil {
		t.Fatalf("re-acquisition after release failed: %v", err)
	}
	scope2.Release()
}

// TestExecutionLockScope_ReleasedButClaimed models the primary A6 defect: the
// lock is released but the ownership claim still says true. With the scope
// design the claim dies with the release, so a competitor that acquires the
// freed lock immediately blocks re-verification.
func TestExecutionLockScope_ReleasedButClaimed(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Now().UTC()
	elapsed := now.Add(-24 * time.Hour)
	st, req := openScopeStore(t, ctx, dir, "a6-released.db", "a6-release", elapsed, now)
	defer st.Close()
	v := NewVerifier()

	scope, err := AcquireExecutionLockScope(st)
	if err != nil {
		t.Fatal(err)
	}
	el, err := v.Check(ctx, NewStoreFactsWithScope(st, scope), req)
	if err != nil || !el.Eligible {
		t.Fatalf("re-verification under our own scope must pass, got %v (%s)", err, el.Reason)
	}

	// Release, then a competing writer takes the freed lock. The stale scope
	// no longer claims ownership, so the competitor's lock must block.
	scope.Release()
	releaseCompetitor, err := st.AcquireExecutionLock()
	if err != nil {
		t.Fatalf("competitor could not acquire the freed lock: %v", err)
	}
	defer releaseCompetitor()

	el, err = v.Check(ctx, NewStoreFactsWithScope(st, scope), req)
	if err != nil {
		t.Fatal(err)
	}
	if el.Eligible {
		t.Fatal("a released scope must not neutralize a competing writer's lock")
	}
	if el.Reason != ReasonRecoveryLock {
		t.Fatalf("expected recovery_lock, got %s", el.Reason)
	}
}

// TestExecutionLockScope_ImpersonationRefused proves a non-holder cannot
// impersonate the holder: the only constructor performs the real flock, so
// the impersonator either fails to acquire or — if the lock happens to be
// free — genuinely becomes the holder. No path claims ownership without
// holding the lock.
func TestExecutionLockScope_ImpersonationRefused(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Now().UTC()
	elapsed := now.Add(-24 * time.Hour)
	st, req := openScopeStore(t, ctx, dir, "a6-impersonation.db", "a6-imperson", elapsed, now)
	defer st.Close()
	v := NewVerifier()

	// The holder reserves the lock.
	holder, err := AcquireExecutionLockScope(st)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()

	// An impersonator tries to obtain a scope for the same store while it is
	// held: the real flock refuses the acquisition — there is no way to
	// construct a scope that claims ownership of a lock someone else holds.
	if _, err := AcquireExecutionLockScope(st); err == nil {
		t.Fatal("impersonator must not be able to acquire the held lock")
	}

	// And the facts layer refuses to treat any non-covering scope as
	// ownership: a zero-value or foreign scope leaves the held probe as a
	// blocking recovery state.
	el, err := v.Check(ctx, NewStoreFactsWithScope(st, &ExecutionLockScope{}), req)
	if err != nil {
		t.Fatal(err)
	}
	if el.Eligible || el.Reason != ReasonRecoveryLock {
		t.Fatalf("zero-value scope must not impersonate ownership, got %s", el.Reason)
	}
	el, err = v.Check(ctx, NewStoreFactsWithScope(st, nil), req)
	if err != nil {
		t.Fatal(err)
	}
	if el.Eligible || el.Reason != ReasonRecoveryLock {
		t.Fatalf("nil scope must not impersonate ownership, got %s", el.Reason)
	}
}

// TestExecutionLockScope_WrongDatabaseIdentity proves a scope from one
// database is never ownership evidence for another, even when both stores are
// open in the same process.
func TestExecutionLockScope_WrongDatabaseIdentity(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Now().UTC()
	elapsed := now.Add(-24 * time.Hour)
	stA, reqA := openScopeStore(t, ctx, dir, "a6-wrongdb-a.db", "a6-wrongdb-a", elapsed, now)
	defer stA.Close()
	stB, reqB := openScopeStore(t, ctx, dir, "a6-wrongdb-b.db", "a6-wrongdb-b", elapsed, now)
	defer stB.Close()
	v := NewVerifier()

	scopeA, err := AcquireExecutionLockScope(stA)
	if err != nil {
		t.Fatal(err)
	}
	defer scopeA.Release()

	// B's lock is held by a competitor. A's scope must not neutralize it.
	releaseCompetitorB, err := stB.AcquireExecutionLock()
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCompetitorB()

	if scopeA.Covers(stB) {
		t.Fatal("a scope bound to A must not cover B")
	}
	el, err := v.Check(ctx, NewStoreFactsWithScope(stB, scopeA), reqB)
	if err != nil {
		t.Fatal(err)
	}
	if el.Eligible || el.Reason != ReasonRecoveryLock {
		t.Fatalf("A's scope must not clear B's competing lock, got %s", el.Reason)
	}

	// And symmetrically, re-verifying A's request against B's store cannot
	// borrow A's ownership either: the plan is unknown in B, which refuses
	// (wrong_plan, the reviewed short-circuit for unknown plans) before any
	// lock consideration — either way, fail-closed.
	el, err = v.Check(ctx, NewStoreFactsWithScope(stB, scopeA), reqA)
	if err != nil {
		t.Fatal(err)
	}
	if el.Eligible {
		t.Fatal("A's scope must not authorize against B's store")
	}
	if el.Reason != ReasonWrongPlan {
		t.Fatalf("expected wrong_plan for a plan unknown in B, got %s", el.Reason)
	}
}

// TestExecutionLockScope_MidVerificationRelease proves that a release inside
// the authorization window stops being ownership for every later facts read:
// the second re-verification with the same (now stale) scope sees the world
// exactly as a non-owner would.
func TestExecutionLockScope_MidVerificationRelease(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Now().UTC()
	elapsed := now.Add(-24 * time.Hour)
	st, req := openScopeStore(t, ctx, dir, "a6-mid.db", "a6-mid", elapsed, now)
	defer st.Close()
	v := NewVerifier()

	scope, err := AcquireExecutionLockScope(st)
	if err != nil {
		t.Fatal(err)
	}
	el, err := v.Check(ctx, NewStoreFactsWithScope(st, scope), req)
	if err != nil || !el.Eligible {
		t.Fatalf("first re-verification under the scope must pass, got %v (%s)", err, el.Reason)
	}

	// The mid-verification release. No competitor yet: the lock is genuinely
	// free afterwards, so eligibility persists — but through the probe, not
	// through any ownership claim.
	scope.Release()
	if scope.Owns() {
		t.Fatal("scope must stop owning immediately at release")
	}
	el, err = v.Check(ctx, NewStoreFactsWithScope(st, scope), req)
	if err != nil || !el.Eligible {
		t.Fatalf("after release with a free lock, eligibility comes from the free probe, got %v (%s)", err, el.Reason)
	}

	// A competitor now takes the lock: the stale scope must not clear it.
	releaseCompetitor, err := st.AcquireExecutionLock()
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCompetitor()
	el, err = v.Check(ctx, NewStoreFactsWithScope(st, scope), req)
	if err != nil {
		t.Fatal(err)
	}
	if el.Eligible || el.Reason != ReasonRecoveryLock {
		t.Fatalf("stale scope must not clear a competitor acquired after release, got %s", el.Reason)
	}
}

// TestExecutionLockScope_ConcurrentOwnershipReads hammers Owns/Covers readers
// against Release under the race detector: ownership flips atomically, no
// reader ever observes a torn state, and the underlying unlock runs exactly
// once.
func TestExecutionLockScope_ConcurrentOwnershipReads(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "a6-race.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	scope, err := AcquireExecutionLockScope(st)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = scope.Owns()
					_ = scope.Covers(st)
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		scope.Release()
	}()
	// Wait for the releaser, then stop the readers.
	released := make(chan struct{})
	go func() {
		for scope.Owns() {
			time.Sleep(time.Millisecond)
		}
		close(released)
	}()
	<-released
	close(stop)
	wg.Wait()

	// After the dust settles the scope is permanently unowned and the lock
	// really is free.
	if scope.Owns() || scope.Covers(st) {
		t.Fatal("scope must be permanently unowned after the racing release")
	}
	if held, err := st.ExecutionLockHeld(); err != nil || held {
		t.Fatalf("lock must be free after release, got held=%v err=%v", held, err)
	}
}

// TestExecutionLockScope_PendingJournalStillBlocks proves journal-based
// recovery evidence blocks even when the scope genuinely owns the execution
// lock: ownership never overrides durable recovery state.
func TestExecutionLockScope_PendingJournalStillBlocks(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Now().UTC()
	elapsed := now.Add(-24 * time.Hour)
	st, req := openScopeStore(t, ctx, dir, "a6-journal.db", "a6-journal", elapsed, now)
	defer st.Close()
	v := NewVerifier()

	scope, err := AcquireExecutionLockScope(st)
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Release()

	// A recoverable purge journal (BeginPurge without completion) is durable
	// recovery evidence, independent of the lock probe.
	plan, err := st.GetPurgePlan(ctx, req.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	item, err := st.GetQuarantineItem(ctx, plan.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	stagingPath := filepath.Join(dir, "staging-a6-journal")
	if err := st.BeginPurge(ctx, plan, item, stagingPath, now); err != nil {
		t.Fatal(err)
	}

	el, err := v.Check(ctx, NewStoreFactsWithScope(st, scope), req)
	if err != nil {
		t.Fatal(err)
	}
	if el.Eligible || el.Reason != ReasonRecoveryLock {
		t.Fatalf("owned scope must not override a pending recovery journal, got %s", el.Reason)
	}
}

// TestExecutionLockScope_UnknownLockStateFailsClosed proves an inaccessible
// lock file is an error — a refusal — even under an owned scope: ownership
// never converts an unknown lock state into a known-free one.
func TestExecutionLockScope_UnknownLockStateFailsClosed(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	now := time.Now().UTC()
	elapsed := now.Add(-24 * time.Hour)
	st, req := openScopeStore(t, ctx, dir, "a6-unknown.db", "a6-unknown", elapsed, now)
	defer st.Close()
	v := NewVerifier()

	scope, err := AcquireExecutionLockScope(st)
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Release()

	// Corrupt the lock surface: replace the lock file path with a directory
	// so the probe can neither confirm free nor confirm held. Note the scope
	// holds an open descriptor to the old file, but the probe opens by path,
	// which now resolves to the directory.
	if err := os.Remove(st.Path() + ".execution.lock"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(st.Path()+".execution.lock", 0o700); err != nil {
		t.Fatal(err)
	}

	_, err = v.Check(ctx, NewStoreFactsWithScope(st, scope), req)
	if err == nil {
		t.Fatal("unknown lock state must be an error under an owned scope")
	}
}
