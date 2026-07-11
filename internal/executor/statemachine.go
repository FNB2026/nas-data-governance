package executor

import (
	"fmt"

	"nas-data-governance/internal/domain"
)

// ErrIllegalTransition is returned when a state transition is not allowed
// by the white-paper pipeline. The error never includes the plan ID or
// path, only the state names, so it is safe to log at any level.
var ErrIllegalTransition = fmt.Errorf("executor: illegal plan state transition")

// legalTransitions encodes the white-paper pipeline (§36-41):
//
//	DRAFT ──approve──▶ APPROVED ──stale-check──▶ STALE_CHECKED ──execute──▶ EXECUTING ──verify──▶ VERIFIED
//	  │                   │                                                            │
//	  │                   └──stale-fail──▶ DRAFT (back for re-review)                   │
//	  │                                                                                │
//	  └──reject──▶ ROLLED_BACK ◀──rollback──────────────────────────────────────────────┘
//
// VERIFIED and ROLLED_BACK are terminal; no further transition is allowed.
// The forward path cannot skip steps: APPROVED cannot jump to EXECUTING,
// because the stale check is mandatory before any write.
var legalTransitions = map[domain.PlanState]map[domain.PlanState]bool{
	domain.PlanDraft: {
		domain.PlanApproved:   true,
		domain.PlanRolledBack: true,
	},
	domain.PlanApproved: {
		domain.PlanStaleChecked: true,
		domain.PlanDraft:        true, // stale check failed; back for re-review
	},
	domain.PlanStaleChecked: {
		domain.PlanExecuting: true,
	},
	domain.PlanExecuting: {
		domain.PlanVerified:   true,
		domain.PlanRolledBack: true,
	},
	domain.PlanVerified:   {},
	domain.PlanRolledBack: {},
}

// CanTransition reports whether moving from `from` to `to` is legal.
func CanTransition(from, to domain.PlanState) bool {
	targets, ok := legalTransitions[from]
	if !ok {
		return false
	}
	return targets[to]
}

// Transition applies a state change to a plan in place. Returns
// ErrIllegalTransition when the move is not allowed by the pipeline.
// The plan's State field is updated only on success.
func Transition(plan *domain.OperationPlan, to domain.PlanState) error {
	if plan == nil {
		return fmt.Errorf("executor: nil plan")
	}
	if !CanTransition(plan.State, to) {
		return fmt.Errorf("%w: %s → %s", ErrIllegalTransition, plan.State, to)
	}
	plan.State = to
	return nil
}
