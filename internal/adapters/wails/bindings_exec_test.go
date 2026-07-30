package wails

import (
	"path/filepath"
	"testing"

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
