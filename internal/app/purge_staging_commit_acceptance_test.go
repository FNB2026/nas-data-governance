package app

// Purge staging → commit → reconciliation acceptance on REAL disposable
// files, behind the compile-time release gate (PurgeReleaseAllowed stays
// false; these tests drive the private service path directly, exactly so
// that no public channel can). The scenarios are the R4 Task E mandate:
// approval + dry-run, retention, valid-copy protection, snapshot/mount
// identity, pre-commit revocation, crash-before-commit (SIGKILL footprint),
// recovery lock + audit, and the wrong file is never deleted.
//
// No external delete API exists or is called anywhere here: the only
// irreversible step is the executor's SafeRemove of the staged object
// inside the quarantine root itself.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/purge"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// purgeSeed registers one real disposable file as a purge-eligible item
// whose retention has already elapsed, and returns the built DRAFT plan.
type purgeSeed struct {
	t              *testing.T
	st             *store.SQLiteStore
	item           domain.QuarantineItem
	plan           domain.PurgePlan
	digest         string
	quarantineRoot string
}

func seedPurgeItem(t *testing.T, ctx context.Context, st *store.SQLiteStore, quarantineRoot, name, content string, at time.Time) purgeSeed {
	t.Helper()
	objectPath := filepath.Join(quarantineRoot, name)
	if err := os.WriteFile(objectPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	contentHash := hex.EncodeToString(sum[:])
	size := int64(len(content))
	retainUntil := at.Add(-24 * time.Hour) // retention elapsed
	taskID := "purge-task-" + name
	plan := domain.OperationPlan{
		ID: "purge-exec-" + name, TaskID: taskID, State: domain.PlanExecuting,
		Actions: []domain.PlannedAction{{
			Path:    "/unused/" + name,
			Action:  domain.OperationQuarantine,
			Context: domain.DirectoryContext{Role: domain.RoleTemporary},
			File:    domain.FileInstance{Path: "/unused/" + name, Size: size, ContentSHA256: contentHash},
		}},
	}
	if err := st.CreateTask(ctx, domain.OperationTask{ID: taskID, RootPath: "/unused", State: "completed", CreatedAt: at.Add(-48 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := st.SavePlans(ctx, taskID, []domain.OperationPlan{plan}); err != nil {
		t.Fatal(err)
	}
	if err := st.BeginJournal(ctx, taskID, plan.ID, plan.Actions); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkJournalDone(ctx, plan.ID, 0, objectPath); err != nil {
		t.Fatal(err)
	}
	if err := st.CompletePlanExecution(ctx, plan, domain.PlanVerified, nil, at.Add(-48*time.Hour), retainUntil); err != nil {
		t.Fatal(err)
	}
	items, err := st.ListQuarantineItems(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	seed := purgeSeed{t: t, st: st, quarantineRoot: quarantineRoot}
	for _, it := range items {
		if it.QuarantinePath == objectPath {
			seed.item = it
		}
	}
	if seed.item.ID == "" {
		t.Fatal("seed: quarantine item not registered")
	}
	// The plan comes from the real planner at the retention deadline: its
	// deterministic id and approval digest are the product identity.
	plans := purge.BuildPlans([]domain.QuarantineItem{seed.item}, seed.item.RetainUntil)
	if len(plans) != 1 {
		t.Fatalf("planner must propose exactly one plan, got %d", len(plans))
	}
	seed.plan = plans[0]
	seed.digest = seed.plan.ApprovalDigest
	return seed
}

// stagingPath mirrors the executor's fixed staging layout.
func (e purgeSeed) stagingPath() string {
	return filepath.Join(e.quarantineRoot, ".purge-staging", e.plan.ID+".stage")
}

func TestPurgeStagingCommitAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("purge acceptance uses real files and a real store")
	}
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("full_chain_dry_run_gate_then_commit", func(t *testing.T) {
		dir := t.TempDir()
		st, err := store.Open(ctx, filepath.Join(dir, "purge.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close() //nolint:errcheck
		env := seedPurgeItem(t, ctx, st, dir, "doomed.txt", "doomed-payload", now)
		svc := NewPurgeService(st)
		gate := NewAuthorizationGate(st)
		if err := st.SavePurgePlans(ctx, []domain.PurgePlan{env.plan}); err != nil {
			t.Fatalf("promotion with elapsed retention must succeed: %v", err)
		}
		if err := svc.approvePlan(ctx, env.plan.ID, env.digest); err != nil {
			t.Fatal(err)
		}
		// Dry run validates and persists the exact-digest verification gate.
		dry, err := svc.executePurge(ctx, PurgeExecuteInput{PlanID: env.plan.ID, Digest: env.digest,
			QuarantineRoot: env.quarantineRoot, DryRun: true})
		if err != nil {
			t.Fatalf("dry run failed: %v", dry)
		}
		// The business authorization needs both durable records (approval +
		// dry-run) plus the caller's live identity observation, and must
		// come back eligible.
		live, err := executor.Snapshot(env.item.QuarantinePath, false)
		if err != nil {
			t.Fatal(err)
		}
		el, err := gate.AuthorizePurge(ctx, env.plan.ID, env.digest, live.Device, live.Inode)
		if err != nil || !el.Eligible {
			t.Fatalf("authorized purge must be eligible: %v (%s)", err, el.Reason)
		}
		// Real execution with the exact confirmation text.
		res, err := svc.executePurge(ctx, PurgeExecuteInput{PlanID: env.plan.ID, Digest: env.digest,
			QuarantineRoot: env.quarantineRoot, DryRun: false,
			Confirmation: PurgeConfirmationText(env.plan)})
		if err != nil {
			t.Fatalf("real purge failed: %v (%s)", err, res)
		}
		if res.Result.FinalState != domain.PurgeCommitted {
			t.Fatalf("purge final state: %s", res.Result.FinalState)
		}
		if _, err := os.Lstat(env.item.QuarantinePath); !os.IsNotExist(err) {
			t.Fatal("the quarantine copy must be gone after the real commit")
		}
		if _, err := os.Lstat(env.stagingPath()); !os.IsNotExist(err) {
			t.Fatal("the staging copy must be gone after the real commit")
		}
		fresh, err := st.GetQuarantineItem(ctx, env.item.ID)
		if err != nil {
			t.Fatal(err)
		}
		if fresh.Status != domain.QuarantinePurged || fresh.PurgedAt == nil {
			t.Fatalf("item after commit: %s", fresh.Status)
		}
		committed, err := st.GetPurgePlan(ctx, env.plan.ID)
		if err != nil {
			t.Fatal(err)
		}
		if committed.State != domain.PurgeCommitted {
			t.Fatalf("plan after commit: %s", committed.State)
		}
	})

	t.Run("retention_gate_refuses_early_promotion", func(t *testing.T) {
		dir := t.TempDir()
		st, err := store.Open(ctx, filepath.Join(dir, "purge.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close() //nolint:errcheck
		env := seedPurgeItem(t, ctx, st, dir, "young.txt", "young-payload", now)
		// Push the promotion timestamp before the retention deadline: the
		// store itself is the retention rule and must refuse.
		early := env.plan
		early.CreatedAt = env.item.RetainUntil.Add(-time.Minute)
		if err := st.SavePurgePlans(ctx, []domain.PurgePlan{early}); err == nil {
			t.Fatal("promotion before the retention deadline must be refused")
		}
	})

	t.Run("other_valid_copy_survives_the_unlink", func(t *testing.T) {
		dir := t.TempDir()
		st, err := store.Open(ctx, filepath.Join(dir, "purge.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close() //nolint:errcheck
		env := seedPurgeItem(t, ctx, st, dir, "linked.txt", "linked-payload", now)
		// A second hard link to the same inode is another valid copy; the
		// purge unlinks exactly one path and must never touch the other.
		sibling := filepath.Join(dir, "sibling-copy.txt")
		if err := os.Link(env.item.QuarantinePath, sibling); err != nil {
			t.Fatal(err)
		}
		if err := st.SavePurgePlans(ctx, []domain.PurgePlan{env.plan}); err != nil {
			t.Fatal(err)
		}
		svc := NewPurgeService(st)
		if err := svc.approvePlan(ctx, env.plan.ID, env.digest); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.executePurge(ctx, PurgeExecuteInput{PlanID: env.plan.ID, Digest: env.digest,
			QuarantineRoot: env.quarantineRoot, DryRun: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.executePurge(ctx, PurgeExecuteInput{PlanID: env.plan.ID, Digest: env.digest,
			QuarantineRoot: env.quarantineRoot, DryRun: false,
			Confirmation: PurgeConfirmationText(env.plan)}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(env.item.QuarantinePath); !os.IsNotExist(err) {
			t.Fatal("the purged path must be gone")
		}
		got, err := os.ReadFile(sibling)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "linked-payload" {
			t.Fatalf("the surviving valid copy was altered: %q", got)
		}
	})

	t.Run("snapshot_drift_is_refused_before_any_write", func(t *testing.T) {
		dir := t.TempDir()
		st, err := store.Open(ctx, filepath.Join(dir, "purge.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close() //nolint:errcheck
		env := seedPurgeItem(t, ctx, st, dir, "drifted.txt", "drifted-payload", now)
		if err := st.SavePurgePlans(ctx, []domain.PurgePlan{env.plan}); err != nil {
			t.Fatal(err)
		}
		svc := NewPurgeService(st)
		if err := svc.approvePlan(ctx, env.plan.ID, env.digest); err != nil {
			t.Fatal(err)
		}
		// Tamper after plan creation: the snapshot no longer matches.
		if err := os.WriteFile(env.item.QuarantinePath, []byte("tampered"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.executePurge(ctx, PurgeExecuteInput{PlanID: env.plan.ID, Digest: env.digest,
			QuarantineRoot: env.quarantineRoot, DryRun: true}); err == nil {
			t.Fatal("drifted snapshot must fail the dry run")
		}
		if _, err := svc.executePurge(ctx, PurgeExecuteInput{PlanID: env.plan.ID, Digest: env.digest,
			QuarantineRoot: env.quarantineRoot, DryRun: false,
			Confirmation: PurgeConfirmationText(env.plan)}); err == nil {
			t.Fatal("drifted snapshot must fail real execution")
		}
		// Nothing moved: the (tampered) object is still at its quarantine
		// path and no staging directory was created.
		if _, err := os.Lstat(env.item.QuarantinePath); err != nil {
			t.Fatal("the drifted object must remain in place")
		}
		if _, err := os.Lstat(filepath.Join(env.quarantineRoot, ".purge-staging")); !os.IsNotExist(err) {
			t.Fatal("a refused purge must not create staging state")
		}
	})

	t.Run("pre_commit_revocation_returns_the_object", func(t *testing.T) {
		dir := t.TempDir()
		st, err := store.Open(ctx, filepath.Join(dir, "purge.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close() //nolint:errcheck
		env := seedPurgeItem(t, ctx, st, dir, "revoked.txt", "revoked-payload", now)
		if err := st.SavePurgePlans(ctx, []domain.PurgePlan{env.plan}); err != nil {
			t.Fatal(err)
		}
		svc := NewPurgeService(st)
		if err := svc.approvePlan(ctx, env.plan.ID, env.digest); err != nil {
			t.Fatal(err)
		}
		// Crash footprint up to 'staged': journal begun, object moved to the
		// staging area — the irreversible commit has NOT happened.
		if err := st.BeginPurge(ctx, env.plan, env.item, env.stagingPath(), now); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(env.stagingPath()), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(env.item.QuarantinePath, env.stagingPath()); err != nil {
			t.Fatal(err)
		}
		if err := st.MarkPurgeStaged(ctx, env.plan.ID); err != nil {
			t.Fatal(err)
		}
		// Pre-commit revocation: the reversible direction. The journal is
		// marked rolled back and the staged object returns byte-for-byte.
		if err := st.MarkPurgeRolledBack(ctx, env.plan.ID, env.item.ID, now); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(env.stagingPath(), env.item.QuarantinePath); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(env.item.QuarantinePath)
		if err != nil || string(got) != "revoked-payload" {
			t.Fatalf("the revoked object must return intact: %q (%v)", got, err)
		}
		freshItem, err := st.GetQuarantineItem(ctx, env.item.ID)
		if err != nil {
			t.Fatal(err)
		}
		if freshItem.Status != domain.QuarantinePurgeEligible {
			t.Fatalf("item after revocation: %s", freshItem.Status)
		}
		freshPlan, err := st.GetPurgePlan(ctx, env.plan.ID)
		if err != nil {
			t.Fatal(err)
		}
		if freshPlan.State != domain.PurgeRolledBack {
			t.Fatalf("plan after revocation: %s", freshPlan.State)
		}
	})

	t.Run("crashed_journal_blocks_execution_and_preserves_bytes", func(t *testing.T) {
		dir := t.TempDir()
		st, err := store.Open(ctx, filepath.Join(dir, "purge.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close() //nolint:errcheck
		env := seedPurgeItem(t, ctx, st, dir, "crashed.txt", "crashed-payload", now)
		if err := st.SavePurgePlans(ctx, []domain.PurgePlan{env.plan}); err != nil {
			t.Fatal(err)
		}
		svc := NewPurgeService(st)
		if err := svc.approvePlan(ctx, env.plan.ID, env.digest); err != nil {
			t.Fatal(err)
		}
		// SIGKILL footprint after staging, before the commit point.
		if err := st.BeginPurge(ctx, env.plan, env.item, env.stagingPath(), now); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(env.stagingPath()), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(env.item.QuarantinePath, env.stagingPath()); err != nil {
			t.Fatal(err)
		}
		if err := st.MarkPurgeStaged(ctx, env.plan.ID); err != nil {
			t.Fatal(err)
		}
		// A second, unrelated plan must be refused while the journal is
		// unresolved: the recovery lock spans every dangerous execution.
		other := seedPurgeItem(t, ctx, st, dir, "blocked.txt", "blocked-payload", now)
		if err := st.SavePurgePlans(ctx, []domain.PurgePlan{other.plan}); err != nil {
			t.Fatal(err)
		}
		if err := svc.approvePlan(ctx, other.plan.ID, other.digest); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.executePurge(ctx, PurgeExecuteInput{PlanID: other.plan.ID, Digest: other.digest,
			QuarantineRoot: other.quarantineRoot, DryRun: false,
			Confirmation: PurgeConfirmationText(other.plan)}); err == nil {
			t.Fatal("an unresolved purge journal must block new executions")
		}
		// The authorization gate refuses on the same evidence.
		if err := st.MarkPurgeDryRunVerified(ctx, other.plan.ID, other.digest, now); err != nil {
			t.Fatal(err)
		}
		live, err := executor.Snapshot(other.item.QuarantinePath, false)
		if err != nil {
			t.Fatal(err)
		}
		el, err := NewAuthorizationGate(st).AuthorizePurge(ctx, other.plan.ID, other.digest, live.Device, live.Inode)
		if err == nil || el.Eligible || el.Reason != "recovery_lock" {
			t.Fatalf("the gate must refuse while a purge journal is unresolved: %s (%v)", el.Reason, err)
		}
		// Recovery keeps the staged bytes for manual reconciliation — no
		// automatic commit, no automatic rollback, nothing lost.
		recovered, err := svc.RecoverPurges(ctx, env.quarantineRoot)
		if err != nil {
			t.Fatal(err)
		}
		if len(recovered) != 1 || recovered[0].ErrorType != "manual_reconciliation_required" {
			t.Fatalf("recovery must demand manual reconciliation: %#v", recovered)
		}
		got, err := os.ReadFile(env.stagingPath())
		if err != nil || string(got) != "crashed-payload" {
			t.Fatalf("the staged object must survive the crash untouched: %q (%v)", got, err)
		}
	})

	t.Run("wrong_file_is_never_deleted", func(t *testing.T) {
		dir := t.TempDir()
		st, err := store.Open(ctx, filepath.Join(dir, "purge.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close() //nolint:errcheck
		env := seedPurgeItem(t, ctx, st, dir, "target.txt", "target-payload", now)
		neighbor := seedPurgeItem(t, ctx, st, dir, "neighbor.txt", "neighbor-payload", now)
		if err := st.SavePurgePlans(ctx, []domain.PurgePlan{env.plan, neighbor.plan}); err != nil {
			t.Fatal(err)
		}
		svc := NewPurgeService(st)
		if err := svc.approvePlan(ctx, env.plan.ID, env.digest); err != nil {
			t.Fatal(err)
		}
		// Commit exactly one identity: the neighbor's plan stays DRAFT and
		// its bytes stay in place.
		if _, err := svc.executePurge(ctx, PurgeExecuteInput{PlanID: env.plan.ID, Digest: env.digest,
			QuarantineRoot: env.quarantineRoot, DryRun: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.executePurge(ctx, PurgeExecuteInput{PlanID: env.plan.ID, Digest: env.digest,
			QuarantineRoot: env.quarantineRoot, DryRun: false,
			Confirmation: PurgeConfirmationText(env.plan)}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(env.item.QuarantinePath); !os.IsNotExist(err) {
			t.Fatal("the planned object must be gone")
		}
		got, err := os.ReadFile(neighbor.item.QuarantinePath)
		if err != nil || string(got) != "neighbor-payload" {
			t.Fatalf("the wrong file must never be deleted or altered: %q (%v)", got, err)
		}
	})
}
