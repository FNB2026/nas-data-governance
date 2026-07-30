package wails

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
)

// ---- Mapping helper tests ----

func TestMapExecutionStepPreservesAllFields(t *testing.T) {
	step := executor.AuditStep{
		Name:   "stale_check",
		Status: executor.StepOK,
		Detail: map[string]any{"checked": 3, "stale": 0},
	}
	dto := mapExecutionStep(step)
	if dto.Name != "stale_check" || dto.Status != "ok" {
		t.Fatalf("name=%q status=%q, want stale_check/ok", dto.Name, dto.Status)
	}
	if dto.Detail["checked"] != 3 || dto.Detail["stale"] != 0 {
		t.Fatalf("detail not preserved: %#v", dto.Detail)
	}
}

func TestMapExecutionStepWithNilDetail(t *testing.T) {
	step := executor.AuditStep{
		Name:   "scope_validation",
		Status: executor.StepSkipped,
	}
	dto := mapExecutionStep(step)
	if dto.Status != "skipped" {
		t.Fatalf("status=%q, want skipped", dto.Status)
	}
	if dto.Detail != nil {
		t.Fatalf("expected nil detail, got %#v", dto.Detail)
	}
}

func TestMapExecutionResultPreservesSteps(t *testing.T) {
	result := app.ExecutionResult{
		PlanID:     "plan-001",
		FinalState: domain.PlanVerified,
		Steps: []executor.AuditStep{
			{Name: "stale_check", Status: executor.StepOK},
			{Name: "scope_validation", Status: executor.StepOK},
			{Name: "execute_actions", Status: executor.StepOK},
		},
	}
	dto := mapExecutionResult(result)
	if dto.PlanID != "plan-001" {
		t.Fatalf("plan_id=%q, want plan-001", dto.PlanID)
	}
	if dto.FinalState != "VERIFIED" {
		t.Fatalf("final_state=%q, want VERIFIED", dto.FinalState)
	}
	if len(dto.Steps) != 3 {
		t.Fatalf("steps len=%d, want 3", len(dto.Steps))
	}
	if dto.Steps[0].Name != "stale_check" {
		t.Fatalf("step[0].name=%q, want stale_check", dto.Steps[0].Name)
	}
}

func TestMapExecutionResultWithErrorType(t *testing.T) {
	result := app.ExecutionResult{
		PlanID:     "plan-002",
		FinalState: domain.PlanRolledBack,
		Steps:      []executor.AuditStep{{Name: "stale_check", Status: executor.StepFailed}},
		ErrorType:  "stale_plan",
	}
	dto := mapExecutionResult(result)
	if dto.ErrorType != "stale_plan" {
		t.Fatalf("error_type=%q, want stale_plan", dto.ErrorType)
	}
	if dto.FinalState != "ROLLED_BACK" {
		t.Fatalf("final_state=%q, want ROLLED_BACK", dto.FinalState)
	}
}

// ---- ExecutePlans validation tests ----

func TestExecutePlansRejectsNoProjectOpen(t *testing.T) {
	api := NewAPI()
	_, err := api.ExecutePlans(ExecutePlansRequest{
		PlanIDs:        []string{"p1"},
		QuarantineRoot: "/quarantine",
		SourceRoots:    []string{"/source"},
	})
	if err != ErrNoProjectOpen {
		t.Fatalf("expected ErrNoProjectOpen, got %v", err)
	}
}

func TestExecutePlansRejectsReadOnlyMode(t *testing.T) {
	path := createProjectDB(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	_, err := api.ExecutePlans(ExecutePlansRequest{
		PlanIDs:        []string{"p1"},
		QuarantineRoot: "/quarantine",
		SourceRoots:    []string{"/source"},
	})
	if err != ErrProjectNotReadWrite {
		t.Fatalf("expected ErrProjectNotReadWrite, got %v", err)
	}
}

func TestExecutePlansRejectsEmptyPlanIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	_, err := api.ExecutePlans(ExecutePlansRequest{
		PlanIDs:        []string{},
		QuarantineRoot: "/quarantine",
		SourceRoots:    []string{"/source"},
	})
	if err == nil {
		t.Fatal("expected error for empty plan_ids")
	}
}

func TestExecutePlansRejectsMissingQuarantineRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	_, err := api.ExecutePlans(ExecutePlansRequest{
		PlanIDs:     []string{"p1"},
		SourceRoots: []string{"/source"},
	})
	if err == nil {
		t.Fatal("expected error for missing quarantine_root")
	}
}

func TestExecutePlansRejectsMissingSourceRoots(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	_, err := api.ExecutePlans(ExecutePlansRequest{
		PlanIDs:        []string{"p1"},
		QuarantineRoot: "/quarantine",
	})
	if err == nil {
		t.Fatal("expected error for missing source_roots")
	}
}

func TestExecutePlansRejectsLowRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	_, err := api.ExecutePlans(ExecutePlansRequest{
		PlanIDs:        []string{"p1"},
		QuarantineRoot: "/quarantine",
		SourceRoots:    []string{"/source"},
		RetentionHours: 1,
	})
	if err == nil {
		t.Fatal("expected error for retention < 24h")
	}
}

func TestExecutePlansNoEligiblePlansReturnsEmpty(t *testing.T) {
	path := createProjectDB(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	resp, err := api.ExecutePlans(ExecutePlansRequest{
		PlanIDs:        []string{"nonexistent-plan-id"},
		QuarantineRoot: "/quarantine",
		SourceRoots:    []string{"/source"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Executed != 0 || resp.Skipped != 0 || resp.Failed != 0 {
		t.Fatalf("expected all-zero counts, got executed=%d skipped=%d failed=%d",
			resp.Executed, resp.Skipped, resp.Failed)
	}
	if len(resp.Results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(resp.Results))
	}
}

// ---- SaveDraftPlans tests ----

func TestSaveDraftPlansRejectsReadOnlyMode(t *testing.T) {
	path := createProjectDB(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	_, err := api.SaveDraftPlans("")
	if err != ErrProjectNotReadWrite {
		t.Fatalf("expected ErrProjectNotReadWrite, got %v", err)
	}
}

func TestSaveDraftPlansEmptyDatabaseReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	plans, err := api.SaveDraftPlans("")
	if err != nil {
		t.Fatalf("SaveDraftPlans: %v", err)
	}
	if len(plans) != 0 {
		t.Fatalf("expected 0 plans for empty database, got %d", len(plans))
	}
}

func TestSaveDraftPlansPersistsGeneratedPlans(t *testing.T) {
	path := createProjectDBWithDuplicates(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}

	// SaveDraftPlans should generate plans from duplicate files and persist them.
	saved, err := api.SaveDraftPlans("")
	if err != nil {
		t.Fatalf("SaveDraftPlans: %v", err)
	}
	if len(saved) == 0 {
		t.Fatal("expected plans from seeded duplicate files, got 0")
	}

	// Verify plans are persisted by listing from the database.
	all, err := api.ListAllPlans()
	if err != nil {
		t.Fatalf("ListAllPlans: %v", err)
	}
	if len(all) != len(saved) {
		t.Fatalf("ListAllPlans returned %d, expected %d (same as saved)", len(all), len(saved))
	}

	// All persisted plans should be in DRAFT state.
	for _, p := range all {
		if p.State != string(domain.PlanDraft) {
			t.Errorf("plan %s state=%q, want DRAFT", p.ID, p.State)
		}
	}
}

// TestSmokeGovernanceExecutionLifecycle exercises the complete desktop write
// path through Wails bindings: scan -> persist drafts -> approve -> dry-run ->
// quarantine execution -> audit/journal registration.
func TestSmokeGovernanceExecutionLifecycle(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "dataset")
	cacheDir := filepath.Join(sourceRoot, "cache")
	quarantineRoot := filepath.Join(tmp, "quarantine")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(quarantineRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("desktop governance execution lifecycle")
	sources := []string{
		filepath.Join(cacheDir, "copy-a.txt"),
		filepath.Join(cacheDir, "copy-b.txt"),
	}
	for _, path := range sources {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}

	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })
	if _, err := api.OpenProjectReadWrite(filepath.Join(tmp, "project.db")); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}

	scan, err := api.StartScan(StartScanRequest{
		Root: sourceRoot, StorageID: "execution-smoke", FullScan: true, Workers: 1,
	})
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	progress := waitForJobTerminal(t, api, scan.JobID, 30*time.Second)
	if progress.State != "COMPLETED" {
		t.Fatalf("scan state=%s error=%s", progress.State, progress.ErrorCode)
	}

	drafts, err := api.SaveDraftPlans("execution-smoke")
	if err != nil {
		t.Fatalf("SaveDraftPlans: %v", err)
	}
	if len(drafts) == 0 {
		t.Fatal("expected at least one persisted governance plan")
	}
	planIDs := make([]string, len(drafts))
	for i, plan := range drafts {
		planIDs[i] = plan.ID
	}
	approved, err := api.ApprovePlans(ApprovePlansRequest{PlanIDs: planIDs})
	if err != nil {
		t.Fatalf("ApprovePlans: %v", err)
	}
	if len(approved.Approved) == 0 {
		t.Fatal("expected at least one approved plan")
	}

	dryRun, err := api.ExecutePlans(ExecutePlansRequest{
		PlanIDs: planIDs, QuarantineRoot: quarantineRoot,
		SourceRoots: []string{sourceRoot}, DryRun: true, RetentionHours: 720,
	})
	if err != nil {
		t.Fatalf("ExecutePlans dry-run: %v", err)
	}
	if dryRun.Failed != 0 || dryRun.Skipped == 0 || len(dryRun.Results) == 0 {
		t.Fatalf("unexpected dry-run result: %#v", dryRun)
	}
	for _, path := range sources {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("dry-run changed source %s: %v", path, err)
		}
	}
	if entries, err := os.ReadDir(quarantineRoot); err != nil || len(entries) != 0 {
		t.Fatalf("dry-run wrote quarantine entries=%d err=%v", len(entries), err)
	}

	realRun, err := api.ExecutePlans(ExecutePlansRequest{
		PlanIDs: planIDs, QuarantineRoot: quarantineRoot,
		SourceRoots: []string{sourceRoot}, DryRun: false, RetentionHours: 720,
	})
	if err != nil {
		t.Fatalf("ExecutePlans real: %v", err)
	}
	if realRun.Failed != 0 || realRun.Executed == 0 {
		t.Fatalf("unexpected real execution result: %#v", realRun)
	}
	items, err := api.ListQuarantineItems("")
	if err != nil {
		t.Fatalf("ListQuarantineItems: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("expected quarantine lifecycle items after real execution")
	}
	logs, err := api.ListOperationLogs(planIDs[0])
	if err != nil {
		t.Fatalf("ListOperationLogs: %v", err)
	}
	journal, err := api.ListJournalEntries(planIDs[0])
	if err != nil {
		t.Fatalf("ListJournalEntries: %v", err)
	}
	if len(logs) == 0 || len(journal) == 0 {
		t.Fatalf("expected audit and journal records, logs=%d journal=%d", len(logs), len(journal))
	}
}
