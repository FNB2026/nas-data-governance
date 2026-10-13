package wails

import (
	"context"
	"errors"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/domain"
)

var errGovernanceUnavailable = errors.New("governance recording unavailable; refresh and review safety state")

func (a *API) governanceServiceLocked() *app.GovernanceDecisionService {
	return app.NewGovernanceDecisionService(a.store, a.scanRunner != nil && !a.sandbox && !a.closing)
}
func (a *API) GetGovernanceCapabilities() (domain.GovernanceCapabilities, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.store == nil {
		return domain.GovernanceCapabilities{MaxBatchSize: 100, ReasonCodes: []string{"PROJECT_CLOSED"}}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, e := a.governanceServiceLocked().Capabilities(ctx)
	if e != nil {
		return c, errGovernanceUnavailable
	}
	return c, nil
}
func (a *API) ListGovernanceGroups(q domain.GovernanceQuery) (domain.GovernancePage, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.store == nil {
		return domain.GovernancePage{}, ErrNoProjectOpen
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	v, e := a.governanceServiceLocked().List(ctx, q)
	if e != nil {
		return domain.GovernancePage{}, errGovernanceUnavailable
	}
	return v, nil
}
func (a *API) PreviewBatchDecisions(q domain.PreviewBatchRequest) (domain.GovernancePreview, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.store == nil {
		return domain.GovernancePreview{}, ErrNoProjectOpen
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	v, e := a.governanceServiceLocked().Preview(ctx, q)
	if e != nil {
		return domain.GovernancePreview{}, errGovernanceUnavailable
	}
	return v, nil
}
func (a *API) CommitBatchDecisions(q domain.CommitBatchRequest) (domain.GovernanceBatchResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.store == nil {
		return domain.GovernanceBatchResult{}, ErrNoProjectOpen
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	v, e := a.governanceServiceLocked().Commit(ctx, q)
	if e != nil {
		return domain.GovernanceBatchResult{}, errGovernanceUnavailable
	}
	return v, nil
}
func (a *API) GetBatchDecisionResult(id string) (domain.GovernanceBatchResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.store == nil {
		return domain.GovernanceBatchResult{}, ErrNoProjectOpen
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	v, e := a.governanceServiceLocked().Result(ctx, id)
	if e != nil {
		return domain.GovernanceBatchResult{}, errGovernanceUnavailable
	}
	return v, nil
}
