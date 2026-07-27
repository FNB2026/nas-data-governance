package app

import (
	"context"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// ReviewStore is the store subset needed by ReviewService.
// store.SQLiteStore satisfies this interface structurally.
type ReviewStore interface {
	SaveGroupDecision(ctx context.Context, d domain.GroupDecision) error
	GetGroupDecision(ctx context.Context, groupID string) (domain.GroupDecision, error)
	ListGroupDecisions(ctx context.Context, decisionType domain.ReviewDecisionType) ([]domain.GroupDecision, error)
	ListRules(ctx context.Context, source domain.RuleSource, status domain.RuleStatus) ([]domain.Rule, error)
	UpdateRuleStatus(ctx context.Context, ruleID string, status domain.RuleStatus, approvedAt *time.Time) error
	DisableBatch(ctx context.Context, batchID string) error
	ListPlans(ctx context.Context, taskID string) ([]domain.OperationPlan, error)
	ListTasks(ctx context.Context) ([]domain.OperationTask, error)
}

// ReviewService manages review decisions and rule lifecycle transitions.
// It does NOT handle interactive I/O; the CLI layer reads stdin and prints
// prompts, then calls this service to persist decisions.
type ReviewService struct {
	store ReviewStore
}

// NewReviewService creates a review service backed by the given store.
func NewReviewService(st ReviewStore) *ReviewService {
	return &ReviewService{store: st}
}

// SaveDecision records a review decision for a duplicate group.
func (s *ReviewService) SaveDecision(ctx context.Context, d domain.GroupDecision) error {
	return s.store.SaveGroupDecision(ctx, d)
}

// GetDecision returns the latest decision for a group.
func (s *ReviewService) GetDecision(ctx context.Context, groupID string) (domain.GroupDecision, error) {
	return s.store.GetGroupDecision(ctx, groupID)
}

// ListDecisions returns decisions, optionally filtered by type.
func (s *ReviewService) ListDecisions(ctx context.Context, dt domain.ReviewDecisionType) ([]domain.GroupDecision, error) {
	return s.store.ListGroupDecisions(ctx, dt)
}

// ListReviewPlans loads all plans from all tasks and returns those that
// contain at least one REVIEW action. These are the plans that need human
// review before they can proceed.
func (s *ReviewService) ListReviewPlans(ctx context.Context) ([]domain.OperationPlan, error) {
	tasks, err := s.store.ListTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("app: list review plans: %w", err)
	}
	var reviewPlans []domain.OperationPlan
	for _, t := range tasks {
		taskPlans, err := s.store.ListPlans(ctx, t.ID)
		if err != nil {
			return nil, fmt.Errorf("app: list plans for task %s: %w", t.ID, err)
		}
		for _, p := range taskPlans {
			for _, a := range p.Actions {
				if a.Action == domain.OperationReview {
					reviewPlans = append(reviewPlans, p)
					break
				}
			}
		}
	}
	return reviewPlans, nil
}

// ConvertReviewToSkip converts all REVIEW actions in a plan to SKIP,
// indicating the user chose to keep all files. The reason is prefixed
// with "keep-all by human review: ". This does not create an execution
// plan; it only records the user's intent.
func (s *ReviewService) ConvertReviewToSkip(p *domain.OperationPlan) {
	for j := range p.Actions {
		if p.Actions[j].Action == domain.OperationReview {
			p.Actions[j].Action = domain.OperationSkip
			p.Actions[j].Reason = "keep-all by human review: " + p.Actions[j].Reason
		}
	}
}

// ListPendingRules returns draft and probation rules that need review.
func (s *ReviewService) ListPendingRules(ctx context.Context) (drafts, probations []domain.Rule, err error) {
	drafts, err = s.store.ListRules(ctx, "", domain.RuleDraft)
	if err != nil {
		return nil, nil, fmt.Errorf("app: list draft rules: %w", err)
	}
	probations, err = s.store.ListRules(ctx, "", domain.RuleProbation)
	if err != nil {
		return nil, nil, fmt.Errorf("app: list probation rules: %w", err)
	}
	return drafts, probations, nil
}

// PromoteDraftToProbation transitions a draft rule to probation.
func (s *ReviewService) PromoteDraftToProbation(ctx context.Context, ruleID string) error {
	now := time.Now().UTC()
	return s.store.UpdateRuleStatus(ctx, ruleID, domain.RuleProbation, &now)
}

// PromoteProbationToApproved transitions a probation rule to approved.
func (s *ReviewService) PromoteProbationToApproved(ctx context.Context, ruleID string) error {
	now := time.Now().UTC()
	return s.store.UpdateRuleStatus(ctx, ruleID, domain.RuleApproved, &now)
}

// RejectRule transitions a rule to rejected status.
func (s *ReviewService) RejectRule(ctx context.Context, ruleID string) error {
	return s.store.UpdateRuleStatus(ctx, ruleID, domain.RuleRejected, nil)
}

// DisableRule transitions a rule to disabled status.
func (s *ReviewService) DisableRule(ctx context.Context, ruleID string) error {
	return s.store.UpdateRuleStatus(ctx, ruleID, domain.RuleDisabled, nil)
}

// DisableBatch disables all rules in a batch.
func (s *ReviewService) DisableBatch(ctx context.Context, batchID string) error {
	return s.store.DisableBatch(ctx, batchID)
}
