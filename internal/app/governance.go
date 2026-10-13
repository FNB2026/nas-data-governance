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
	GetBatchDecisionResult(context.Context, string) (domain.GovernanceBatchResult, error)
	ListGovernanceBatches(context.Context, domain.GovernanceBatchQuery) (domain.GovernanceBatchPage, error)
}
type GovernanceDecisionService struct {
	store         GovernanceDecisionStore
	recordAllowed bool
}

func NewGovernanceDecisionService(st GovernanceDecisionStore, recordAllowed bool) *GovernanceDecisionService {
	return &GovernanceDecisionService{st, recordAllowed}
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
	return s.store.PreviewBatchDecisions(ctx, q, s.recordAllowed)
}
func (s *GovernanceDecisionService) Commit(ctx context.Context, q domain.CommitBatchRequest) (domain.GovernanceBatchResult, error) {
	if !s.recordAllowed {
		return domain.GovernanceBatchResult{}, ErrGovernanceReadOnly
	}
	return s.store.CommitBatchDecisions(ctx, q)
}
func (s *GovernanceDecisionService) Result(ctx context.Context, id string) (domain.GovernanceBatchResult, error) {
	return s.store.GetBatchDecisionResult(ctx, id)
}

func (s *GovernanceDecisionService) History(ctx context.Context, q domain.GovernanceBatchQuery) (domain.GovernanceBatchPage, error) {
	return s.store.ListGovernanceBatches(ctx, q)
}
