package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// seedQuarantineItem creates a managed quarantine item in the database by
// simulating the execution pipeline: create task → save plan → begin journal →
// mark done → register quarantine. Returns the seeded item.
//
// The quarantineRoot directory must already exist. The quarantine file is
// created on disk at quarantineRoot/name.
func seedQuarantineItem(
	t *testing.T,
	st *store.SQLiteStore,
	sourceRoot, quarantineRoot, name string,
	quarantinedAt, retainUntil time.Time,
) domain.QuarantineItem {
	t.Helper()
	ctx := context.Background()
	qPath := filepath.Join(quarantineRoot, name)
	if err := os.WriteFile(qPath, []byte("test-"+name), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := executor.Snapshot(qPath, true)
	if err != nil {
		t.Fatal(err)
	}
	taskID := "task-" + name
	planID := "plan-" + name
	sourcePath := filepath.Join(sourceRoot, name)
	if err := st.CreateTask(ctx, domain.OperationTask{
		ID: taskID, RootPath: sourceRoot, State: "completed", CreatedAt: quarantinedAt,
	}); err != nil {
		t.Fatal(err)
	}
	action := domain.PlannedAction{
		Path:    sourcePath,
		Action:  domain.OperationQuarantine,
		Context: domain.DirectoryContext{Role: domain.RoleTemporary},
		File: domain.FileInstance{
			Path: sourcePath, Size: snapshot.Size, ContentSHA256: snapshot.Hash,
		},
	}
	plan := domain.OperationPlan{
		ID: planID, TaskID: taskID, State: domain.PlanApproved,
		Actions: []domain.PlannedAction{action},
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
	items, err := st.RegisterQuarantinesFromJournal(ctx, planID, quarantinedAt, retainUntil)
	if err != nil || len(items) != 1 {
		t.Fatalf("register item: len=%d err=%v", len(items), err)
	}
	return items[0]
}

// openTestStore creates a temporary SQLite database for testing.
func openTestStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// ---- QuarantineService: ListItems ----

func TestQuarantineService_ListItems_EmptyDB(t *testing.T) {
	st := openTestStore(t)
	svc := NewQuarantineService(st)
	items, err := svc.ListItems(context.Background(), "")
	if err != nil {
		t.Fatalf("list on empty DB: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestQuarantineService_ListItems_WithStatusFilter(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	seedQuarantineItem(t, st, sourceRoot, qRoot, "a.dat", now, now.Add(30*24*time.Hour))

	svc := NewQuarantineService(st)
	// Filter by active status — should find the seeded item.
	active, err := svc.ListItems(context.Background(), domain.QuarantineActive)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active item, got %d", len(active))
	}
	// Filter by a different status — should find nothing.
	purged, err := svc.ListItems(context.Background(), domain.QuarantinePurged)
	if err != nil {
		t.Fatalf("list purged: %v", err)
	}
	if len(purged) != 0 {
		t.Fatalf("expected 0 purged items, got %d", len(purged))
	}
	// No filter — should find the item.
	all, err := svc.ListItems(context.Background(), "")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 item (no filter), got %d", len(all))
	}
}

// ---- QuarantineService: CreateRestorePlan ----

func TestQuarantineService_CreateRestorePlan(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	item := seedQuarantineItem(t, st, sourceRoot, qRoot, "a.dat", now, now.Add(30*24*time.Hour))

	svc := NewQuarantineService(st)
	plan, err := svc.CreateRestorePlan(context.Background(), item.ID)
	if err != nil {
		t.Fatalf("create restore plan: %v", err)
	}
	if plan.ItemID != item.ID {
		t.Fatalf("plan item ID mismatch: %s != %s", plan.ItemID, item.ID)
	}
	if plan.ApprovalDigest == "" {
		t.Fatal("plan should have a non-empty approval digest")
	}
	if plan.State != domain.RestoreDraft {
		t.Fatalf("plan should be DRAFT, got %s", plan.State)
	}
	if plan.QuarantinePath != item.QuarantinePath {
		t.Fatalf("plan quarantine path mismatch: %s != %s", plan.QuarantinePath, item.QuarantinePath)
	}
	listed, err := svc.ListRestorePlans(context.Background())
	if err != nil || len(listed) != 1 || listed[0].ID != plan.ID {
		t.Fatalf("durable restore plan was not reloadable: %#v, %v", listed, err)
	}
}

func TestQuarantineService_CreateRestorePlan_EmptyItemID(t *testing.T) {
	st := openTestStore(t)
	svc := NewQuarantineService(st)
	_, err := svc.CreateRestorePlan(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty item ID")
	}
}

func TestQuarantineService_CreateRestorePlan_NonexistentItem(t *testing.T) {
	st := openTestStore(t)
	svc := NewQuarantineService(st)
	_, err := svc.CreateRestorePlan(context.Background(), "nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent item")
	}
}

// ---- QuarantineService: ApproveRestorePlan ----

func TestQuarantineService_ApproveRestorePlan(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	item := seedQuarantineItem(t, st, sourceRoot, qRoot, "a.dat", now, now.Add(30*24*time.Hour))
	svc := NewQuarantineService(st)
	plan, err := svc.CreateRestorePlan(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ApproveRestorePlan(context.Background(), plan.ID, plan.ApprovalDigest); err != nil {
		t.Fatalf("approve: %v", err)
	}
}

func TestQuarantineService_ApproveRestorePlan_EmptyInputs(t *testing.T) {
	st := openTestStore(t)
	svc := NewQuarantineService(st)
	if err := svc.ApproveRestorePlan(context.Background(), "", "digest"); err == nil {
		t.Fatal("expected error for empty plan ID")
	}
	if err := svc.ApproveRestorePlan(context.Background(), "plan-id", ""); err == nil {
		t.Fatal("expected error for empty digest")
	}
}

func TestQuarantineService_ApproveRestorePlan_WrongDigest(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	item := seedQuarantineItem(t, st, sourceRoot, qRoot, "a.dat", now, now.Add(30*24*time.Hour))
	svc := NewQuarantineService(st)
	plan, err := svc.CreateRestorePlan(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ApproveRestorePlan(context.Background(), plan.ID, "wrong-digest"); err == nil {
		t.Fatal("expected error for wrong digest")
	}
}

// ---- QuarantineService: ExecuteRestore ----

func TestQuarantineService_ExecuteRestore_DryRun(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	item := seedQuarantineItem(t, st, sourceRoot, qRoot, "a.dat", now, now.Add(30*24*time.Hour))
	svc := NewQuarantineService(st)
	plan, err := svc.CreateRestorePlan(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ApproveRestorePlan(context.Background(), plan.ID, plan.ApprovalDigest); err != nil {
		t.Fatal(err)
	}
	result, err := svc.ExecuteRestore(context.Background(), RestoreExecuteInput{
		PlanID:         plan.ID,
		Digest:         plan.ApprovalDigest,
		QuarantineRoot: qRoot,
		SourceRoots:    []string{sourceRoot},
		DryRun:         true,
	})
	if err != nil {
		t.Fatalf("dry-run execute: %v", err)
	}
	if result.Result.Err != nil {
		t.Fatalf("dry-run should not fail: %s", result.Result.ErrorType)
	}
	// Quarantine file should still exist after dry-run.
	if _, err := os.Stat(item.QuarantinePath); err != nil {
		t.Fatalf("quarantine file should exist after dry-run: %v", err)
	}
	// Source file should NOT exist after dry-run (it was quarantined, not restored).
	if _, err := os.Stat(item.SourcePath); !os.IsNotExist(err) {
		t.Fatalf("source should not exist (was quarantined): %v", err)
	}
}

func TestQuarantineService_ExecuteRestore_RealExecution(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	item := seedQuarantineItem(t, st, sourceRoot, qRoot, "a.dat", now, now.Add(30*24*time.Hour))
	svc := NewQuarantineService(st)
	plan, err := svc.CreateRestorePlan(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ApproveRestorePlan(context.Background(), plan.ID, plan.ApprovalDigest); err != nil {
		t.Fatal(err)
	}
	result, err := svc.ExecuteRestore(context.Background(), RestoreExecuteInput{
		PlanID:         plan.ID,
		Digest:         plan.ApprovalDigest,
		QuarantineRoot: qRoot,
		SourceRoots:    []string{sourceRoot},
		DryRun:         false,
	})
	if err != nil {
		t.Fatalf("real execute: %v", err)
	}
	if result.Result.Err != nil {
		t.Fatalf("execute should not fail: %s", result.Result.ErrorType)
	}
	// Source file should be restored.
	if _, err := os.Stat(item.SourcePath); err != nil {
		t.Fatalf("source should exist after restore: %v", err)
	}
	// Quarantine file should be removed after restore.
	if _, err := os.Stat(item.QuarantinePath); !os.IsNotExist(err) {
		t.Fatalf("quarantine file should be removed after restore: %v", err)
	}
}

func TestQuarantineService_ExecuteRestore_WrongDigest(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	item := seedQuarantineItem(t, st, sourceRoot, qRoot, "a.dat", now, now.Add(30*24*time.Hour))
	svc := NewQuarantineService(st)
	plan, err := svc.CreateRestorePlan(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ApproveRestorePlan(context.Background(), plan.ID, plan.ApprovalDigest); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ExecuteRestore(context.Background(), RestoreExecuteInput{
		PlanID:         plan.ID,
		Digest:         "wrong-digest",
		QuarantineRoot: qRoot,
		SourceRoots:    []string{sourceRoot},
		DryRun:         true,
	})
	if err == nil {
		t.Fatal("expected error for wrong digest at execution")
	}
}

func TestQuarantineService_ExecuteRestore_MissingInputs(t *testing.T) {
	st := openTestStore(t)
	svc := NewQuarantineService(st)
	// All required fields missing.
	_, err := svc.ExecuteRestore(context.Background(), RestoreExecuteInput{})
	if err == nil {
		t.Fatal("expected error for missing inputs")
	}
	// Missing source roots.
	_, err = svc.ExecuteRestore(context.Background(), RestoreExecuteInput{
		PlanID:         "p1",
		Digest:         "d1",
		QuarantineRoot: "/tmp/q",
	})
	if err == nil {
		t.Fatal("expected error for missing source roots")
	}
}

// ---- QuarantineService: RecoverRestores ----

func TestQuarantineService_RecoverRestores_EmptyDB(t *testing.T) {
	tmp := t.TempDir()
	qRoot := filepath.Join(tmp, "quarantine")
	sourceRoot := filepath.Join(tmp, "source")
	os.MkdirAll(qRoot, 0o700)
	os.MkdirAll(sourceRoot, 0o700)
	st := openTestStore(t)
	svc := NewQuarantineService(st)
	results, err := svc.RecoverRestores(context.Background(), RecoverRestoresInput{
		QuarantineRoot: qRoot,
		SourceRoots:    []string{sourceRoot},
	})
	if err != nil {
		t.Fatalf("recover on empty DB: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results on empty DB, got %d", len(results))
	}
}

func TestQuarantineService_RecoverRestores_MissingInputs(t *testing.T) {
	st := openTestStore(t)
	svc := NewQuarantineService(st)
	_, err := svc.RecoverRestores(context.Background(), RecoverRestoresInput{})
	if err == nil {
		t.Fatal("expected error for missing inputs")
	}
	_, err = svc.RecoverRestores(context.Background(), RecoverRestoresInput{
		QuarantineRoot: "/tmp/q",
	})
	if err == nil {
		t.Fatal("expected error for missing source roots")
	}
}

// ---- PurgeService: CreatePlans ----

func TestPurgeService_CreatePlans_NoEligibleItems(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	// Seed an item that is NOT purge-eligible (retainUntil is in the future).
	seedQuarantineItem(t, st, sourceRoot, qRoot, "a.dat", now, now.Add(30*24*time.Hour))

	svc := NewPurgeService(st)
	plans, err := svc.CreatePlans(context.Background())
	if err != nil {
		t.Fatalf("create plans: %v", err)
	}
	if len(plans) != 0 {
		t.Fatalf("expected 0 plans for non-eligible item, got %d", len(plans))
	}
}

func TestPurgeService_CreatePlans_WithEligibleItem(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	// Seed an item that IS purge-eligible (retainUntil is in the past).
	item := seedQuarantineItem(t, st, sourceRoot, qRoot, "purge.dat",
		now.Add(-48*time.Hour), now.Add(-24*time.Hour))

	svc := NewPurgeService(st)
	plans, err := svc.CreatePlans(context.Background())
	if err != nil {
		t.Fatalf("create plans: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if plans[0].ItemID != item.ID {
		t.Fatalf("plan item ID mismatch: %s != %s", plans[0].ItemID, item.ID)
	}
	if plans[0].ApprovalDigest == "" {
		t.Fatal("plan should have a non-empty approval digest")
	}
	if plans[0].State != domain.PurgeDraft {
		t.Fatalf("plan should be DRAFT, got %s", plans[0].State)
	}
}

func TestPurgeService_CreatePlans_NoDuplicatePlans(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	seedQuarantineItem(t, st, sourceRoot, qRoot, "purge.dat",
		now.Add(-48*time.Hour), now.Add(-24*time.Hour))

	svc := NewPurgeService(st)
	// First call should create 1 plan.
	plans1, err := svc.CreatePlans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(plans1) != 1 {
		t.Fatalf("first call: expected 1 plan, got %d", len(plans1))
	}
	// Second call should not duplicate (the item already has an active plan).
	plans2, err := svc.CreatePlans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(plans2) != 0 {
		t.Fatalf("second call: expected 0 plans (no duplicates), got %d", len(plans2))
	}
}

// ---- PurgeService: ApprovePlan ----

func TestPurgeService_ApprovePlan(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	seedQuarantineItem(t, st, sourceRoot, qRoot, "purge.dat",
		now.Add(-48*time.Hour), now.Add(-24*time.Hour))
	svc := NewPurgeService(st)
	plans, err := svc.CreatePlans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ApprovePlan(context.Background(), plans[0].ID, plans[0].ApprovalDigest); err != nil {
		t.Fatalf("approve: %v", err)
	}
}

func TestPurgeService_ApprovePlan_EmptyInputs(t *testing.T) {
	st := openTestStore(t)
	svc := NewPurgeService(st)
	if err := svc.ApprovePlan(context.Background(), "", "digest"); err == nil {
		t.Fatal("expected error for empty plan ID")
	}
	if err := svc.ApprovePlan(context.Background(), "plan-id", ""); err == nil {
		t.Fatal("expected error for empty digest")
	}
}

func TestPurgeService_ApprovePlan_WrongDigest(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	seedQuarantineItem(t, st, sourceRoot, qRoot, "purge.dat",
		now.Add(-48*time.Hour), now.Add(-24*time.Hour))
	svc := NewPurgeService(st)
	plans, err := svc.CreatePlans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ApprovePlan(context.Background(), plans[0].ID, "wrong-digest"); err == nil {
		t.Fatal("expected error for wrong digest")
	}
}

// ---- PurgeService: ExecutePurge ----

func TestPurgeService_ExecutePurge_DryRun(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	item := seedQuarantineItem(t, st, sourceRoot, qRoot, "purge.dat",
		now.Add(-48*time.Hour), now.Add(-24*time.Hour))
	svc := NewPurgeService(st)
	plans, err := svc.CreatePlans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ApprovePlan(context.Background(), plans[0].ID, plans[0].ApprovalDigest); err != nil {
		t.Fatal(err)
	}
	result, err := svc.ExecutePurge(context.Background(), PurgeExecuteInput{
		PlanID:         plans[0].ID,
		Digest:         plans[0].ApprovalDigest,
		QuarantineRoot: qRoot,
		DryRun:         true,
	})
	if err != nil {
		t.Fatalf("dry-run execute: %v", err)
	}
	if result.Result.Err != nil {
		t.Fatalf("dry-run should not fail: %s", result.Result.ErrorType)
	}
	// Quarantine file should still exist after dry-run.
	if _, err := os.Stat(item.QuarantinePath); err != nil {
		t.Fatalf("quarantine file should exist after dry-run: %v", err)
	}
	persisted, err := st.GetPurgePlan(context.Background(), plans[0].ID)
	if err != nil || persisted.DryRunVerifiedAt == nil || persisted.DryRunDigest != plans[0].ApprovalDigest {
		t.Fatalf("dry-run gate was not persisted: %#v, %v", persisted, err)
	}
	listed, err := svc.ListPlans(context.Background())
	if err != nil || len(listed) != 1 || listed[0].DryRunVerifiedAt == nil {
		t.Fatalf("durable purge plan was not reloadable: %#v, %v", listed, err)
	}
}

func TestPurgeService_ExecutePurge_RealExecution(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	item := seedQuarantineItem(t, st, sourceRoot, qRoot, "purge.dat",
		now.Add(-48*time.Hour), now.Add(-24*time.Hour))
	svc := NewPurgeService(st)
	plans, err := svc.CreatePlans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ApprovePlan(context.Background(), plans[0].ID, plans[0].ApprovalDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExecutePurge(context.Background(), PurgeExecuteInput{
		PlanID:         plans[0].ID,
		Digest:         plans[0].ApprovalDigest,
		QuarantineRoot: qRoot,
		DryRun:         true,
	}); err != nil {
		t.Fatalf("required dry-run: %v", err)
	}
	result, err := svc.ExecutePurge(context.Background(), PurgeExecuteInput{
		PlanID:         plans[0].ID,
		Digest:         plans[0].ApprovalDigest,
		QuarantineRoot: qRoot,
		DryRun:         false,
		Confirmation:   PurgeConfirmationText(plans[0]),
	})
	if err != nil {
		t.Fatalf("real execute: %v", err)
	}
	if result.Result.Err != nil {
		t.Fatalf("execute should not fail: %s", result.Result.ErrorType)
	}
	// Quarantine file should be permanently deleted after purge.
	if _, err := os.Stat(item.QuarantinePath); !os.IsNotExist(err) {
		t.Fatalf("quarantine file should be deleted after purge: %v", err)
	}
}

func TestPurgeService_ExecutePurge_RequiresDryRunAndConfirmation(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	seedQuarantineItem(t, st, sourceRoot, qRoot, "purge.dat",
		now.Add(-48*time.Hour), now.Add(-24*time.Hour))
	svc := NewPurgeService(st)
	plans, err := svc.CreatePlans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	plan := plans[0]
	if err := svc.ApprovePlan(context.Background(), plan.ID, plan.ApprovalDigest); err != nil {
		t.Fatal(err)
	}
	base := PurgeExecuteInput{PlanID: plan.ID, Digest: plan.ApprovalDigest, QuarantineRoot: qRoot}
	if _, err := svc.ExecutePurge(context.Background(), base); err == nil {
		t.Fatal("real purge must require a successful dry-run")
	}
	dry := base
	dry.DryRun = true
	if _, err := svc.ExecutePurge(context.Background(), dry); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	base.Confirmation = "wrong confirmation"
	if _, err := svc.ExecutePurge(context.Background(), base); err == nil {
		t.Fatal("real purge must reject an incorrect confirmation")
	}
}

func TestPurgeService_ExecutePurge_WrongDigest(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(sourceRoot, 0o700)
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	now := time.Now().UTC()
	seedQuarantineItem(t, st, sourceRoot, qRoot, "purge.dat",
		now.Add(-48*time.Hour), now.Add(-24*time.Hour))
	svc := NewPurgeService(st)
	plans, err := svc.CreatePlans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ApprovePlan(context.Background(), plans[0].ID, plans[0].ApprovalDigest); err != nil {
		t.Fatal(err)
	}
	_, err = svc.ExecutePurge(context.Background(), PurgeExecuteInput{
		PlanID:         plans[0].ID,
		Digest:         "wrong-digest",
		QuarantineRoot: qRoot,
		DryRun:         true,
	})
	if err == nil {
		t.Fatal("expected error for wrong digest at execution")
	}
}

func TestPurgeService_ExecutePurge_MissingInputs(t *testing.T) {
	st := openTestStore(t)
	svc := NewPurgeService(st)
	// All required fields missing.
	_, err := svc.ExecutePurge(context.Background(), PurgeExecuteInput{})
	if err == nil {
		t.Fatal("expected error for missing inputs")
	}
	// Missing quarantine root.
	_, err = svc.ExecutePurge(context.Background(), PurgeExecuteInput{
		PlanID: "p1",
		Digest: "d1",
	})
	if err == nil {
		t.Fatal("expected error for missing quarantine root")
	}
}

// ---- PurgeService: RecoverPurges ----

func TestPurgeService_RecoverPurges_EmptyDB(t *testing.T) {
	tmp := t.TempDir()
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(qRoot, 0o700)
	st := openTestStore(t)
	svc := NewPurgeService(st)
	results, err := svc.RecoverPurges(context.Background(), qRoot)
	if err != nil {
		t.Fatalf("recover on empty DB: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results on empty DB, got %d", len(results))
	}
}

func TestPurgeService_RecoverPurges_EmptyQuarantineRoot(t *testing.T) {
	st := openTestStore(t)
	svc := NewPurgeService(st)
	_, err := svc.RecoverPurges(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty quarantine root")
	}
}
