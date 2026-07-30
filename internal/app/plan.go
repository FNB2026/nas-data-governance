package app

import (
	"context"
	"fmt"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/planner"
	"github.com/FNB2026/nas-data-governance/internal/report"
)

// PlanService creates governance plans from duplicate groups and manages
// plan approval. Plan creation is read-only (no filesystem writes); approval
// transitions plans from DRAFT to APPROVED, subject to the critical-hold gate.
type PlanService struct{}

// NewPlanService creates a new plan service.
func NewPlanService() *PlanService {
	return &PlanService{}
}

// BuildPlans generates draft governance plans from a set of file instances.
// The files are grouped into duplicate groups, then the planner produces
// one OperationPlan per group. All plans start in DRAFT state.
func (s *PlanService) BuildPlans(ctx context.Context, files []domain.FileInstance) []domain.OperationPlan {
	groups := report.DuplicateGroups(files)
	return planner.Build(groups)
}

// ApprovePlansInput defines which plans to approve.
type ApprovePlansInput struct {
	// Plans is the full set of plans loaded from the draft file.
	Plans []domain.OperationPlan
	// IDs specifies plan IDs to approve. When All is true, IDs is ignored.
	IDs []string
	// All approves all plans in the slice.
	All bool
}

// ApprovePlansResult holds the outcome of an approval batch.
type ApprovePlansResult struct {
	Approved []domain.OperationPlan
}

// ApprovePlans transitions selected plans from DRAFT to APPROVED.
// Critical-risk plans are frozen and cannot be approved here; they require
// an independent hold-release workflow (per existing CLI behavior).
func (s *PlanService) ApprovePlans(ctx context.Context, in ApprovePlansInput) (*ApprovePlansResult, error) {
	if !in.All && len(in.IDs) == 0 {
		return nil, fmt.Errorf("app: ApprovePlans: --plan-id or --all is required")
	}
	want := make(map[string]bool, len(in.IDs))
	for _, id := range in.IDs {
		want[id] = true
	}
	approved := make([]domain.OperationPlan, 0, len(in.Plans))
	for _, p := range in.Plans {
		if !in.All && !want[p.ID] {
			continue
		}
		if p.State != domain.PlanDraft {
			return nil, fmt.Errorf("plan %s is in state %s, expected DRAFT", p.ID, p.State)
		}
		if p.Risk == domain.RiskCritical {
			return nil, fmt.Errorf("plan %s is critical and remains HOLD; independent hold release is required", p.ID)
		}
		if err := executor.Transition(&p, domain.PlanApproved); err != nil {
			return nil, fmt.Errorf("plan %s: %w", p.ID, err)
		}
		approved = append(approved, p)
	}
	if len(approved) == 0 {
		return nil, fmt.Errorf("no plans matched the selection")
	}
	return &ApprovePlansResult{Approved: approved}, nil
}
