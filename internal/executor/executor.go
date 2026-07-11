package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nas-data-governance/internal/domain"
)

var errActionFailed = errors.New("executor: filesystem action failed")
var errOutOfScope = errors.New("executor: action is outside configured source roots")

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
	if err := e.validatePlanScope(plan); err != nil {
		result.Err = err
		return result
	}

	// Step 1: stale-check every filesystem-touching action.
	staleCount, err := e.staleCheckAll(ctx, plan, &result)
	if err != nil {
		result.Err = errActionFailed
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
			result.Err = errActionFailed
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

func (e *Executor) validatePlanScope(plan *domain.OperationPlan) error {
	for _, action := range plan.Actions {
		if !touchesFilesystem(action.Action) {
			continue
		}
		if action.Path == "" || !filepath.IsAbs(action.Path) || action.File.Path != action.Path || action.File.ContentSHA256 == "" {
			return errOutOfScope
		}
		root, ok := rootFor(action.Path, e.quarantine.SourceRoots)
		if !ok {
			return errOutOfScope
		}
		if err := notSymlinkBelowRoot(action.Path, root); err != nil {
			return errActionFailed
		}
		// MOVE, COPY, and RENAME carry a target path that must also be
		// within a configured source root — the executor never writes to
		// paths outside the approved task area. QUARANTINE targets are
		// validated separately in executeQuarantine via the quarantine root.
		if action.TargetPath != "" {
			if !filepath.IsAbs(action.TargetPath) {
				return errOutOfScope
			}
			tRoot, ok := rootFor(action.TargetPath, e.quarantine.SourceRoots)
			if !ok {
				return errOutOfScope
			}
			if err := notSymlinkBelowRoot(action.TargetPath, tRoot); err != nil {
				return errActionFailed
			}
		}
	}
	return nil
}

func withinAnyRoot(path string, roots []string) bool {
	_, ok := rootFor(path, roots)
	return ok
}

func rootFor(path string, roots []string) (string, bool) {
	for _, root := range roots {
		rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return filepath.Clean(root), true
		}
	}
	return "", false
}

// notSymlinkBelowRoot checks every existing component below an explicitly
// approved task root. It deliberately does not inspect ancestors of root:
// platforms may expose safe system aliases there (for example /var on macOS).
func notSymlinkBelowRoot(path, root string) error {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return errOutOfScope
	}
	current := filepath.Clean(root)
	if rel == "." {
		return nil
	}
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrSymlinkRefused
		}
	}
	return nil
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
			return 0, errActionFailed
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
	case domain.OperationMove:
		return e.executeMove(action)
	case domain.OperationCopy:
		return e.executeCopy(action)
	case domain.OperationDelete:
		return e.executeDelete(action)
	case domain.OperationRename:
		return e.executeRename(action)
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
	if err := notSymlinkBelowRoot(nominal, e.quarantine.Root); err != nil {
		return AuditStep{Name: "quarantine", Status: StepFailed,
			Detail: map[string]any{"error_type": "unsafe_quarantine_path"}}, nil, err
	}
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

// executeMove moves a file to action.TargetPath using the copy-verify-delete
// pipeline (MoveFile). The rollback hook moves the file back to its original
// path. After the source is removed, the source directory is cleaned up if
// it became empty (best-effort, non-fatal).
func (e *Executor) executeMove(action domain.PlannedAction) (AuditStep, func() error, error) {
	if action.TargetPath == "" {
		return AuditStep{Name: "move", Status: StepFailed,
			Detail: map[string]any{"error_type": "missing_target"}}, nil, fmt.Errorf("executor: move requires target_path")
	}
	if err := MoveFile(action.Path, action.TargetPath, action.File.ContentSHA256); err != nil {
		return AuditStep{Name: "move", Status: StepFailed,
			Detail: map[string]any{"error_type": "move_failed", "bytes": action.File.Size}}, nil, err
	}
	maybeCleanupEmptyDir(filepath.Dir(action.Path))
	rollback := func() error {
		return MoveFile(action.TargetPath, action.Path, action.File.ContentSHA256)
	}
	return AuditStep{Name: "move", Status: StepOK,
		Detail: map[string]any{"bytes": action.File.Size}}, rollback, nil
}

// executeCopy copies a file to action.TargetPath and verifies integrity.
// The source is NOT removed — use MOVE for copy+delete. The rollback hook
// removes the copied destination.
func (e *Executor) executeCopy(action domain.PlannedAction) (AuditStep, func() error, error) {
	if action.TargetPath == "" {
		return AuditStep{Name: "copy", Status: StepFailed,
			Detail: map[string]any{"error_type": "missing_target"}}, nil, fmt.Errorf("executor: copy requires target_path")
	}
	if _, err := CopyFile(action.Path, action.TargetPath); err != nil {
		return AuditStep{Name: "copy", Status: StepFailed,
			Detail: map[string]any{"error_type": "copy_failed", "bytes": action.File.Size}}, nil, err
	}
	if err := VerifyFile(action.TargetPath, action.File.ContentSHA256); err != nil {
		return AuditStep{Name: "copy", Status: StepFailed,
			Detail: map[string]any{"error_type": "verify_failed", "bytes": action.File.Size}}, nil, err
	}
	rollback := func() error {
		return SafeRemove(action.TargetPath)
	}
	return AuditStep{Name: "copy", Status: StepOK,
		Detail: map[string]any{"bytes": action.File.Size}}, rollback, nil
}

// executeDelete permanently removes a file. DELETE is irreversible — it
// has no rollback hook. DELETE should only be used for files whose content
// has been verified to exist elsewhere (e.g., after a successful COPY or
// MOVE). The stale check has already confirmed the file hasn't changed.
func (e *Executor) executeDelete(action domain.PlannedAction) (AuditStep, func() error, error) {
	if err := SafeRemove(action.Path); err != nil {
		return AuditStep{Name: "delete", Status: StepFailed,
			Detail: map[string]any{"error_type": "delete_failed", "bytes": action.File.Size}}, nil, err
	}
	maybeCleanupEmptyDir(filepath.Dir(action.Path))
	// No rollback: deletion is irreversible by design.
	return AuditStep{Name: "delete", Status: StepOK,
		Detail: map[string]any{"bytes": action.File.Size}}, nil, nil
}

// executeRename renames a file within the same directory (same-volume
// rename). It uses os.Rename for atomicity, then verifies the destination
// hash. The rollback hook renames back.
func (e *Executor) executeRename(action domain.PlannedAction) (AuditStep, func() error, error) {
	if action.TargetPath == "" {
		return AuditStep{Name: "rename", Status: StepFailed,
			Detail: map[string]any{"error_type": "missing_target"}}, nil, fmt.Errorf("executor: rename requires target_path")
	}
	if err := notSymlink(action.Path); err != nil {
		return AuditStep{Name: "rename", Status: StepFailed,
			Detail: map[string]any{"error_type": "symlink_refused"}}, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(action.TargetPath), 0o755); err != nil {
		return AuditStep{Name: "rename", Status: StepFailed,
			Detail: map[string]any{"error_type": "mkdir_failed"}}, nil, err
	}
	if err := os.Rename(action.Path, action.TargetPath); err != nil {
		return AuditStep{Name: "rename", Status: StepFailed,
			Detail: map[string]any{"error_type": "rename_failed", "bytes": action.File.Size}}, nil, err
	}
	if err := VerifyFile(action.TargetPath, action.File.ContentSHA256); err != nil {
		// Rename succeeded but verify failed: rename back to restore.
		_ = os.Rename(action.TargetPath, action.Path)
		return AuditStep{Name: "rename", Status: StepFailed,
			Detail: map[string]any{"error_type": "verify_failed", "bytes": action.File.Size}}, nil, err
	}
	rollback := func() error {
		return os.Rename(action.TargetPath, action.Path)
	}
	return AuditStep{Name: "rename", Status: StepOK,
		Detail: map[string]any{"bytes": action.File.Size}}, rollback, nil
}

// maybeCleanupEmptyDir removes dir if it contains no entries. This is a
// best-effort cleanup after MOVE or DELETE — failures are silently ignored
// because an empty directory is harmless and can be cleaned up later.
func maybeCleanupEmptyDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 0 {
		return
	}
	_ = os.Remove(dir) // best-effort; ignore error
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
