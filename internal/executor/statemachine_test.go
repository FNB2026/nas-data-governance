package executor

import (
	"testing"

	"nas-data-governance/internal/domain"
)

func TestCanTransitionForwardPath(t *testing.T) {
	cases := []struct{ from, to domain.PlanState }{
		{domain.PlanDraft, domain.PlanApproved},
		{domain.PlanApproved, domain.PlanStaleChecked},
		{domain.PlanStaleChecked, domain.PlanExecuting},
		{domain.PlanExecuting, domain.PlanVerified},
	}
	for _, c := range cases {
		if !CanTransition(c.from, c.to) {
			t.Errorf("expected %s → %s to be legal", c.from, c.to)
		}
	}
}

func TestCanTransitionRejectsSkips(t *testing.T) {
	// The forward path cannot skip steps.
	cases := []struct{ from, to domain.PlanState }{
		{domain.PlanDraft, domain.PlanExecuting},       // skip approve + stale
		{domain.PlanApproved, domain.PlanExecuting},    // skip stale check
		{domain.PlanStaleChecked, domain.PlanVerified}, // skip execute
		{domain.PlanDraft, domain.PlanVerified},        // skip everything
	}
	for _, c := range cases {
		if CanTransition(c.from, c.to) {
			t.Errorf("expected %s → %s to be illegal", c.from, c.to)
		}
	}
}

func TestCanTransitionRejectsBackwardFromTerminal(t *testing.T) {
	// Terminal states cannot move.
	cases := []struct{ from, to domain.PlanState }{
		{domain.PlanVerified, domain.PlanDraft},
		{domain.PlanVerified, domain.PlanExecuting},
		{domain.PlanRolledBack, domain.PlanDraft},
		{domain.PlanRolledBack, domain.PlanExecuting},
	}
	for _, c := range cases {
		if CanTransition(c.from, c.to) {
			t.Errorf("expected %s → %s to be illegal (terminal state)", c.from, c.to)
		}
	}
}

func TestCanTransitionAllowsRollbackFromExecuting(t *testing.T) {
	if !CanTransition(domain.PlanExecuting, domain.PlanRolledBack) {
		t.Error("expected EXECUTING → ROLLED_BACK to be legal")
	}
}

func TestCanTransitionAllowsRejectFromDraft(t *testing.T) {
	if !CanTransition(domain.PlanDraft, domain.PlanRolledBack) {
		t.Error("expected DRAFT → ROLLED_BACK to be legal")
	}
}

func TestCanTransitionAllowsBackToDraftOnStaleFail(t *testing.T) {
	if !CanTransition(domain.PlanApproved, domain.PlanDraft) {
		t.Error("expected APPROVED → DRAFT to be legal (stale-fail re-review)")
	}
}

func TestTransitionUpdatesStateOnSuccess(t *testing.T) {
	plan := &domain.OperationPlan{State: domain.PlanDraft}
	if err := Transition(plan, domain.PlanApproved); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.State != domain.PlanApproved {
		t.Fatalf("state not updated, got %s", plan.State)
	}
}

func TestTransitionReturnsErrorOnIllegalMove(t *testing.T) {
	plan := &domain.OperationPlan{State: domain.PlanDraft}
	err := Transition(plan, domain.PlanExecuting)
	if err == nil {
		t.Fatal("expected ErrIllegalTransition")
	}
	if plan.State != domain.PlanDraft {
		t.Fatalf("state should not change on error, got %s", plan.State)
	}
}

func TestTransitionRejectsNilPlan(t *testing.T) {
	if err := Transition(nil, domain.PlanApproved); err == nil {
		t.Fatal("expected error for nil plan")
	}
}
