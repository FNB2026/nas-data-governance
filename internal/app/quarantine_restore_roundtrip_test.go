package app

// The real Quarantine → Restore end-to-end round trip, driven through the
// product service layer (ExecutionService for the quarantine move,
// QuarantineService for the restore), on real synthetic files under a real
// executor: the file is moved INTO quarantine by the executor pipeline
// (stale check → journal → move → verify → durable registration), never
// hand-placed. Content and SHA-256 are verified at every hop.
//
// Boundary statement encoded by these tests: the FILE execution here is the
// existing Executor (the only component that writes the filesystem); the
// NEW Worker's role remains authorization (Verdict-only). Nothing in this
// file reports Worker-executed quarantine.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/authz"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func sha256File(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func sha256Bytes(t *testing.T, b []byte) string {
	t.Helper()
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// fileInstanceFromSnapshot builds the plan identity exactly the way the
// stale check will re-prove it at execution time: size, mtime, device,
// inode, and content hash must all match, so the plan must carry all of
// them from the real snapshot.
func fileInstanceFromSnapshot(path string, snap executor.FileSnapshot) domain.FileInstance {
	return domain.FileInstance{
		Path:          path,
		Size:          snap.Size,
		ModifiedAt:    snap.ModifiedAt,
		Device:        snap.Device,
		Inode:         snap.Inode,
		ContentSHA256: snap.Hash,
	}
}

// roundtripEnv builds one isolated environment with real directories and a
// real store.
type roundtripEnv struct {
	dir            string
	sourceRoot     string
	quarantineRoot string
	st             *store.SQLiteStore
	exec           *ExecutionService
	qsvc           *QuarantineService
}

func newRoundtripEnv(t *testing.T) *roundtripEnv {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	env := &roundtripEnv{
		dir:            dir,
		sourceRoot:     filepath.Join(dir, "source"),
		quarantineRoot: filepath.Join(dir, "quarantine"),
	}
	for _, d := range []string{env.sourceRoot, env.quarantineRoot} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(ctx, filepath.Join(dir, "roundtrip.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	env.st = st
	env.exec = NewExecutionService(st)
	env.qsvc = NewQuarantineService(st)
	return env
}

// quarantineRealFile runs the product path for one real source file: plan
// from the real snapshot, durable draft, durable approval, then the
// ExecutionService pipeline that really moves the file into quarantine and
// registers the lifecycle item. Returns the item and the original hash.
func (env *roundtripEnv) quarantineRealFile(t *testing.T, ctx context.Context, name, content string, at time.Time) (domain.QuarantineItem, string) {
	t.Helper()
	sourcePath := filepath.Join(env.sourceRoot, name)
	if err := os.WriteFile(sourcePath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	// The plan identity comes from a real snapshot of the real file — the
	// same identity the scanner would bind.
	snapshot, err := executor.Snapshot(sourcePath, true)
	if err != nil {
		t.Fatal(err)
	}
	originalHash := snapshot.Hash
	taskID := "task-rt-" + name
	planID := "plan-rt-" + name
	plan := domain.OperationPlan{
		ID: planID, TaskID: taskID, State: domain.PlanDraft,
		Actions: []domain.PlannedAction{{
			Path:    sourcePath,
			Action:  domain.OperationQuarantine,
			Context: domain.DirectoryContext{Role: domain.RoleTemporary},
			File:    fileInstanceFromSnapshot(sourcePath, snapshot),
		}},
	}
	if err := env.st.CreateTask(ctx, domain.OperationTask{ID: taskID, RootPath: env.sourceRoot, State: "executing", CreatedAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := env.st.SavePlans(ctx, taskID, []domain.OperationPlan{plan}); err != nil {
		t.Fatal(err)
	}
	// Durable plan approval: the desktop governance path persists the
	// approval as a state transition; execution refuses non-APPROVED plans.
	if err := env.st.CompareAndSwapPlanState(ctx, planID, domain.PlanDraft, domain.PlanApproved); err != nil {
		t.Fatal(err)
	}

	summary, err := env.exec.Execute(ctx, ExecutionInput{
		Plans:          []domain.OperationPlan{plan},
		QuarantineRoot: env.quarantineRoot,
		SourceRoots:    []string{env.sourceRoot},
		DryRun:         false,
		Retention:      25 * time.Hour,
	})
	if err != nil {
		t.Fatalf("real quarantine execution failed: %v", err)
	}
	if summary.Executed != 1 || summary.LifecycleErr != nil {
		t.Fatalf("quarantine did not complete: executed=%d lifecycleErr=%v results=%#v",
			summary.Executed, summary.LifecycleErr, summary.Results)
	}

	// The file really moved: the source location is empty, the quarantine
	// copy exists and hashes exactly like the original.
	if _, err := os.Lstat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("source location must be empty after a real move: %v", err)
	}
	items, err := env.st.ListQuarantineItems(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.SourcePath == sourcePath {
			if got := sha256File(t, it.QuarantinePath); got != originalHash {
				t.Fatalf("quarantined bytes differ from the original: %s vs %s", got, originalHash)
			}
			if it.ContentSHA256 != originalHash {
				t.Fatalf("durable item hash differs from the original: %s vs %s", it.ContentSHA256, originalHash)
			}
			return it, originalHash
		}
	}
	t.Fatalf("registered quarantine item not found for %s", sourcePath)
	return domain.QuarantineItem{}, ""
}

func TestQuarantineRestoreRoundTrip_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("round trip uses a real store and real moves")
	}
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("real_file_quarantine_then_restore", func(t *testing.T) {
		env := newRoundtripEnv(t)
		payload := "the-real-source-payload"
		item, originalHash := env.quarantineRealFile(t, ctx, "important.txt", payload, now.Add(-time.Hour))

		// The durable item binds the original destination.
		if item.RestoredAt != nil || item.Status != domain.QuarantineActive {
			t.Fatalf("fresh item state: %s", item.Status)
		}
		if item.SourcePath != filepath.Join(env.sourceRoot, "important.txt") {
			t.Fatalf("item lost the original destination: %s", item.SourcePath)
		}

		// Independent restore plan through the product planner, then approval.
		plan, err := env.qsvc.CreateRestorePlan(ctx, item.ID)
		if err != nil {
			t.Fatal(err)
		}
		if plan.State != domain.RestoreDraft {
			t.Fatalf("restore plan state: %s", plan.State)
		}
		if err := env.qsvc.ApproveRestorePlan(ctx, plan.ID, plan.ApprovalDigest); err != nil {
			t.Fatal(err)
		}

		// The authz contract sees a legitimately authorized restore (its own
		// fact source, no purge retention precondition).
		el, err := authz.NewVerifier().Check(ctx, authz.NewStoreFacts(env.st), authz.Request{
			Operation: authz.OpRestore, PlanID: plan.ID, PresentedDigest: plan.ApprovalDigest,
			ObjectID: item.ID, ContentSHA256: item.ContentSHA256, Size: authz.Int64Field(item.FileSize),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !el.Eligible {
			t.Fatalf("the approved restore must be eligible, got %s (%s)", el.Reason, el.Detail)
		}

		// Real restore through the product service (single-writer lock,
		// recovery check, executor move).
		res, err := env.qsvc.ExecuteRestore(ctx, RestoreExecuteInput{
			PlanID: plan.ID, Digest: plan.ApprovalDigest,
			QuarantineRoot: env.quarantineRoot, SourceRoots: []string{env.sourceRoot},
			DryRun: false,
		})
		if err != nil {
			t.Fatalf("real restore failed: %v (%s)", err, res)
		}
		if res.Result.Status != executor.StepOK || res.Result.FinalState != domain.RestoreCompleted {
			t.Fatalf("restore result: %#v", res.Result)
		}

		// The file is back, byte for byte, and the quarantine copy is gone.
		restoredPath := filepath.Join(env.sourceRoot, "important.txt")
		if got := sha256File(t, restoredPath); got != originalHash {
			t.Fatalf("restored bytes differ: %s vs %s", got, originalHash)
		}
		if _, err := os.Lstat(item.QuarantinePath); !os.IsNotExist(err) {
			t.Fatal("the quarantine copy must be gone after the real restore")
		}
		// Durable completion.
		freshItem, err := env.st.GetQuarantineItem(ctx, item.ID)
		if err != nil {
			t.Fatal(err)
		}
		if freshItem.Status != domain.QuarantineRestored {
			t.Fatalf("item after restore: %s", freshItem.Status)
		}
		freshPlan, err := env.st.GetRestorePlan(ctx, plan.ID)
		if err != nil {
			t.Fatal(err)
		}
		if freshPlan.State != domain.RestoreCompleted {
			t.Fatalf("restore plan after execution: %s", freshPlan.State)
		}
	})

	t.Run("purge_eligible_item_is_still_restorable", func(t *testing.T) {
		// A purge plan promotes the item to PURGE_ELIGIBLE; restoring it back
		// remains the reversible direction and must work.
		env := newRoundtripEnv(t)
		item, originalHash := env.quarantineRealFile(t, ctx, "promoted.txt", "promoted-payload", now.Add(-time.Hour))
		retainUntil := item.RetainUntil
		// The store promotes an item to PURGE_ELIGIBLE only when the purge
		// plan is created at or after the item's retention deadline — the
		// retention rule itself. Plan creation time therefore sits at the
		// deadline.
		if err := env.st.SavePurgePlans(ctx, []domain.PurgePlan{{
			ID: "purge-promoted", ItemID: item.ID, State: domain.PurgeDraft,
			ExpectedPath: item.QuarantinePath, ExpectedSHA256: item.ContentSHA256,
			ExpectedSize: item.FileSize, RetainUntil: retainUntil,
			ApprovalDigest: strings.Repeat("a", 64), CreatedAt: retainUntil,
		}}); err != nil {
			t.Fatal(err)
		}
		plan, err := env.qsvc.CreateRestorePlan(ctx, item.ID)
		if err != nil {
			t.Fatalf("a PURGE_ELIGIBLE item must be restorable: %v", err)
		}
		if err := env.qsvc.ApproveRestorePlan(ctx, plan.ID, plan.ApprovalDigest); err != nil {
			t.Fatal(err)
		}
		res, err := env.qsvc.ExecuteRestore(ctx, RestoreExecuteInput{
			PlanID: plan.ID, Digest: plan.ApprovalDigest,
			QuarantineRoot: env.quarantineRoot, SourceRoots: []string{env.sourceRoot},
			DryRun: false,
		})
		if err != nil {
			t.Fatalf("restore of a purge-eligible item failed: %v", err)
		}
		if got := sha256File(t, filepath.Join(env.sourceRoot, "promoted.txt")); got != originalHash {
			t.Fatalf("restored bytes differ: %s vs %s", got, originalHash)
		}
		_ = res
	})

	t.Run("duplicate_concurrent_restore_single_winner", func(t *testing.T) {
		env := newRoundtripEnv(t)
		item, _ := env.quarantineRealFile(t, ctx, "raced.txt", "raced-payload", now.Add(-time.Hour))
		plan, err := env.qsvc.CreateRestorePlan(ctx, item.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := env.qsvc.ApproveRestorePlan(ctx, plan.ID, plan.ApprovalDigest); err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(slot int) {
				defer wg.Done()
				_, errs[slot] = env.qsvc.ExecuteRestore(ctx, RestoreExecuteInput{
					PlanID: plan.ID, Digest: plan.ApprovalDigest,
					QuarantineRoot: env.quarantineRoot, SourceRoots: []string{env.sourceRoot},
					DryRun: false,
				})
			}(i)
		}
		wg.Wait()
		successes := 0
		for _, err := range errs {
			if err == nil {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("exactly one racing restore must win, got %d (%v)", successes, errs)
		}
		// Exactly one copy in exactly one place.
		if _, err := os.Lstat(item.QuarantinePath); !os.IsNotExist(err) {
			t.Fatal("quarantine copy must be gone")
		}
		if got := sha256File(t, filepath.Join(env.sourceRoot, "raced.txt")); got != sha256Bytes(t, []byte("raced-payload")) {
			t.Fatal("restored bytes differ")
		}
	})

	t.Run("destination_occupied_is_never_overwritten", func(t *testing.T) {
		env := newRoundtripEnv(t)
		item, _ := env.quarantineRealFile(t, ctx, "occupied.txt", "occupied-payload", now.Add(-time.Hour))
		plan, err := env.qsvc.CreateRestorePlan(ctx, item.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := env.qsvc.ApproveRestorePlan(ctx, plan.ID, plan.ApprovalDigest); err != nil {
			t.Fatal(err)
		}
		occupant := "someone-elses-bytes"
		if err := os.WriteFile(item.SourcePath, []byte(occupant), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := env.qsvc.ExecuteRestore(ctx, RestoreExecuteInput{
			PlanID: plan.ID, Digest: plan.ApprovalDigest,
			QuarantineRoot: env.quarantineRoot, SourceRoots: []string{env.sourceRoot},
			DryRun: false,
		}); err == nil {
			t.Fatal("restore must refuse an occupied destination")
		}
		if got, _ := os.ReadFile(item.SourcePath); string(got) != occupant {
			t.Fatalf("the occupant must be untouched, got %q", got)
		}
		if _, err := os.Lstat(item.QuarantinePath); err != nil {
			t.Fatal("the quarantine copy must survive the refusal")
		}
	})

	t.Run("interrupted_restore_keeps_recovery_lock_until_reconciled", func(t *testing.T) {
		env := newRoundtripEnv(t)
		item, _ := env.quarantineRealFile(t, ctx, "crashed.txt", "crashed-payload", now.Add(-time.Hour))
		plan, err := env.qsvc.CreateRestorePlan(ctx, item.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := env.qsvc.ApproveRestorePlan(ctx, plan.ID, plan.ApprovalDigest); err != nil {
			t.Fatal(err)
		}
		// The waiter is quarantined BEFORE the crash footprint is planted:
		// a pending restore journal blocks every new execution by design
		// (the single-writer recovery lock spans the whole domain).
		item2, _ := env.quarantineRealFile(t, ctx, "waiter.txt", "waiter-payload", now.Add(-time.Hour))
		plan2, err := env.qsvc.CreateRestorePlan(ctx, item2.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := env.qsvc.ApproveRestorePlan(ctx, plan2.ID, plan2.ApprovalDigest); err != nil {
			t.Fatal(err)
		}
		// The durable footprint of a mid-flight crash: journal pending, the
		// process gone before completion.
		freshPlan, err := env.st.GetRestorePlan(ctx, plan.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := env.st.BeginRestore(ctx, freshPlan, item, now); err != nil {
			t.Fatal(err)
		}
		if _, err := env.qsvc.ExecuteRestore(ctx, RestoreExecuteInput{
			PlanID: plan2.ID, Digest: plan2.ApprovalDigest,
			QuarantineRoot: env.quarantineRoot, SourceRoots: []string{env.sourceRoot},
			DryRun: false,
		}); err == nil {
			t.Fatal("a pending restore journal must block new restores")
		}
		// And the file system is untouched for the waiter.
		if _, err := os.Lstat(item2.QuarantinePath); err != nil {
			t.Fatal("the blocked waiter's quarantine copy must be intact")
		}

		// Restart reconciliation: both files are proven unchanged, so the
		// crashed journal is rolled back and the lock releases.
		results, err := env.qsvc.RecoverRestores(ctx, RecoverRestoresInput{
			QuarantineRoot: env.quarantineRoot, SourceRoots: []string{env.sourceRoot},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != 1 || results[0].Status != executor.StepOK || results[0].FinalState != domain.RestoreRolledBack {
			t.Fatalf("reconciliation must roll back the proven-unchanged journal: %#v", results)
		}
		if _, err := os.Lstat(item.QuarantinePath); err != nil {
			t.Fatal("the interrupted object must still sit in quarantine, untouched")
		}

		// After reconciliation the waiter's restore goes through for real.
		if _, err := env.qsvc.ExecuteRestore(ctx, RestoreExecuteInput{
			PlanID: plan2.ID, Digest: plan2.ApprovalDigest,
			QuarantineRoot: env.quarantineRoot, SourceRoots: []string{env.sourceRoot},
			DryRun: false,
		}); err != nil {
			t.Fatalf("restore after reconciliation must succeed: %v", err)
		}
		if _, err := os.Lstat(item2.QuarantinePath); !os.IsNotExist(err) {
			t.Fatal("the waiter's quarantine copy must be gone after its real restore")
		}
		if got := sha256File(t, filepath.Join(env.sourceRoot, "waiter.txt")); got != sha256Bytes(t, []byte("waiter-payload")) {
			t.Fatal("the waiter's restored bytes differ")
		}
	})

	t.Run("quarantine_rollback_on_midflight_failure", func(t *testing.T) {
		// A plan with two quarantine actions: the second is made unsafe by a
		// planted symlink at its nominal quarantine destination, so the
		// executor fails it mid-flight and rolls the first move back — the
		// source file must end up back where it was.
		env := newRoundtripEnv(t)
		ctx := context.Background()
		aPath := filepath.Join(env.sourceRoot, "rollback-a.txt")
		bPath := filepath.Join(env.sourceRoot, "rollback-b.txt")
		payloadA := "rollback-payload-a"
		payloadB := "rollback-payload-b"
		for _, p := range []struct{ path, content string }{{aPath, payloadA}, {bPath, payloadB}} {
			if err := os.WriteFile(p.path, []byte(p.content), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		// Plant an unsafe symlink at B's flat quarantine destination.
		if err := os.Symlink("/etc/hostname", filepath.Join(env.quarantineRoot, "rollback-b.txt")); err != nil {
			t.Fatal(err)
		}
		var actions []domain.PlannedAction
		for _, p := range []struct{ path, content string }{{aPath, payloadA}, {bPath, payloadB}} {
			snapshot, err := executor.Snapshot(p.path, true)
			if err != nil {
				t.Fatal(err)
			}
			actions = append(actions, domain.PlannedAction{
				Path:    p.path,
				Action:  domain.OperationQuarantine,
				Context: domain.DirectoryContext{Role: domain.RoleTemporary},
				File:    fileInstanceFromSnapshot(p.path, snapshot),
			})
		}
		taskID := "task-rt-rollback"
		planID := "plan-rt-rollback"
		plan := domain.OperationPlan{ID: planID, TaskID: taskID, State: domain.PlanDraft, Actions: actions}
		if err := env.st.CreateTask(ctx, domain.OperationTask{ID: taskID, RootPath: env.sourceRoot, State: "executing", CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if err := env.st.SavePlans(ctx, taskID, []domain.OperationPlan{plan}); err != nil {
			t.Fatal(err)
		}
		if err := env.st.CompareAndSwapPlanState(ctx, planID, domain.PlanDraft, domain.PlanApproved); err != nil {
			t.Fatal(err)
		}
		summary, err := env.exec.Execute(ctx, ExecutionInput{
			Plans:          []domain.OperationPlan{plan},
			QuarantineRoot: env.quarantineRoot,
			SourceRoots:    []string{env.sourceRoot},
			DryRun:         false,
			Retention:      25 * time.Hour,
		})
		if err != nil {
			t.Fatal(err)
		}
		if summary.Executed != 0 {
			t.Fatalf("a failed plan must not count as executed: %#v", summary)
		}
		if summary.LifecycleErr != nil {
			t.Fatalf("a fully rolled-back plan must not demand recovery: %v", summary.LifecycleErr)
		}
		// The rollback is durably terminal: the plan reaches ROLLED_BACK in
		// the store, so no recovery lock stays engaged on this plan.
		durablePlan, err := env.st.GetPlan(ctx, planID)
		if err != nil {
			t.Fatal(err)
		}
		if durablePlan.State != domain.PlanRolledBack {
			t.Fatalf("durable plan after mid-flight rollback: %s", durablePlan.State)
		}
		// Both source files are back where they were, byte for byte.
		if got := sha256File(t, aPath); got != sha256Bytes(t, []byte(payloadA)) {
			t.Fatal("action A must have been rolled back to its source")
		}
		if got := sha256File(t, bPath); got != sha256Bytes(t, []byte(payloadB)) {
			t.Fatal("action B must never have moved")
		}
		// The quarantine area holds no moved copy of A.
		if _, err := os.Lstat(filepath.Join(env.quarantineRoot, "rollback-a.txt")); !os.IsNotExist(err) {
			t.Fatal("the rolled-back quarantine copy must be gone")
		}
	})
}
