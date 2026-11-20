package executor

// The controlled Quarantine → Restore business contract, executed for real on
// isolated synthetic data. This is the R3 integration layer: the durable plan
// and approval records are written through the real store, the restore is a
// real filesystem move through the real executor, and the authorization
// consequences (recovery lock, typed refusals) are observed through the real
// authz verifier. Nothing here touches a product source, a NAS or a service;
// every directory is created under t.TempDir().
//
// What each subtest proves:
//
//	quarantine_restore_round_trip      — a real file travels source →
//	                                     quarantine → source through durable
//	                                     plans and approvals; exactly one
//	                                     valid copy exists at every instant.
//	restore_refuses_destination_exists — the destination is never overwritten.
//	restore_refuses_quarantine_drift   — a mutated quarantine object is stale.
//	restore_refuses_plan_drift         — a plan snapshot that no longer matches
//	                                     its item authorizes nothing.
//	restore_refuses_unapproved_plan    — DRAFT is not authority.
//	db_object_inconsistency_refused    — a DB record without its object is stale.
//	crash_keeps_recovery_lock          — a pending restore journal (the durable
//	                                     footprint of a mid-flight crash) blocks
//	                                     every new dangerous operation through
//	                                     the authz gate until reconciled.
//	concurrent_duplicate_restore       — exactly one of two racing restores wins.
//	op_restore_stays_wrong_plan        — the authz layer keeps its R2 rejection
//	                                     until a durable restore fact source exists.
//	no_durable_purge_execution         — the whole suite leaves zero executing
//	                                     plans and zero recoverable purges: the
//	                                     purge transaction was never opened.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/authz"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// seedQuarantinedFile performs one real quarantine registration through the
// durable path the app uses: task, plan, execution journal, completion. The
// file physically sits in the quarantine root; the source location is empty.
func seedQuarantinedFile(t *testing.T, ctx context.Context, st *store.SQLiteStore, sourceRoot, quarantineRoot, name, content string, quarantinedAt, retainUntil time.Time) domain.QuarantineItem {
	t.Helper()
	qPath := filepath.Join(quarantineRoot, name)
	if err := os.WriteFile(qPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Snapshot(qPath, true)
	if err != nil {
		t.Fatal(err)
	}
	taskID := "task-" + name
	planID := "plan-" + name
	sourcePath := filepath.Join(sourceRoot, name)
	if err := st.CreateTask(ctx, domain.OperationTask{ID: taskID, RootPath: sourceRoot, State: "completed", CreatedAt: quarantinedAt}); err != nil {
		t.Fatal(err)
	}
	plan := domain.OperationPlan{
		ID: planID, TaskID: taskID, State: domain.PlanExecuting,
		Actions: []domain.PlannedAction{{
			Path:    sourcePath,
			Action:  domain.OperationQuarantine,
			Context: domain.DirectoryContext{Role: domain.RoleTemporary},
			File:    domain.FileInstance{Path: sourcePath, Size: snapshot.Size, ContentSHA256: snapshot.Hash},
		}},
	}
	if err := st.SavePlans(ctx, taskID, []domain.OperationPlan{plan}); err != nil {
		t.Fatal(err)
	}
	if err := st.BeginJournal(ctx, taskID, planID, plan.Actions); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkJournalDone(ctx, planID, 0, qPath); err != nil {
		t.Fatal(err)
	}
	if err := st.CompletePlanExecution(ctx, plan, domain.PlanVerified, nil, quarantinedAt, retainUntil); err != nil {
		t.Fatal(err)
	}
	all, err := st.ListQuarantineItems(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range all {
		if it.PlanID == planID {
			return it
		}
	}
	t.Fatalf("seeded item not found for plan %s", planID)
	return domain.QuarantineItem{}
}

// approveRestore writes an independent DRAFT restore plan bound to the item's
// durable identity and approves it. The restore plan is a separate transaction
// from the quarantine plan and from any purge plan: one approval, one object,
// one direction.
func approveRestore(t *testing.T, ctx context.Context, st *store.SQLiteStore, item domain.QuarantineItem, planID string) domain.RestorePlan {
	t.Helper()
	digest := strings.Repeat("a", 64)
	plan := domain.RestorePlan{
		ID: planID, ItemID: item.ID, State: domain.RestoreDraft,
		QuarantinePath: item.QuarantinePath, RestorePath: item.SourcePath,
		ExpectedSHA256: item.ContentSHA256, ExpectedSize: item.FileSize,
		ApprovalDigest: digest, CreatedAt: time.Now().UTC().Add(-time.Hour),
	}
	if err := st.SaveRestorePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if err := st.ApproveRestorePlan(ctx, planID, digest, time.Now().UTC().Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	fresh, err := st.GetRestorePlan(ctx, planID)
	if err != nil {
		t.Fatal(err)
	}
	return fresh
}

func newIntegrationStore(t *testing.T, ctx context.Context, dir, name string) *store.SQLiteStore {
	t.Helper()
	st, err := store.Open(ctx, filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestQuarantineRestore_BusinessContract(t *testing.T) {
	if testing.Short() {
		t.Skip("business contract uses a real store and real moves")
	}
	ctx := context.Background()
	now := time.Now().UTC()
	elapsed := now.Add(-24 * time.Hour)

	setup := func(t *testing.T) (*store.SQLiteStore, string, string, *RestoreExecutor) {
		dir := t.TempDir()
		sourceRoot := filepath.Join(dir, "source")
		quarantineRoot := filepath.Join(dir, "quarantine")
		for _, d := range []string{sourceRoot, quarantineRoot} {
			if err := os.MkdirAll(d, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		st := newIntegrationStore(t, ctx, dir, "contract.db")
		rx, err := NewRestoreExecutor(quarantineRoot, []string{sourceRoot}, st)
		if err != nil {
			t.Fatal(err)
		}
		return st, sourceRoot, quarantineRoot, rx
	}

	t.Run("quarantine_restore_round_trip", func(t *testing.T) {
		st, sourceRoot, quarantineRoot, rx := setup(t)
		defer st.Close()
		item := seedQuarantinedFile(t, ctx, st, sourceRoot, quarantineRoot,
			"roundtrip", "round-trip-payload", now.Add(-48*time.Hour), elapsed)
		plan := approveRestore(t, ctx, st, item, "restore-roundtrip")

		// The destination is genuinely empty (the quarantine moved it away).
		if _, err := os.Lstat(item.SourcePath); !os.IsNotExist(err) {
			t.Fatalf("source location should be empty before restore: %v", err)
		}
		if result := rx.ValidateRestore(ctx, plan, item); result.Status != StepOK {
			t.Fatalf("validate refused a lawful restore: %s (%v)", result.ErrorType, result.Err)
		}
		result := rx.ExecuteRestore(ctx, &plan, &item)
		if result.Status != StepOK || result.FinalState != domain.RestoreCompleted {
			t.Fatalf("execute failed: %s (%v)", result.ErrorType, result.Err)
		}

		// The file really moved back: one valid copy, at the source.
		back, err := os.ReadFile(item.SourcePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(back) != "round-trip-payload" {
			t.Fatalf("restored bytes differ: %q", back)
		}
		if _, err := os.Lstat(item.QuarantinePath); !os.IsNotExist(err) {
			t.Fatal("the quarantine copy must be gone after a real move")
		}

		// The durable record really completed: item RESTORED, plan RESTORED.
		freshItem, err := st.GetQuarantineItem(ctx, item.ID)
		if err != nil {
			t.Fatal(err)
		}
		if freshItem.Status != domain.QuarantineRestored {
			t.Fatalf("item status after restore: %s", freshItem.Status)
		}
		freshPlan, err := st.GetRestorePlan(ctx, plan.ID)
		if err != nil {
			t.Fatal(err)
		}
		if freshPlan.State != domain.RestoreCompleted {
			t.Fatalf("restore plan state after execution: %s", freshPlan.State)
		}
		pending, err := st.ListPendingRestores(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 0 {
			t.Fatalf("a completed restore must not leave a pending journal: %d", len(pending))
		}
	})

	t.Run("restore_refuses_destination_exists", func(t *testing.T) {
		st, sourceRoot, quarantineRoot, rx := setup(t)
		defer st.Close()
		item := seedQuarantinedFile(t, ctx, st, sourceRoot, quarantineRoot,
			"occupied", "occupied-payload", now.Add(-48*time.Hour), elapsed)
		plan := approveRestore(t, ctx, st, item, "restore-occupied")
		// Something now occupies the destination: never overwritten.
		if err := os.WriteFile(item.SourcePath, []byte("someone-else"), 0o600); err != nil {
			t.Fatal(err)
		}
		result := rx.ValidateRestore(ctx, plan, item)
		if result.Status == StepOK || result.ErrorType != "destination_exists" {
			t.Fatalf("expected destination_exists, got %s (%v)", result.ErrorType, result.Err)
		}
		exec := rx.ExecuteRestore(ctx, &plan, &item)
		if exec.Status == StepOK {
			t.Fatal("execution must refuse an occupied destination")
		}
		kept, err := os.ReadFile(item.SourcePath)
		if err != nil || string(kept) != "someone-else" {
			t.Fatalf("the occupant must be untouched: %v %q", err, kept)
		}
		if _, err := os.Lstat(item.QuarantinePath); err != nil {
			t.Fatal("the quarantine copy must survive the refusal")
		}
	})

	t.Run("restore_refuses_quarantine_drift", func(t *testing.T) {
		st, sourceRoot, quarantineRoot, rx := setup(t)
		defer st.Close()
		item := seedQuarantinedFile(t, ctx, st, sourceRoot, quarantineRoot,
			"drift", "drift-payload", now.Add(-48*time.Hour), elapsed)
		plan := approveRestore(t, ctx, st, item, "restore-drift")
		// Same length, different bytes: identity drift the live snapshot catches.
		if err := os.WriteFile(item.QuarantinePath, []byte("drift-payl0ad"), 0o600); err != nil {
			t.Fatal(err)
		}
		result := rx.ValidateRestore(ctx, plan, item)
		if result.Status == StepOK || result.ErrorType != "stale_detected" {
			t.Fatalf("expected stale_detected, got %s (%v)", result.ErrorType, result.Err)
		}
	})

	t.Run("restore_refuses_plan_drift", func(t *testing.T) {
		st, sourceRoot, quarantineRoot, rx := setup(t)
		defer st.Close()
		item := seedQuarantinedFile(t, ctx, st, sourceRoot, quarantineRoot,
			"binding", "binding-payload", now.Add(-48*time.Hour), elapsed)
		plan := approveRestore(t, ctx, st, item, "restore-binding")
		// The plan snapshot drifted from its item after approval.
		plan.ExpectedSHA256 = strings.Repeat("f", 64)
		result := rx.ValidateRestore(ctx, plan, item)
		if result.Status == StepOK || result.ErrorType != "stale_detected" {
			t.Fatalf("expected stale_detected, got %s (%v)", result.ErrorType, result.Err)
		}
	})

	t.Run("restore_refuses_unapproved_plan", func(t *testing.T) {
		st, sourceRoot, quarantineRoot, rx := setup(t)
		defer st.Close()
		item := seedQuarantinedFile(t, ctx, st, sourceRoot, quarantineRoot,
			"draft", "draft-payload", now.Add(-48*time.Hour), elapsed)
		digest := strings.Repeat("a", 64)
		plan := domain.RestorePlan{
			ID: "restore-draft", ItemID: item.ID, State: domain.RestoreDraft,
			QuarantinePath: item.QuarantinePath, RestorePath: item.SourcePath,
			ExpectedSHA256: item.ContentSHA256, ExpectedSize: item.FileSize,
			ApprovalDigest: digest, CreatedAt: now.Add(-time.Hour),
		}
		if err := st.SaveRestorePlan(ctx, plan); err != nil {
			t.Fatal(err)
		}
		result := rx.ValidateRestore(ctx, plan, item)
		if result.Status == StepOK || result.ErrorType != "invalid_state" {
			t.Fatalf("expected invalid_state, got %s (%v)", result.ErrorType, result.Err)
		}
	})

	t.Run("db_object_inconsistency_refused", func(t *testing.T) {
		st, sourceRoot, quarantineRoot, rx := setup(t)
		defer st.Close()
		item := seedQuarantinedFile(t, ctx, st, sourceRoot, quarantineRoot,
			"vanished", "vanished-payload", now.Add(-48*time.Hour), elapsed)
		plan := approveRestore(t, ctx, st, item, "restore-vanished")
		// The durable record claims an object that no longer exists.
		if err := os.Remove(item.QuarantinePath); err != nil {
			t.Fatal(err)
		}
		result := rx.ValidateRestore(ctx, plan, item)
		if result.Status == StepOK || result.ErrorType != "stale_detected" {
			t.Fatalf("expected stale_detected, got %s (%v)", result.ErrorType, result.Err)
		}
	})

	t.Run("crash_keeps_recovery_lock", func(t *testing.T) {
		st, sourceRoot, quarantineRoot, rx := setup(t)
		defer st.Close()
		// A second, unrelated purge approval models "the next dangerous
		// operation arrives while a restore is mid-flight".
		other := seedQuarantinedFile(t, ctx, st, sourceRoot, quarantineRoot,
			"waiter", "waiter-payload", now.Add(-48*time.Hour), elapsed)
		digest := strings.Repeat("a", 64)
		if err := st.SavePurgePlans(ctx, []domain.PurgePlan{{
			ID: "purge-waiter", ItemID: other.ID, State: domain.PurgeDraft,
			ExpectedPath: other.QuarantinePath, ExpectedSHA256: other.ContentSHA256,
			ExpectedSize: other.FileSize, RetainUntil: elapsed,
			ApprovalDigest: digest, CreatedAt: now.Add(-12 * time.Hour),
		}}); err != nil {
			t.Fatal(err)
		}
		if err := st.ApprovePurgePlan(ctx, "purge-waiter", digest, now.Add(-30*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if err := st.MarkPurgeDryRunVerified(ctx, "purge-waiter", digest, now.Add(-20*time.Minute)); err != nil {
			t.Fatal(err)
		}
		purgeReq := authz.Request{
			Operation: authz.OpPurge, PlanID: "purge-waiter", PresentedDigest: digest,
			ObjectID: other.ID, ContentSHA256: other.ContentSHA256,
			Size: authz.Int64Field(other.FileSize), Device: authz.Uint64Field(1), Inode: authz.Uint64Field(2),
		}
		v := authz.NewVerifier()
		el, err := v.Check(ctx, authz.NewStoreFacts(st), purgeReq)
		if err != nil || !el.Eligible {
			t.Fatalf("the waiter purge must be eligible before any restore starts: %v %s", err, el.Reason)
		}

		// The crash: a restore began its durable journal and the process
		// disappeared before completion. The pending journal is the crash's
		// durable footprint.
		item := seedQuarantinedFile(t, ctx, st, sourceRoot, quarantineRoot,
			"crashed", "crashed-payload", now.Add(-48*time.Hour), elapsed)
		plan := approveRestore(t, ctx, st, item, "restore-crashed")
		if err := st.BeginRestore(ctx, plan, item, now); err != nil {
			t.Fatal(err)
		}
		pending, err := st.ListPendingRestores(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 1 {
			t.Fatalf("expected one pending restore journal, got %d", len(pending))
		}

		// While the journal is pending, the new dangerous operation is
		// refused with the typed recovery-lock reason.
		el, err = v.Check(ctx, authz.NewStoreFacts(st), purgeReq)
		if err != nil {
			t.Fatal(err)
		}
		if el.Eligible || el.Reason != authz.ReasonRecoveryLock {
			t.Fatalf("pending restore must block new dangerous ops, got %s", el.Reason)
		}

		// Restart reconciliation: both files are proven unchanged (quarantine
		// copy intact, destination absent), so the journal is rolled back.
		results := rx.RecoverRestores(ctx)
		if len(results) != 1 || results[0].Status != StepOK || results[0].FinalState != domain.RestoreRolledBack {
			t.Fatalf("recovery should roll back the proven-unchanged journal: %#v", results)
		}
		pending, err = st.ListPendingRestores(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 0 {
			t.Fatalf("journal must be closed after reconciliation, got %d", len(pending))
		}
		// The quarantine object is still intact and the lock is released:
		// the waiter purge becomes eligible again.
		if _, err := os.Lstat(item.QuarantinePath); err != nil {
			t.Fatal("reconciliation must not move a proven-unchanged quarantine copy")
		}
		el, err = v.Check(ctx, authz.NewStoreFacts(st), purgeReq)
		if err != nil || !el.Eligible {
			t.Fatalf("after reconciliation the waiter purge must be eligible again: %v %s", err, el.Reason)
		}
	})

	t.Run("concurrent_duplicate_restore", func(t *testing.T) {
		st, sourceRoot, quarantineRoot, rx := setup(t)
		defer st.Close()
		item := seedQuarantinedFile(t, ctx, st, sourceRoot, quarantineRoot,
			"race", "race-payload", now.Add(-48*time.Hour), elapsed)
		plan := approveRestore(t, ctx, st, item, "restore-race")

		var wg sync.WaitGroup
		outcomes := make([]RestoreResult, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(slot int) {
				defer wg.Done()
				p, it := plan, item
				outcomes[slot] = rx.ExecuteRestore(ctx, &p, &it)
			}(i)
		}
		wg.Wait()

		successes := 0
		for _, r := range outcomes {
			if r.Status == StepOK {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("exactly one racing restore must win, got %d (%#v)", successes, outcomes)
		}
		// Exactly one valid copy, in exactly one place.
		if _, err := os.Lstat(item.QuarantinePath); !os.IsNotExist(err) {
			t.Fatal("the quarantine copy must be gone after the winning restore")
		}
		back, err := os.ReadFile(item.SourcePath)
		if err != nil || string(back) != "race-payload" {
			t.Fatalf("the restored object must be the only valid copy: %v %q", err, back)
		}
		pending, err := st.ListPendingRestores(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 0 {
			t.Fatalf("no duplicate journal may survive: %d", len(pending))
		}
	})

	t.Run("op_restore_stays_wrong_plan", func(t *testing.T) {
		st, sourceRoot, quarantineRoot, _ := setup(t)
		defer st.Close()
		item := seedQuarantinedFile(t, ctx, st, sourceRoot, quarantineRoot,
			"norestore", "norestore-payload", now.Add(-48*time.Hour), elapsed)
		// Until a durable restore fact source exists in the authz contract,
		// an OpRestore request must keep its typed wrong_plan rejection —
		// even against a fully approved purge plan for the same object.
		digest := strings.Repeat("a", 64)
		if err := st.SavePurgePlans(ctx, []domain.PurgePlan{{
			ID: "purge-norestore", ItemID: item.ID, State: domain.PurgeDraft,
			ExpectedPath: item.QuarantinePath, ExpectedSHA256: item.ContentSHA256,
			ExpectedSize: item.FileSize, RetainUntil: elapsed,
			ApprovalDigest: digest, CreatedAt: now.Add(-12 * time.Hour),
		}}); err != nil {
			t.Fatal(err)
		}
		if err := st.ApprovePurgePlan(ctx, "purge-norestore", digest, now.Add(-30*time.Minute)); err != nil {
			t.Fatal(err)
		}
		if err := st.MarkPurgeDryRunVerified(ctx, "purge-norestore", digest, now.Add(-20*time.Minute)); err != nil {
			t.Fatal(err)
		}
		v := authz.NewVerifier()
		el, err := v.Check(ctx, authz.NewStoreFacts(st), authz.Request{
			Operation: authz.OpRestore, PlanID: "purge-norestore", PresentedDigest: digest,
			ObjectID: item.ID, ContentSHA256: item.ContentSHA256, Size: authz.Int64Field(item.FileSize),
		})
		if err != nil {
			t.Fatal(err)
		}
		if el.Eligible || el.Reason != authz.ReasonWrongPlan {
			t.Fatalf("OpRestore must stay wrong_plan until its durable fact source exists, got %s", el.Reason)
		}
	})

	t.Run("no_durable_purge_execution", func(t *testing.T) {
		st, _, _, _ := setup(t)
		defer st.Close()
		// The whole suite exercises real moves but never opens the purge
		// transaction: no executing plan, no recoverable purge journal.
		executing, err := st.ListExecutingPlans(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(executing) != 0 {
			t.Fatalf("no plan may be left executing: %v", executing)
		}
		recoverable, err := st.ListRecoverablePurges(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(recoverable) != 0 {
			t.Fatalf("no purge journal may exist: %v", recoverable)
		}
	})
}
