package app

import (
	"context"
	"errors"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

var ErrGovernanceReadOnly = errors.New("governance recording unavailable")

// GovernanceDecisionStore is record-only: approval, execution, source writes
// and recovery-lock clearing are intentionally absent from the port.
type GovernanceDecisionStore interface {
	GovernanceCapabilities(context.Context) (domain.GovernanceCapabilities, error)
	ListGovernanceGroups(context.Context, domain.GovernanceQuery) (domain.GovernancePage, error)
	PreviewBatchDecisions(context.Context, domain.PreviewBatchRequest, bool) (domain.GovernancePreview, error)
	CommitBatchDecisions(context.Context, domain.CommitBatchRequest) (domain.GovernanceBatchResult, error)
	EvaluateGovernancePreview(context.Context, domain.PreviewBatchRequest) (domain.GovernancePreview, domain.GovernanceCapabilities, error)
	PersistGovernancePreview(context.Context, domain.PreviewBatchRequest, domain.GovernancePreview) (domain.GovernancePreview, error)
	GetGovernancePreview(context.Context, string) (domain.GovernancePreview, error)
	CommitBatchDecisionsWithFreshness(context.Context, domain.CommitBatchRequest, map[string]domain.SourceFreshness) (domain.GovernanceBatchResult, error)
	GetBatchDecisionResult(context.Context, string) (domain.GovernanceBatchResult, error)
	ListGovernanceBatches(context.Context, domain.GovernanceBatchQuery) (domain.GovernanceBatchPage, error)
}

// GovernanceSourceFreshness verifies only explicitly selected duplicate-group
// members. It owns source authorization; storage persistence never opens paths.
type GovernanceSourceFreshness interface {
	VerifyGovernanceSources(context.Context, []domain.GovernanceSubject) map[string]domain.SourceFreshness
}
type GovernanceDecisionService struct {
	store         GovernanceDecisionStore
	recordAllowed bool
	freshness     GovernanceSourceFreshness
}

func NewGovernanceDecisionService(st GovernanceDecisionStore, recordAllowed bool, freshness GovernanceSourceFreshness) *GovernanceDecisionService {
	return &GovernanceDecisionService{store: st, recordAllowed: recordAllowed, freshness: freshness}
}
func (s *GovernanceDecisionService) Capabilities(ctx context.Context) (domain.GovernanceCapabilities, error) {
	c, e := s.store.GovernanceCapabilities(ctx)
	if !s.recordAllowed {
		c.CanRecordDecisions = false
		c.ReasonCodes = append(c.ReasonCodes, "READ_ONLY_CHANNEL")
	}
	return c, e
}
func (s *GovernanceDecisionService) List(ctx context.Context, q domain.GovernanceQuery) (domain.GovernancePage, error) {
	return s.store.ListGovernanceGroups(ctx, q)
}
func (s *GovernanceDecisionService) Preview(ctx context.Context, q domain.PreviewBatchRequest) (domain.GovernancePreview, error) {
	preview, caps, err := s.store.EvaluateGovernancePreview(ctx, q)
	if err != nil {
		return preview, err
	}
	if !s.recordAllowed || !caps.CanRecordDecisions {
		for i := range preview.Items {
			if preview.Items[i].Eligibility == "ELIGIBLE" {
				preview.Items[i].Eligibility, preview.Items[i].ReasonCode = "BLOCKED", "RECORDING_UNAVAILABLE"
			}
		}
		return preview, nil
	}
	refs := make([]domain.GovernanceSubject, 0, len(preview.Items))
	for _, item := range preview.Items {
		if item.Eligibility == "ELIGIBLE" {
			refs = append(refs, item.GovernanceSubject)
		}
	}
	checks := s.verify(ctx, refs)
	for i := range preview.Items {
		item := &preview.Items[i]
		if item.Eligibility != "ELIGIBLE" {
			continue
		}
		check, ok := checks[item.GroupID]
		item.SourceFreshness, item.SourceCheckedAt = check.Status, check.CheckedAt
		if !ok || check.Status != "MATCH" {
			item.Eligibility = "BLOCKED"
			item.ReasonCode = domain.SourceFreshnessReason(check.Status, ok)
		}
	}
	return s.store.PersistGovernancePreview(ctx, q, preview)
}
func (s *GovernanceDecisionService) Commit(ctx context.Context, q domain.CommitBatchRequest) (domain.GovernanceBatchResult, error) {
	if !s.recordAllowed {
		return domain.GovernanceBatchResult{}, ErrGovernanceReadOnly
	}
	preview, err := s.store.GetGovernancePreview(ctx, q.PreviewToken)
	if err != nil {
		return domain.GovernanceBatchResult{}, err
	}
	refs := make([]domain.GovernanceSubject, 0, len(preview.Items))
	for _, item := range preview.Items {
		if item.Eligibility == "ELIGIBLE" {
			refs = append(refs, item.GovernanceSubject)
		}
	}
	return s.store.CommitBatchDecisionsWithFreshness(ctx, q, s.verify(ctx, refs))
}

func (s *GovernanceDecisionService) verify(ctx context.Context, refs []domain.GovernanceSubject) map[string]domain.SourceFreshness {
	checks := make(map[string]domain.SourceFreshness, len(refs))
	if len(refs) == 0 {
		return checks
	}
	if s.freshness != nil {
		checks = s.freshness.VerifyGovernanceSources(ctx, refs)
	}
	for _, ref := range refs {
		if _, ok := checks[ref.GroupID]; !ok {
			checks[ref.GroupID] = domain.SourceFreshness{Status: "UNVERIFIED"}
		}
	}
	return checks
}
func (s *GovernanceDecisionService) Result(ctx context.Context, id string) (domain.GovernanceBatchResult, error) {
	return s.store.GetBatchDecisionResult(ctx, id)
}

func (s *GovernanceDecisionService) History(ctx context.Context, q domain.GovernanceBatchQuery) (domain.GovernanceBatchPage, error) {
	return s.store.ListGovernanceBatches(ctx, q)
}
