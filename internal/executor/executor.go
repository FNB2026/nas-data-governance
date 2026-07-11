package executor

import (
	"context"
	"fmt"
	"time"

	"nas-data-governance/internal/domain"
)

// StepStatus is the outcome of one pipeline step.
type StepStatus string

const (
	StepOK      StepStatus = "ok"
	StepSkipped StepStatus = "skipped"
	StepFailed  StepStatus = "failed"
)

// AuditStep is one entry in the execution trail. Detail carries only
// abstract, safe-to-log information (byte counts, stale reasons, error
// types) — never file paths or content, per AGENTS rule 6.
type AuditStep struct {
	Name   string         `json:"name"`
	Status StepStatus     `json:"status"`
	Detail map[string]any `json:"detail,omitempty"`
}

// Result captures the outcome of executing one plan.
type Result struct {
	PlanID     string           `json:"plan_id"`
	FinalState domain.PlanState `json:"final_state"`
	Steps      []AuditStep      `json:"steps"`
	// Err is non-nil when the pipeline failed. It never carries file
	// paths; callers can correlate via PlanID and the plan's actions.
	Err error `json:"-"`
}

// Executor runs approved plans through the safe-operation pipeline.
// It is the only component that writes to the user's file system.
type Executor struct {
	quarantine QuarantineConfig
	now        func() time.Time
}

// New creates an executor with the given quarantine config. `now` defaults
// to time.Now but can be overridden in tests for deterministic paths.
func New(q QuarantineConfig) (*Executor, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	return &Executor{quarantine: q, now: time.Now}, nil
}

// Execute runs all actions in a plan through the pipeline:
//
//	APPROVED → stale-check → STALE_CHECKED → execute → VERIFIED
//	                                          ↓ fail
//	                                      ROLLED_BACK (with rollback)
//
// plan.State must be PlanApproved before calling. On any action failure,
// already-executed actions are rolled back in reverse order before the
// plan transitions to ROLLED_BACK. The returned Result.Steps is the full
// audit trail; callers are responsible for persisting it.
func (e *Executor) Execute(ctx context.Context, plan *domain.OperationPlan) Result {
	result := Result{PlanID: plan.ID, Steps: make([]AuditStep, 0), FinalState: plan.State}

	if plan.State != domain.PlanApproved {
		result.Err = fmt.Errorf("executor: plan state is %s, expected APPROVED", plan.State)
		return result
	}

	// Step 1: stale-check every filesystem-touching action.
	staleCount, err := e.staleCheckAll(ctx, plan, &result)
	if err != nil {
		result.Err = err
		return result
	}
	if staleCount > 0 {
		// Some files changed since the plan was generated: send the plan
		// back to DRAFT for human re-review. No filesystem writes happen.
		_ = Transition(plan, domain.PlanDraft)
		result.FinalState = plan.State
		return result
	}
	if err := Transition(plan, domain.PlanStaleChecked); err != nil {
		result.Err = err
		return result
	}

	// Step 2: execute actions in order.
	_ = Transition(plan, domain.PlanExecuting)
	result.FinalState = plan.State
	var rollbacks []func() error
	for _, action := range plan.Actions {
		if err := ctx.Err(); err != nil {
			result.Err = err
			e.rollbackAll(&result, &rollbacks)
			_ = Transition(plan, domain.PlanRolledBack)
			result.FinalState = plan.State
			return result
		}
		if !touchesFilesystem(action.Action) {
			result.Steps = append(result.Steps, AuditStep{
				Name: string(action.Action), Status: StepSkipped,
				Detail: map[string]any{"reason": "non-filesystem action"},
			})
			continue
		}
		step, rollback, err := e.executeAction(ctx, action)
		result.Steps = append(result.Steps, step)
		if err != nil {
			result.Err = err
			e.rollbackAll(&result, &rollbacks)
			_ = Transition(plan, domain.PlanRolledBack)
			result.FinalState = plan.State
			return result
		}
		if rollback != nil {
			rollbacks = append(rollbacks, rollback)
		}
	}

	// All actions succeeded.
	if err := Transition(plan, domain.PlanVerified); err != nil {
		result.Err = err
		return result
	}
	result.FinalState = plan.State
	return result
}

// staleCheckAll runs CheckStale on every filesystem-touching action and
// appends one AuditStep summarizing the result. Returns the count of
// stale files found (0 = all fresh).
func (e *Executor) staleCheckAll(ctx context.Context, plan *domain.OperationPlan, result *Result) (int, error) {
	checked, stale := 0, 0
	reasons := make(map[string]StaleReason)
	for _, action := range plan.Actions {
		if !touchesFilesystem(action.Action) {
			continue
		}
		checked++
		reason, err := CheckStale(action.File, true)
		if err != nil {
			return 0, fmt.Errorf("stale check: %w", err)
		}
		if reason != StaleNone {
			stale++
			reasons[action.File.Path] = reason
		}
	}
	if stale > 0 {
		result.Steps = append(result.Steps, AuditStep{
			Name: "stale_check", Status: StepFailed,
			Detail: map[string]any{
				"checked": checked,
				"stale":   stale,
			},
		})
	} else {
		result.Steps = append(result.Steps, AuditStep{
			Name: "stale_check", Status: StepOK,
			Detail: map[string]any{"checked": checked},
		})
	}
	return stale, nil
}

// executeAction dispatches one action to its handler. Returns an audit
// step, an optional rollback function, and an error.
func (e *Executor) executeAction(ctx context.Context, action domain.PlannedAction) (AuditStep, func() error, error) {
	switch action.Action {
	case domain.OperationQuarantine:
		return e.executeQuarantine(action)
	case domain.OperationKeep, domain.OperationSkip, domain.OperationReview:
		return AuditStep{Name: string(action.Action), Status: StepSkipped,
			Detail: map[string]any{"reason": "non-filesystem action"}}, nil, nil
	default:
		return AuditStep{Name: string(action.Action), Status: StepFailed,
				Detail: map[string]any{"error_type": "not_implemented"}}, nil,
			fmt.Errorf("executor: action %s not implemented", action.Action)
	}
}

// executeQuarantine moves a source file into the quarantine area using
// the copy-verify-delete pipeline. The rollback hook moves the file back
// to its original path if a later action fails.
func (e *Executor) executeQuarantine(action domain.PlannedAction) (AuditStep, func() error, error) {
	nominal := e.quarantine.PathFor(action.Path, e.now())
	dst, err := ResolveCollision(nominal)
	if err != nil {
		return AuditStep{Name: "quarantine", Status: StepFailed,
			Detail: map[string]any{"error_type": "collision_unresolved"}}, nil, err
	}
	if err := MoveFile(action.Path, dst, action.File.ContentSHA256); err != nil {
		return AuditStep{Name: "quarantine", Status: StepFailed,
			Detail: map[string]any{"error_type": "move_failed", "bytes": action.File.Size}}, nil, err
	}
	rollback := func() error {
		return MoveFile(dst, action.Path, action.File.ContentSHA256)
	}
	return AuditStep{Name: "quarantine", Status: StepOK,
		Detail: map[string]any{"bytes": action.File.Size}}, rollback, nil
}

// rollbackAll runs rollback functions in reverse order, appending one
// audit step per rollback attempt.
func (e *Executor) rollbackAll(result *Result, rollbacks *[]func() error) {
	for i := len(*rollbacks) - 1; i >= 0; i-- {
		if err := (*rollbacks)[i](); err != nil {
			result.Steps = append(result.Steps, AuditStep{
				Name: "rollback", Status: StepFailed,
				Detail: map[string]any{"error_type": "rollback_failed", "index": i},
			})
			continue
		}
		result.Steps = append(result.Steps, AuditStep{
			Name: "rollback", Status: StepOK,
			Detail: map[string]any{"index": i},
		})
	}
	*rollbacks = nil
}

// touchesFilesystem reports whether an action type writes to the user's
// filesystem. KEEP, SKIP and REVIEW are advisory-only and never touch files.
func touchesFilesystem(action domain.OperationType) bool {
	switch action {
	case domain.OperationKeep, domain.OperationSkip, domain.OperationReview:
		return false
	default:
		return true
	}
}
