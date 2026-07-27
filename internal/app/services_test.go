package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/query"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// ---- ScanService ----

func TestScanService_JSONLOnlyMode(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "data")
	os.MkdirAll(filepath.Join(root, "temp"), 0o755)
	os.WriteFile(filepath.Join(root, "temp", "a.txt"), []byte("hello"), 0o644)
	os.WriteFile(filepath.Join(root, "temp", "b.txt"), []byte("world"), 0o644)

	svc := NewScanService(nil) // JSONL-only, no DB
	result, err := svc.Scan(context.Background(), ScanInput{
		Root:           root,
		StorageID:      "test",
		Workers:        1,
		HashAttempts:   3,
		HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(result.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result.Files))
	}
	if len(result.HashFailures) != 0 {
		t.Fatalf("expected 0 hash failures, got %d", len(result.HashFailures))
	}
	if result.CheckpointID != 0 {
		t.Fatalf("JSONL-only mode should not create checkpoint, got %d", result.CheckpointID)
	}
	// Each file should have a quick hash.
	for _, f := range result.Files {
		if f.QuickHash == "" {
			t.Fatalf("file %s has no quick hash", f.Name)
		}
	}
}

func TestScanService_DuplicateDetectionTriggersFullHash(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "data")
	os.MkdirAll(root, 0o755)
	content := []byte("identical content for full hash")
	os.WriteFile(filepath.Join(root, "a.txt"), content, 0o644)
	os.WriteFile(filepath.Join(root, "b.txt"), content, 0o644)

	svc := NewScanService(nil)
	result, err := svc.Scan(context.Background(), ScanInput{
		Root:           root,
		StorageID:      "test",
		Workers:        1,
		HashAttempts:   3,
		HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(result.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result.Files))
	}
	// Duplicate files should have ContentSHA256 computed.
	for _, f := range result.Files {
		if f.ContentSHA256 == "" {
			t.Fatalf("duplicate file %s should have full SHA-256", f.Name)
		}
	}
	// Both should share the same SHA-256.
	if result.Files[0].ContentSHA256 != result.Files[1].ContentSHA256 {
		t.Fatalf("duplicates should share SHA-256")
	}
}

func TestScanService_HashFailuresRecorded(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "data")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("content"), 0o644)

	// Use a hash function that always fails.
	failFn := func(string, int64) (string, error) {
		return "", errTestHash
	}
	svc := NewScanServiceWithHashFunc(nil, failFn, failFn)
	result, err := svc.Scan(context.Background(), ScanInput{
		Root:           root,
		StorageID:      "test",
		Workers:        1,
		HashAttempts:   2,
		HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("scan should not fail on hash errors: %v", err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("file should still be in index: got %d files", len(result.Files))
	}
	if result.Files[0].QuickHash != "" {
		t.Fatalf("failed file should not have quick hash")
	}
	if len(result.HashFailures) != 1 {
		t.Fatalf("expected 1 hash failure, got %d", len(result.HashFailures))
	}
	if result.HashFailures[0].Stage != "quick" {
		t.Fatalf("failure stage should be quick, got %s", result.HashFailures[0].Stage)
	}
	if result.HashFailures[0].Attempts != 2 {
		t.Fatalf("failure attempts should be 2, got %d", result.HashFailures[0].Attempts)
	}
}

func TestScanService_DBPersistenceAndIncrementalReuse(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "data")
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "a.txt"), []byte("content"), 0o644)

	dbPath := filepath.Join(tmp, "governance.db")
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// First scan: computes hashes and persists to DB.
	svc := NewScanService(st)
	result, err := svc.Scan(context.Background(), ScanInput{
		Root:           root,
		StorageID:      "test",
		Workers:        1,
		HashAttempts:   3,
		HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}
	if result.Files[0].QuickHash == "" {
		t.Fatalf("file should have quick hash after first scan")
	}
	if result.CheckpointID == 0 {
		t.Fatalf("DB-backed scan should create checkpoint")
	}
	if !result.FullTraversal {
		t.Fatalf("scan without errors should be full traversal")
	}

	// Verify file was persisted to DB.
	files, err := st.ListFiles(context.Background(), "test")
	if err != nil || len(files) != 1 {
		t.Fatalf("DB should have 1 file: len=%d err=%v", len(files), err)
	}

	// Second scan: should reuse cached hash (incremental).
	hashCalls := 0
	realQuick := svc.quickHash
	svc.quickHash = func(path string, size int64) (string, error) {
		hashCalls++
		return realQuick(path, size)
	}
	result2, err := svc.Scan(context.Background(), ScanInput{
		Root:           root,
		StorageID:      "test",
		Workers:        1,
		HashAttempts:   3,
		HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if hashCalls != 0 {
		t.Fatalf("incremental scan should not re-hash unchanged file, got %d calls", hashCalls)
	}
	if len(result2.Files) != 1 {
		t.Fatalf("second scan should find 1 file, got %d", len(result2.Files))
	}
	if result2.Files[0].QuickHash != result.Files[0].QuickHash {
		t.Fatalf("incremental scan should reuse cached quick hash")
	}
}

func TestScanService_ProgressReporting(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "data")
	os.MkdirAll(root, 0o755)
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(root, fileLetter(i)+".txt"), []byte("x"), 0o644)
	}

	svc := NewScanService(nil)
	// Before scan, progress should be idle.
	p := svc.Progress()
	if p.Stage != "idle" {
		t.Fatalf("expected idle stage, got %s", p.Stage)
	}

	_, err := svc.Scan(context.Background(), ScanInput{
		Root:           root,
		StorageID:      "test",
		Workers:        1,
		HashAttempts:   3,
		HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	// After scan, progress should be completed.
	p = svc.Progress()
	if p.Stage != "completed" {
		t.Fatalf("expected completed stage, got %s", p.Stage)
	}
	if p.Discovered != 5 {
		t.Fatalf("expected 5 discovered, got %d", p.Discovered)
	}
	if p.Processed != 5 {
		t.Fatalf("expected 5 processed, got %d", p.Processed)
	}
}

// ---- DuplicateService ----

func TestDuplicateService_InMemoryGrouping(t *testing.T) {
	files := []domain.FileInstance{
		{StorageID: "s", Path: "/a.txt", Name: "a.txt", Size: 10, ContentSHA256: "abc"},
		{StorageID: "s", Path: "/b.txt", Name: "b.txt", Size: 10, ContentSHA256: "abc"},
		{StorageID: "s", Path: "/c.txt", Name: "c.txt", Size: 20, ContentSHA256: "def"},
	}
	svc := NewDuplicateService()
	groups := svc.DuplicateGroups(context.Background(), files)
	if len(groups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d", len(groups))
	}
	if groups[0].PathCount != 2 {
		t.Fatalf("group should have 2 paths, got %d", groups[0].PathCount)
	}
}

func TestDuplicateService_PanicsWithoutReader(t *testing.T) {
	svc := NewDuplicateService()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when calling ListGroups without reader")
		}
	}()
	svc.ListGroups(context.Background(), query.GroupQuery{})
}

// ---- PlanService ----

func TestPlanService_BuildPlans(t *testing.T) {
	files := []domain.FileInstance{
		{StorageID: "s", Path: "/a.txt", Name: "a.txt", Size: 10, ContentSHA256: "abc"},
		{StorageID: "s", Path: "/b.txt", Name: "b.txt", Size: 10, ContentSHA256: "abc"},
	}
	svc := NewPlanService()
	plans := svc.BuildPlans(context.Background(), files)
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if plans[0].State != domain.PlanDraft {
		t.Fatalf("plan should be DRAFT, got %s", plans[0].State)
	}
}

func TestPlanService_ApprovePlansByAll(t *testing.T) {
	plans := []domain.OperationPlan{
		{ID: "p1", State: domain.PlanDraft, Risk: domain.RiskMedium},
		{ID: "p2", State: domain.PlanDraft, Risk: domain.RiskLow},
	}
	svc := NewPlanService()
	result, err := svc.ApprovePlans(context.Background(), ApprovePlansInput{
		Plans: plans, All: true,
	})
	if err != nil {
		t.Fatalf("approve all: %v", err)
	}
	if len(result.Approved) != 2 {
		t.Fatalf("expected 2 approved, got %d", len(result.Approved))
	}
	for _, p := range result.Approved {
		if p.State != domain.PlanApproved {
			t.Fatalf("plan %s should be APPROVED, got %s", p.ID, p.State)
		}
	}
}

func TestPlanService_ApprovePlansByID(t *testing.T) {
	plans := []domain.OperationPlan{
		{ID: "p1", State: domain.PlanDraft, Risk: domain.RiskMedium},
		{ID: "p2", State: domain.PlanDraft, Risk: domain.RiskLow},
	}
	svc := NewPlanService()
	result, err := svc.ApprovePlans(context.Background(), ApprovePlansInput{
		Plans: plans, IDs: []string{"p2"},
	})
	if err != nil {
		t.Fatalf("approve by ID: %v", err)
	}
	if len(result.Approved) != 1 {
		t.Fatalf("expected 1 approved, got %d", len(result.Approved))
	}
	if result.Approved[0].ID != "p2" {
		t.Fatalf("expected p2 approved, got %s", result.Approved[0].ID)
	}
}

func TestPlanService_ApproveRejectsNonDraft(t *testing.T) {
	plans := []domain.OperationPlan{
		{ID: "p1", State: domain.PlanApproved},
	}
	svc := NewPlanService()
	_, err := svc.ApprovePlans(context.Background(), ApprovePlansInput{
		Plans: plans, All: true,
	})
	if err == nil {
		t.Fatal("expected error approving non-draft plan")
	}
}

func TestPlanService_ApproveFreezesCritical(t *testing.T) {
	plans := []domain.OperationPlan{
		{ID: "p1", State: domain.PlanDraft, Risk: domain.RiskCritical},
	}
	svc := NewPlanService()
	_, err := svc.ApprovePlans(context.Background(), ApprovePlansInput{
		Plans: plans, All: true,
	})
	if err == nil {
		t.Fatal("expected error approving critical plan")
	}
}

func TestPlanService_ApproveRequiresSelection(t *testing.T) {
	svc := NewPlanService()
	_, err := svc.ApprovePlans(context.Background(), ApprovePlansInput{
		Plans: []domain.OperationPlan{{ID: "p1", State: domain.PlanDraft}},
	})
	if err == nil {
		t.Fatal("expected error when no selection provided")
	}
}

// ---- ExecutionService ----

// scanForPlans is a test helper that scans a directory, builds plans from
// the scanned files, and approves them. This ensures file instances have
// valid 64-char SHA-256 hashes so the planner assigns medium risk (not
// critical) to temp-directory duplicates.
func scanForPlans(t *testing.T, root, storageID string) []domain.OperationPlan {
	t.Helper()
	scanSvc := NewScanService(nil)
	result, err := scanSvc.Scan(context.Background(), ScanInput{
		Root: root, StorageID: storageID, Workers: 1,
		HashAttempts: 3, HashRetryDelay: 0,
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	planSvc := NewPlanService()
	plans := planSvc.BuildPlans(context.Background(), result.Files)
	approved, err := planSvc.ApprovePlans(context.Background(), ApprovePlansInput{Plans: plans, All: true})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	return approved.Approved
}

func TestExecutionService_DryRunSkipsFilesystem(t *testing.T) {
	tmp := t.TempDir()
	dataRoot := filepath.Join(tmp, "data")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(filepath.Join(dataRoot, "temp"), 0o755)
	os.MkdirAll(qRoot, 0o755)
	content := []byte("dry run content")
	src1 := filepath.Join(dataRoot, "temp", "a.txt")
	src2 := filepath.Join(dataRoot, "temp", "b.txt")
	os.WriteFile(src1, content, 0o644)
	os.WriteFile(src2, content, 0o644)

	approved := scanForPlans(t, dataRoot, "s")

	// Dry-run execution: no DB, no filesystem writes.
	execSvc := NewExecutionService(nil)
	summary, err := execSvc.Execute(context.Background(), ExecutionInput{
		Plans:          approved,
		QuarantineRoot: qRoot,
		SourceRoots:    []string{dataRoot},
		DryRun:         true,
		Retention:      24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("dry-run execute: %v", err)
	}
	if summary.Failed > 0 {
		t.Fatalf("dry-run should not fail, got %d failed", summary.Failed)
	}
	// Source files should still exist.
	if _, err := os.Stat(src1); err != nil {
		t.Fatalf("src1 should still exist after dry-run: %v", err)
	}
	if _, err := os.Stat(src2); err != nil {
		t.Fatalf("src2 should still exist after dry-run: %v", err)
	}
	// Quarantine should be empty.
	entries, _ := os.ReadDir(qRoot)
	if len(entries) != 0 {
		t.Fatalf("quarantine should be empty after dry-run, got %d entries", len(entries))
	}
}

func TestExecutionService_RealExecutionWithQuarantine(t *testing.T) {
	tmp := t.TempDir()
	dataRoot := filepath.Join(tmp, "data")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(filepath.Join(dataRoot, "temp"), 0o755)
	os.MkdirAll(qRoot, 0o755)
	content := []byte("real exec content")
	src1 := filepath.Join(dataRoot, "temp", "a.txt")
	src2 := filepath.Join(dataRoot, "temp", "b.txt")
	os.WriteFile(src1, content, 0o644)
	os.WriteFile(src2, content, 0o644)

	dbPath := filepath.Join(tmp, "governance.db")
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	approved := scanForPlans(t, dataRoot, "s")

	execSvc := NewExecutionService(st)
	summary, err := execSvc.Execute(context.Background(), ExecutionInput{
		Plans:          approved,
		QuarantineRoot: qRoot,
		SourceRoots:    []string{dataRoot},
		DryRun:         false,
		Retention:      30 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if summary.Executed != 1 {
		t.Fatalf("expected 1 executed, got %d", summary.Executed)
	}
	// Quarantine should have 1 file.
	entries, _ := os.ReadDir(qRoot)
	if len(entries) != 1 {
		t.Fatalf("expected 1 quarantined file, got %d", len(entries))
	}
}

func TestExecutionService_SkipsNonApproved(t *testing.T) {
	tmp := t.TempDir()
	qRoot := filepath.Join(tmp, "quarantine")
	srcRoot := filepath.Join(tmp, "src")
	os.MkdirAll(qRoot, 0o755)
	os.MkdirAll(srcRoot, 0o755)

	execSvc := NewExecutionService(nil)
	summary, err := execSvc.Execute(context.Background(), ExecutionInput{
		Plans: []domain.OperationPlan{
			{ID: "p1", State: domain.PlanDraft},
		},
		QuarantineRoot: qRoot,
		SourceRoots:    []string{srcRoot},
		DryRun:         true,
		Retention:      24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if summary.Skipped != 1 {
		t.Fatalf("expected 1 skipped, got %d", summary.Skipped)
	}
}

func TestExecutionService_RequiresQuarantineRoot(t *testing.T) {
	execSvc := NewExecutionService(nil)
	_, err := execSvc.Execute(context.Background(), ExecutionInput{
		SourceRoots: []string{"/tmp"},
		DryRun:      true,
		Retention:   24 * time.Hour,
	})
	if err == nil {
		t.Fatal("expected error for missing quarantine root")
	}
}

func TestExecutionService_RequiresSourceRoots(t *testing.T) {
	execSvc := NewExecutionService(nil)
	_, err := execSvc.Execute(context.Background(), ExecutionInput{
		QuarantineRoot: "/tmp",
		DryRun:         true,
		Retention:      24 * time.Hour,
	})
	if err == nil {
		t.Fatal("expected error for missing source roots")
	}
}

func TestExecutionService_RequiresDBForRealExecution(t *testing.T) {
	execSvc := NewExecutionService(nil)
	_, err := execSvc.Execute(context.Background(), ExecutionInput{
		QuarantineRoot: "/tmp",
		SourceRoots:    []string{"/tmp"},
		DryRun:         false,
		Retention:      24 * time.Hour,
	})
	if err == nil {
		t.Fatal("expected error for real execution without DB")
	}
}

func TestExecutionService_RetentionMinimum(t *testing.T) {
	execSvc := NewExecutionService(nil)
	_, err := execSvc.Execute(context.Background(), ExecutionInput{
		QuarantineRoot: "/tmp",
		SourceRoots:    []string{"/tmp"},
		DryRun:         true,
		Retention:      1 * time.Hour,
	})
	if err == nil {
		t.Fatal("expected error for retention < 24h")
	}
}

// ---- ReviewService ----

func TestReviewService_ConvertReviewToSkip(t *testing.T) {
	plan := &domain.OperationPlan{
		ID:     "p1",
		Actions: []domain.PlannedAction{
			{Action: domain.OperationReview, Reason: "needs human review"},
			{Action: domain.OperationKeep, Reason: "retain"},
		},
	}
	svc := NewReviewService(nil)
	svc.ConvertReviewToSkip(plan)

	if plan.Actions[0].Action != domain.OperationSkip {
		t.Fatalf("REVIEW should become SKIP, got %s", plan.Actions[0].Action)
	}
	if plan.Actions[1].Action != domain.OperationKeep {
		t.Fatalf("KEEP should remain KEEP, got %s", plan.Actions[1].Action)
	}
}

// ---- Helpers ----

var errTestHash = &testError{"test hash failure"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func fileLetter(i int) string {
	return string(rune('a' + i))
}
