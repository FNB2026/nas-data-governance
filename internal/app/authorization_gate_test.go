package app

// The app-side AuthorizationGate lifecycle (A7): single gate assembly for a
// real SQLiteStore, chained pre-lock / under-lock verification, typed
// refusals, default-deny for plan kinds the operation does not own, audit
// of every decision (including forged probes), wrong-lock-identity
// behavior, and fail-closed behavior across store restart.
//
// Boundary statement: the gate decides; it never executes. File writes stay
// in the Executor; wire-level authorization stays in the Worker's own gate.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/authz"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/purge"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// gateEnv seeds one approved restore plan through the real product path:
// a quarantine item registered from a completed execution journal, a DRAFT
// restore plan bound to it, and a durable approval.
type gateEnv struct {
	t              *testing.T
	dir            string
	st             *store.SQLiteStore
	gate           *AuthorizationGate
	item           domain.QuarantineItem
	restorePlan    domain.RestorePlan
	restoreDigest  string
	quarantineRoot string
	sourceRoot     string
}

func newGateEnv(t *testing.T) *gateEnv {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "gate.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	env := &gateEnv{
		t:              t,
		dir:            dir,
		st:             st,
		gate:           NewAuthorizationGate(st),
		quarantineRoot: filepath.Join(dir, "quarantine"),
		sourceRoot:     filepath.Join(dir, "source"),
	}
	for _, d := range []string{env.quarantineRoot, env.sourceRoot} {
		// The gate itself never touches the filesystem; the roots only need
		// to exist for the executor path used in the wiring test.
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env.seedApprovedRestore(ctx)
	return env
}

// seedApprovedRestore registers one quarantined item through the durable
// journal path and approves a restore plan for it.
func (e *gateEnv) seedApprovedRestore(ctx context.Context) {
	e.t.Helper()
	now := time.Now().UTC()
	retainUntil := now.Add(-24 * time.Hour)
	content := "gate-object-payload"
	objectPath := filepath.Join(e.quarantineRoot, "gate-object.txt")
	if err := os.WriteFile(objectPath, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(sum[:])
	size := int64(len(content))
	taskID := "gate-task"
	plan := domain.OperationPlan{
		ID: "gate-exec-plan", TaskID: taskID, State: domain.PlanExecuting,
		Actions: []domain.PlannedAction{{
			Path:    filepath.Join(e.sourceRoot, "gate-object.txt"),
			Action:  domain.OperationQuarantine,
			Context: domain.DirectoryContext{Role: domain.RoleTemporary},
			File:    domain.FileInstance{Path: filepath.Join(e.sourceRoot, "gate-object.txt"), Size: size, ContentSHA256: contentHash},
		}},
	}
	if err := e.st.CreateTask(ctx, domain.OperationTask{ID: taskID, RootPath: e.sourceRoot, State: "completed", CreatedAt: now.Add(-48 * time.Hour)}); err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.SavePlans(ctx, taskID, []domain.OperationPlan{plan}); err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.BeginJournal(ctx, taskID, plan.ID, plan.Actions); err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.MarkJournalDone(ctx, plan.ID, 0, objectPath); err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.CompletePlanExecution(ctx, plan, domain.PlanVerified, nil, now.Add(-48*time.Hour), retainUntil); err != nil {
		e.t.Fatal(err)
	}
	items, err := e.st.ListQuarantineItems(ctx, "")
	if err != nil {
		e.t.Fatal(err)
	}
	for _, it := range items {
		if it.SourcePath == filepath.Join(e.sourceRoot, "gate-object.txt") {
			e.item = it
		}
	}
	if e.item.ID == "" {
		e.t.Fatal("seed: quarantine item not registered")
	}
	restore, err := purge.BuildRestorePlan(e.item, now)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := e.st.SaveRestorePlan(ctx, restore); err != nil {
		e.t.Fatal(err)
	}
	// The durable digest is the deterministic one the planner bound to the
	// plan identity — approval requires exactly it.
	e.restoreDigest = restore.ApprovalDigest
	if err := e.st.ApproveRestorePlan(ctx, restore.ID, e.restoreDigest, now.Add(-30*time.Minute)); err != nil {
		e.t.Fatal(err)
	}
	fresh, err := e.st.GetRestorePlan(ctx, restore.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	e.restorePlan = fresh
}

func TestAuthorizationGate_Restore(t *testing.T) {
	if testing.Short() {
		t.Skip("gate lifecycle uses a real store")
	}
	ctx := context.Background()

	t.Run("approved_restore_is_eligible_and_audited", func(t *testing.T) {
		env := newGateEnv(t)
		el, err := env.gate.AuthorizeRestore(ctx, env.restorePlan.ID, env.restoreDigest)
		if err != nil {
			t.Fatalf("the approved restore must pass the gate: %v", err)
		}
		if !el.Eligible {
			t.Fatalf("eligible verdict expected, got %s (%s)", el.Reason, el.Detail)
		}
		// The decision is on the durable audit record.
		records, err := env.st.ListAuthorizationDecisions(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 1 {
			t.Fatalf("exactly one audit record expected, got %d", len(records))
		}
		rec := records[0]
		if !rec.Eligible || rec.Operation != "RESTORE" || rec.PlanID != env.restorePlan.ID || rec.ItemID != env.item.ID {
			t.Fatalf("audit record mismatch: %+v", rec)
		}
		if rec.OwnershipVerified {
			t.Fatal("pre-lock decision must not claim ownership")
		}
	})

	t.Run("forged_digest_is_refused_and_audited", func(t *testing.T) {
		env := newGateEnv(t)
		forged := strings.Repeat("e", 64)
		el, err := env.gate.AuthorizeRestore(ctx, env.restorePlan.ID, forged)
		if err == nil {
			t.Fatal("a forged digest must be refused")
		}
		if el.Eligible {
			t.Fatalf("refusal expected, got eligible (%s)", el.Reason)
		}
		var refusal *AuthorizationRefusal
		if !errors.As(err, &refusal) || refusal.Reason != string(authz.ReasonTampered) {
			t.Fatalf("typed tampered refusal expected, got %v", err)
		}
		records, err := env.st.ListAuthorizationDecisions(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 1 || records[0].Eligible || records[0].Reason != string(authz.ReasonTampered) {
			t.Fatalf("the refusal must be audited: %+v (err %v)", records, err)
		}
	})

	t.Run("unknown_plan_probe_is_refused_and_still_audited", func(t *testing.T) {
		env := newGateEnv(t)
		el, err := env.gate.AuthorizeRestore(ctx, "no-such-plan", env.restoreDigest)
		if err == nil || el.Eligible {
			t.Fatalf("a forged plan probe must be refused, got %s (%v)", el.Reason, err)
		}
		// The refusal of a nonexistent plan must still reach the audit
		// stream — this is exactly the record worth keeping.
		records, err := env.st.ListAuthorizationDecisions(ctx, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 1 || records[0].Eligible || records[0].PlanID != "no-such-plan" {
			t.Fatalf("unknown-plan refusal must be audited: %+v", records)
		}
	})

	t.Run("purge_plan_cannot_authorize_restore", func(t *testing.T) {
		env := newGateEnv(t)
		// A restore plan id under the purge entry point is structurally
		// unknown to the purge fact source.
		el, err := env.gate.AuthorizePurge(ctx, env.restorePlan.ID, env.restoreDigest, 0, 0)
		if err == nil || el.Eligible || el.Reason != authz.ReasonWrongPlan {
			t.Fatalf("cross-operation reuse must be wrong_plan, got %s (%v)", el.Reason, err)
		}
	})

	t.Run("restore_gate_does_not_inherit_retention_precondition", func(t *testing.T) {
		// The seeded item's retention already elapsed; this case pins that
		// the gate's eligibility rests on the restore contract, not on purge
		// retention. A future seed with running retention must stay
		// eligible for restore — covered in restore_contract_test.go at the
		// verifier level; here we pin the app gate passes through the same
		// verdict shape.
		env := newGateEnv(t)
		el, err := env.gate.AuthorizeRestore(ctx, env.restorePlan.ID, env.restoreDigest)
		if err != nil || !el.Eligible {
			t.Fatalf("gate must mirror the verifier's restore contract: %v (%s)", err, el.Reason)
		}
	})
}

func TestAuthorizationGate_ChainedOwnership(t *testing.T) {
	if testing.Short() {
		t.Skip("gate lifecycle uses a real store")
	}
	ctx := context.Background()

	t.Run("wrong_lock_identity_is_not_ownership", func(t *testing.T) {
		env := newGateEnv(t)
		// A second, independent store: its scope is real ownership of THAT
		// database, never of this one.
		otherDir := t.TempDir()
		other, err := store.Open(ctx, filepath.Join(otherDir, "other.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer other.Close() //nolint:errcheck
		foreign, err := authz.AcquireExecutionLockScope(other)
		if err != nil {
			t.Fatal(err)
		}
		defer foreign.Release()
		el, err := env.gate.AuthorizeRestoreWithScope(ctx, env.restorePlan.ID, env.restoreDigest, foreign)
		if err != nil {
			t.Fatalf("a foreign scope is not ownership, but the free-lock decision must still stand: %v", err)
		}
		if !el.Eligible {
			t.Fatalf("eligible expected under a foreign scope (plain probe governs), got %s", el.Reason)
		}
		records, _ := env.st.ListAuthorizationDecisions(ctx, 1)
		if len(records) != 1 || records[0].OwnershipVerified {
			t.Fatalf("the foreign scope must not be recorded as ownership: %+v", records)
		}
	})

	t.Run("held_lock_blocks_plain_check_but_owned_scope_passes", func(t *testing.T) {
		env := newGateEnv(t)
		// Someone else (in production: another process) holds the lock.
		held, err := authz.AcquireExecutionLockScope(env.st)
		if err != nil {
			t.Fatal(err)
		}
		defer held.Release()

		// Pre-lock phase: the probe sees a held lock it does not own.
		el, err := env.gate.AuthorizeRestore(ctx, env.restorePlan.ID, env.restoreDigest)
		if err == nil || el.Eligible || el.Reason != authz.ReasonRecoveryLock {
			t.Fatalf("a held foreign lock must refuse with recovery_lock, got %s (%v)", el.Reason, err)
		}
		// The owner of the lock re-verifies: ownership is proven by scope.
		el2, err := env.gate.AuthorizeRestoreWithScope(ctx, env.restorePlan.ID, env.restoreDigest, held)
		if err != nil || !el2.Eligible {
			t.Fatalf("the scope's own owner must pass: %v (%s)", err, el2.Reason)
		}
		// And the audit records the difference between the two phases.
		records, err := env.st.ListAuthorizationDecisions(ctx, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 2 {
			t.Fatalf("two audit records expected, got %d", len(records))
		}
		ownByScope, ownByProbe := records[0].OwnershipVerified, records[1].OwnershipVerified
		if !ownByScope || ownByProbe {
			t.Fatalf("ownership flag must follow the scope, not the probe: %+v", records)
		}
	})

	t.Run("released_scope_stops_being_ownership", func(t *testing.T) {
		env := newGateEnv(t)
		scope, err := authz.AcquireExecutionLockScope(env.st)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := env.gate.AuthorizeRestoreWithScope(ctx, env.restorePlan.ID, env.restoreDigest, scope); err != nil {
			t.Fatalf("owned scope must pass: %v", err)
		}
		scope.Release()
		// After release the plain probe governs again — the lock is free,
		// so the decision still stands, but ownership is no longer claimed.
		el, err := env.gate.AuthorizeRestoreWithScope(ctx, env.restorePlan.ID, env.restoreDigest, scope)
		if err != nil || !el.Eligible {
			t.Fatalf("released scope falls back to the free-lock probe: %v (%s)", err, el.Reason)
		}
		records, _ := env.st.ListAuthorizationDecisions(ctx, 1)
		if len(records) != 1 || records[0].OwnershipVerified {
			t.Fatalf("a released scope must not be recorded as ownership: %+v", records)
		}
	})
}

func TestAuthorizationGate_Lifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("gate lifecycle uses a real store")
	}
	ctx := context.Background()

	t.Run("closed_store_fails_closed_across_restart", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "restart.db")
		st, err := store.Open(ctx, dbPath)
		if err != nil {
			t.Fatal(err)
		}
		gate := NewAuthorizationGate(st)
		// Seed through the same store, then close it.
		env := &gateEnv{t: t, dir: dir, st: st, gate: gate,
			quarantineRoot: filepath.Join(dir, "quarantine"),
			sourceRoot:     filepath.Join(dir, "source")}
		for _, d := range []string{env.quarantineRoot, env.sourceRoot} {
			if err := os.MkdirAll(d, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		env.seedApprovedRestore(ctx)
		if err := st.Close(); err != nil {
			t.Fatal(err)
		}
		// A decision against the closed store must fail closed — never
		// silently eligible.
		if _, err := gate.AuthorizeRestore(ctx, env.restorePlan.ID, env.restoreDigest); err == nil {
			t.Fatal("a closed store must fail the gate")
		}
		// Restart: a new gate on the reopened store decides again.
		st2, err := store.Open(ctx, dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer st2.Close() //nolint:errcheck
		gate2 := NewAuthorizationGate(st2)
		el, err := gate2.AuthorizeRestore(ctx, env.restorePlan.ID, env.restoreDigest)
		if err != nil || !el.Eligible {
			t.Fatalf("after restart the same durable approval must authorize: %v (%s)", err, el.Reason)
		}
	})

	t.Run("gate_is_bound_to_its_own_store", func(t *testing.T) {
		dir := t.TempDir()
		st, err := store.Open(ctx, filepath.Join(dir, "a.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close() //nolint:errcheck
		other, err := store.Open(ctx, filepath.Join(dir, "b.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer other.Close() //nolint:errcheck
		if _, err := NewQuarantineServiceWithGate(st, NewAuthorizationGate(other)); err == nil {
			t.Fatal("a gate bound to another store must not be shareable")
		}
		if _, err := NewQuarantineServiceWithGate(st, nil); err == nil {
			t.Fatal("a nil gate must be a construction error")
		}
		if _, err := NewQuarantineServiceWithGate(st, NewAuthorizationGate(st)); err != nil {
			t.Fatalf("a gate bound to the same store must assemble: %v", err)
		}
	})
}
