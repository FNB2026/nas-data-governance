package wails

import (
	"context"
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
	a := NewAPI()
	defer a.CloseProject()
	if _, e := a.OpenProjectReadWrite(createProjectDBWithDuplicates(t)); e != nil {
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
	a.sandbox = true
	if _, e := a.ApprovePlans(ApprovePlansRequest{PlanIDs: []string{"synthetic-plan"}}); e == nil {
		t.Error("sandbox approval accepted")
	}
	if _, e := a.SaveDraftPlans(""); e == nil {
		t.Error("sandbox manual plan persistence accepted")
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
