package authz

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// chainAudit is the dry-run/audit artifact of the chained contract. Every gate
// is recorded with its real outcome; the irreversible commit is never reached.
type chainAudit struct {
	AuthzInitial   Eligibility
	AuthzUnderLock Eligibility
	LockAcquired   bool
	FreshPlanState string
	ExecutorStatus executor.StepStatus
	ExecutorError  string
	Committed      bool
}

// runChainedContract executes the full chain in an isolated synthetic
// environment:
//
//	durable approval facts -> authz.Verifier
//	  -> acquire execution lock -> re-verify under the lock
//	  -> fresh durable read of plan + item
//	  -> executor live-object and mount validation
//	  -> dry-run / audit result only (the commit is never reached)
//
// midHook runs between the re-verification and the fresh read, modeling state
// changes inside the authorization-to-execution window. Both gates really
// execute: the authz verdicts gate the chain, and the executor's read-only
// validation is actually invoked and its result consulted.
func runChainedContract(t *testing.T, ctx context.Context, st *store.SQLiteStore, quarantineRoot string, req Request, midHook func(t *testing.T)) chainAudit {
	t.Helper()
	v := NewVerifier()
	audit := chainAudit{Committed: false}

	// Gate 1: durable authorization facts.
	el, err := v.Check(ctx, NewStoreFacts(st), req)
	if err != nil {
		t.Fatalf("authz gate: %v", err)
	}
	audit.AuthzInitial = el
	if !el.Eligible {
		return audit
	}

	// Reserve the execution lock for the whole re-verification + validation span.
	release, err := st.AcquireExecutionLock()
	if err != nil {
		t.Fatalf("acquire execution lock: %v", err)
	}
	defer release()
	audit.LockAcquired = true

	// Re-verify while holding the lock: our own reservation is not a
	// competing writer's, but journal-based recovery still blocks.
	el2, err := v.Check(ctx, NewStoreFactsOwningExecutionLock(st, func() bool { return true }), req)
	if err != nil {
		t.Fatalf("authz re-check under lock: %v", err)
	}
	audit.AuthzUnderLock = el2
	if !el2.Eligible {
		return audit
	}

	// The controlled window: any state change that lands here is caught by
	// the fresh durable read and the executor's own gates below.
	if midHook != nil {
		midHook(t)
	}

	// Fresh durable read under the lock — no stale in-memory plan.
	plan, err := st.GetPurgePlan(ctx, req.PlanID)
	if err != nil {
		t.Fatalf("fresh plan read: %v", err)
	}
	item, err := st.GetQuarantineItem(ctx, plan.ItemID)
	if err != nil {
		t.Fatalf("fresh item read: %v", err)
	}
	audit.FreshPlanState = string(plan.State)

	// Gate 2: live object + mount binding (read-only; the dry-run gate).
	px, err := executor.NewPurgeExecutor(quarantineRoot, st)
	if err != nil {
		t.Fatalf("purge executor: %v", err)
	}
	result := px.ValidatePurge(ctx, plan, item)
	audit.ExecutorStatus = result.Status
	if result.Err != nil {
		audit.ExecutorError = result.ErrorType
	}
	audit.Committed = false // the irreversible commit is never reached
	return audit
}

// TestChainedContract_SyntheticEndToEnd wires the real contract end to end in
// an isolated synthetic environment.
func TestChainedContract_SyntheticEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("chained contract uses a real store")
	}
	ctx := context.Background()
	dir := t.TempDir()
	sourceRoot := filepath.Join(dir, "source")
	quarantineRoot := filepath.Join(dir, "quarantine")
	for _, d := range []string{sourceRoot, quarantineRoot} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	elapsed := now.Add(-24 * time.Hour)

	t.Run("happy_chain_produces_dry_run_only", func(t *testing.T) {
		st, err := store.Open(ctx, filepath.Join(dir, "chain-happy.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		_, _, _, req := approvedPurge(t, ctx, st, sourceRoot, quarantineRoot, "chain-alpha",
			elapsed, now.Add(-12*time.Hour), elapsed, true)
		planRec, err := st.GetPurgePlan(ctx, req.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		itemRec, err := st.GetQuarantineItem(ctx, planRec.ItemID)
		if err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(itemRec.QuarantinePath)
		if err != nil {
			t.Fatal(err)
		}

		audit := runChainedContract(t, ctx, st, quarantineRoot, req, nil)

		if !audit.AuthzInitial.Eligible {
			t.Fatalf("gate 1 refused: %s (%s)", audit.AuthzInitial.Reason, audit.AuthzInitial.Detail)
		}
		if !audit.LockAcquired {
			t.Fatal("execution lock was not acquired")
		}
		if !audit.AuthzUnderLock.Eligible {
			t.Fatalf("re-verification under lock refused: %s (%s)", audit.AuthzUnderLock.Reason, audit.AuthzUnderLock.Detail)
		}
		if audit.FreshPlanState != string(domain.PurgeApproved) {
			t.Fatalf("fresh plan state: %s", audit.FreshPlanState)
		}
		if audit.ExecutorStatus != executor.StepOK {
			t.Fatalf("executor gate failed: %s (%s)", audit.ExecutorStatus, audit.ExecutorError)
		}
		if audit.Committed {
			t.Fatal("the chain must never reach the irreversible commit")
		}
		after, err := os.ReadFile(itemRec.QuarantinePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatal("the dry-run chain must not modify the object")
		}
	})

	t.Run("zero_byte_object_passes_both_gates", func(t *testing.T) {
		st, err := store.Open(ctx, filepath.Join(dir, "chain-zero.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		item := seedQuarantineItem(t, st, sourceRoot, quarantineRoot, "chain-zero", "",
			now.Add(-48*time.Hour), elapsed)
		digest := strings.Repeat("a", 64)
		planID := "purge-chain-zero"
		if err := st.SavePurgePlans(ctx, []domain.PurgePlan{{
			ID: planID, ItemID: item.ID, State: domain.PurgeDraft,
			ExpectedPath: item.QuarantinePath, ExpectedSHA256: item.ContentSHA256,
			ExpectedSize: item.FileSize, RetainUntil: item.RetainUntil,
			ApprovalDigest: digest, CreatedAt: now.Add(-12 * time.Hour),
		}}); err != nil {
			t.Fatal(err)
		}
		if err := st.ApprovePurgePlan(ctx, planID, digest, now.Add(-30*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if err := st.MarkPurgeDryRunVerified(ctx, planID, digest, now.Add(-20*time.Minute)); err != nil {
			t.Fatal(err)
		}
		req := Request{
			Operation: OpPurge, PlanID: planID, PresentedDigest: digest,
			ObjectID: item.ID, ContentSHA256: item.ContentSHA256,
			Size:   Int64Field(item.FileSize),
			Device: Uint64Field(1), Inode: Uint64Field(2),
		}

		audit := runChainedContract(t, ctx, st, quarantineRoot, req, nil)

		if !audit.AuthzInitial.Eligible || !audit.AuthzUnderLock.Eligible {
			t.Fatalf("zero-byte chain refused: %s / %s", audit.AuthzInitial.Reason, audit.AuthzUnderLock.Reason)
		}
		if audit.ExecutorStatus != executor.StepOK {
			t.Fatalf("zero-byte executor gate failed: %s (%s)", audit.ExecutorStatus, audit.ExecutorError)
		}
	})

	t.Run("object_changed_refused_by_live_gate", func(t *testing.T) {
		st, err := store.Open(ctx, filepath.Join(dir, "chain-drift.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		_, _, _, req := approvedPurge(t, ctx, st, sourceRoot, quarantineRoot, "chain-beta",
			elapsed, now.Add(-12*time.Hour), elapsed, true)

		// Mutate the live bytes mid-chain (same length, different content):
		// the durable facts are untouched, so gate 1 still passes — proving
		// the executor's live gate is independent and indispensable.
		mutate := func(t *testing.T) {
			planRec, err := st.GetPurgePlan(ctx, req.PlanID)
			if err != nil {
				t.Fatal(err)
			}
			itemRec, err := st.GetQuarantineItem(ctx, planRec.ItemID)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(itemRec.QuarantinePath, []byte("XXXX-XXXX"), 0o600); err != nil {
				t.Fatal(err)
			}
		}

		audit := runChainedContract(t, ctx, st, quarantineRoot, req, mutate)

		if !audit.AuthzInitial.Eligible || !audit.AuthzUnderLock.Eligible {
			t.Fatalf("durable authorization should still pass: %s / %s",
				audit.AuthzInitial.Reason, audit.AuthzUnderLock.Reason)
		}
		if audit.ExecutorStatus == executor.StepOK {
			t.Fatal("a changed object must be refused by the live gate")
		}
		if audit.ExecutorError != "preflight_failed" {
			t.Fatalf("expected preflight_failed, got %q", audit.ExecutorError)
		}
		if audit.Committed {
			t.Fatal("refusal must stop before any commit")
		}
	})

	t.Run("approval_invalidated_refused_before_chain", func(t *testing.T) {
		st, err := store.Open(ctx, filepath.Join(dir, "chain-revoked.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		item, plan, _, req := approvedPurge(t, ctx, st, sourceRoot, quarantineRoot, "chain-gamma",
			elapsed, now.Add(-12*time.Hour), elapsed, true)
		if err := st.MarkPurgeRolledBack(ctx, plan.ID, item.ID, now); err != nil {
			t.Fatal(err)
		}

		audit := runChainedContract(t, ctx, st, quarantineRoot, req, nil)

		if audit.AuthzInitial.Eligible {
			t.Fatal("an invalidated approval must be refused at gate 1")
		}
		if audit.AuthzInitial.Reason != ReasonRevoked {
			t.Fatalf("expected revoked, got %s", audit.AuthzInitial.Reason)
		}
		if audit.LockAcquired {
			t.Fatal("the chain must stop before reserving the execution lock")
		}

		// Even bypassing authz, the executor's own state gate refuses.
		px, err := executor.NewPurgeExecutor(quarantineRoot, st)
		if err != nil {
			t.Fatal(err)
		}
		freshPlan, err := st.GetPurgePlan(ctx, req.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		freshItem, err := st.GetQuarantineItem(ctx, freshPlan.ItemID)
		if err != nil {
			t.Fatal(err)
		}
		if result := px.ValidatePurge(ctx, freshPlan, freshItem); result.Status == executor.StepOK {
			t.Fatal("the executor must refuse a rolled-back plan on its own")
		}
	})

	t.Run("in_window_state_change_is_controlled", func(t *testing.T) {
		st, err := store.Open(ctx, filepath.Join(dir, "chain-window.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		item, plan, _, req := approvedPurge(t, ctx, st, sourceRoot, quarantineRoot, "chain-delta",
			elapsed, now.Add(-12*time.Hour), elapsed, true)

		// Revoke inside the authorization-to-execution window.
		hook := func(t *testing.T) {
			if err := st.MarkPurgeRolledBack(ctx, plan.ID, item.ID, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
		}

		audit := runChainedContract(t, ctx, st, quarantineRoot, req, hook)

		if !audit.AuthzInitial.Eligible || !audit.AuthzUnderLock.Eligible {
			t.Fatalf("gates before the window should pass: %s / %s",
				audit.AuthzInitial.Reason, audit.AuthzUnderLock.Reason)
		}
		if audit.FreshPlanState != string(domain.PurgeRolledBack) {
			t.Fatalf("fresh read should see the in-window change, got %s", audit.FreshPlanState)
		}
		if audit.ExecutorStatus == executor.StepOK {
			t.Fatal("the in-window revocation must be caught by the fresh read + executor gate")
		}
		if audit.Committed {
			t.Fatal("the controlled window must never reach a commit")
		}
	})

	t.Run("competing_writer_lock_blocks_chain", func(t *testing.T) {
		st, err := store.Open(ctx, filepath.Join(dir, "chain-lock.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		_, _, _, req := approvedPurge(t, ctx, st, sourceRoot, quarantineRoot, "chain-epsilon",
			elapsed, now.Add(-12*time.Hour), elapsed, true)

		release, err := st.AcquireExecutionLock()
		if err != nil {
			t.Fatal(err)
		}
		blocked := runChainedContract(t, ctx, st, quarantineRoot, req, nil)
		release()

		if blocked.AuthzInitial.Eligible {
			t.Fatal("a competing writer's lock must block the chain at gate 1")
		}
		if blocked.AuthzInitial.Reason != ReasonRecoveryLock {
			t.Fatalf("expected recovery_lock, got %s", blocked.AuthzInitial.Reason)
		}
		if blocked.LockAcquired {
			t.Fatal("the chain must not reserve a lock while another writer holds it")
		}

		// After release the full chain passes.
		audit := runChainedContract(t, ctx, st, quarantineRoot, req, nil)
		if !audit.AuthzInitial.Eligible || audit.ExecutorStatus != executor.StepOK {
			t.Fatalf("chain after release should pass: %s / %s", audit.AuthzInitial.Reason, audit.ExecutorStatus)
		}
	})
}
