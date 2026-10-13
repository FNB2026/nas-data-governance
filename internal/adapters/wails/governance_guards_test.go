package wails

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestLegacyDecisionRejectsSandboxAndRecoveryLock(t *testing.T) {
	for _, mode := range []string{"sandbox", "recovery"} {
		t.Run(mode, func(t *testing.T) {
			a, _ := governanceAPIWithSource(t)
			page, err := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 20})
			if err != nil || len(page.Items) == 0 {
				t.Fatal("valid indexed subject required", err)
			}
			ref := page.Items[0].GovernanceSubject
			if mode == "sandbox" {
				a.sandbox = true
			}
			if mode == "recovery" {
				ctx := context.Background()
				task := domain.OperationTask{ID: "locked-task", RootPath: t.TempDir(), State: "executing", CreatedAt: time.Now()}
				if err := a.store.CreateTask(ctx, task); err != nil {
					t.Fatal(err)
				}
				if err := a.store.SavePlans(ctx, task.ID, []domain.OperationPlan{{ID: "locked-plan", TaskID: task.ID, State: domain.PlanExecuting}}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := a.SaveGroupDecision(SaveDecisionRequest{GroupID: ref.GroupID, DecisionType: "KEEP_ALL", SubjectVersion: ref.SubjectVersion, DecisionRevision: ref.DecisionRevision}); err == nil {
				t.Fatal("restricted decision write accepted")
			}
			if _, err := a.store.GetGroupDecision(context.Background(), ref.GroupID); err == nil {
				t.Fatal("restricted decision persisted")
			}
		})
	}
}

func TestSandboxManualDraftAndApprovalAreRejected(t *testing.T) {
	// Exercise the actual sandbox composition and an authorized, container-owned
	// disposable project, including when the binary's default build is MAS.
	a, _, source := sandboxTestAPI(t)
	if _, e := a.PickDirectory(""); e != nil {
		t.Fatal(e)
	}
	if _, e := a.CreateProjectFromSource(CreateProjectInput{SourcePath: source}); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	task := domain.OperationTask{ID: "synthetic-task", RootPath: t.TempDir(), State: "planning", CreatedAt: time.Now()}
	if e := a.store.CreateTask(ctx, task); e != nil {
		t.Fatal(e)
	}
	if e := a.store.SavePlans(ctx, task.ID, []domain.OperationPlan{{ID: "synthetic-plan", TaskID: task.ID, State: domain.PlanDraft, Risk: domain.RiskLow}}); e != nil {
		t.Fatal(e)
	}
	if _, e := a.ApprovePlans(ApprovePlansRequest{PlanIDs: []string{"synthetic-plan"}}); !errors.Is(e, errGovernanceUnavailable) {
		t.Error("sandbox approval did not return the governance refusal")
	}
	if _, e := a.SaveDraftPlans(""); !errors.Is(e, errGovernanceUnavailable) {
		t.Error("sandbox manual plan persistence did not return the governance refusal")
	}
	plans, e := a.store.ListAllPlans(ctx)
	if e != nil || len(plans) != 1 || plans[0].State != domain.PlanDraft {
		t.Error("sandbox changed approved/draft plan state")
	}
	c, e := a.GetAppCapabilities()
	if e != nil || c.CanApprovePlans {
		t.Error("sandbox advertised approval capability")
	}
}
