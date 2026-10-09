package executor

import (
	"context"
	"errors"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

type StateWriter func(context.Context, string, domain.PlanState, domain.PlanState) error

// NewWithJournalAndState reserves execution through conditional durable
// transitions before journal creation and before filesystem writes.
// Terminal states are finalized atomically by the application service.
func NewWithJournalAndState(q QuarantineConfig, j Journal, w StateWriter) (*Executor, error) {
	e, err := NewWithJournal(q, j)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, errors.New("executor: state writer required")
	}
	e.stateWriter = w
	return e, nil
}

func (e *Executor) transition(ctx context.Context, p *domain.OperationPlan, to domain.PlanState) error {
	if !CanTransition(p.State, to) {
		return ErrIllegalTransition
	}
	if e.stateWriter != nil {
		if err := e.stateWriter(ctx, p.ID, p.State, to); err != nil {
			return errors.New("executor: durable state transition failed")
		}
	}
	return Transition(p, to)
}
