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
	"nas-data-governance/internal/store"
)

var errActionFailed = errors.New("executor: filesystem action failed")
var errOutOfScope = errors.New("executor: action is outside configured source roots")
var errStaleDetected = errors.New("executor: stale plan detected")

// Journal is the persistence port for crash-recovery logging. The executor
// writes one entry per filesystem action before/during/after execution; on
// restart, Recover() reads these entries to decide whether to continue or
// roll back. A nil journal disables crash recovery (backward compatible).
// store.SQLiteStore satisfies this interface structurally.
type Journal interface {
	BeginJournal(ctx context.Context, taskID, planID string, actions []domain.PlannedAction) error
	MarkJournalDone(ctx context.Context, planID string, actionIndex int, actualTargetPath string) error
	MarkJournalFailed(ctx context.Context, planID string, actionIndex int) error
	MarkJournalRolledBack(ctx context.Context, planID string, actionIndex int, rollbackErr error) error
	ListJournalDone(ctx context.Context, planID string) ([]store.JournalEntry, error)
	ListJournalPending(ctx context.Context, planID string) ([]store.JournalEntry, error)
	ListExecutingPlans(ctx context.Context) ([]string, error)
}

// rollbackEntry pairs a rollback function with its action index so the
// journal can be marked during rollback.
type rollbackEntry struct {
	actionIndex int
	fn          func() error
}

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
	ErrorType  string           `json:"error_type,omitempty"`
	// Err is non-nil when the pipeline failed. It never carries file
	// paths; callers can correlate via PlanID and the plan's actions.
	Err error `json:"-"`
}

// Executor runs approved plans through the safe-operation pipeline.
// It is the only component that writes to the user's file system.
type Executor struct {
	quarantine QuarantineConfig
	now        func() time.Time
	journal    Journal // optional; nil disables crash-recovery logging
}

// New creates an executor with the given quarantine config. `now` defaults
// to time.Now but can be overridden in tests for deterministic paths.
// The returned executor has no journal (backward compatible).
func New(q QuarantineConfig) (*Executor, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	return &Executor{quarantine: q, now: time.Now}, nil
}

// NewWithJournal creates an executor with crash-recovery journaling. Every
// filesystem action is recorded in the journal before execution; on
// success the actual target path is persisted; on failure/rollback the
// status is updated. If journal is nil, behaves like New (no journaling).
func NewWithJournal(q QuarantineConfig, journal Journal) (*Executor, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	return &Executor{quarantine: q, now: time.Now, journal: journal}, nil
}

// NewForRecovery creates an executor for crash recovery only. It does
// not require a quarantine config because Recover() uses journal-recorded
// paths (not the quarantine path generator) for rollback. Use this when
// you only need to call Recover() at startup.
func NewForRecovery() *Executor {
	return &Executor{now: time.Now}
}

// Validate performs the complete read-only preflight used by CLI --dry-run:
// approval state, source/target root boundaries, symlink checks, and the full
// stale check including SHA-256. It never changes plan state or writes files.
func (e *Executor) Validate(ctx context.Context, plan *domain.OperationPlan) Result {
	result := Result{PlanID: plan.ID, Steps: make([]AuditStep, 0), FinalState: plan.State}
	if plan.State != domain.PlanApproved {
		result.Err = fmt.Errorf("executor: plan state is %s, expected APPROVED", plan.State)
		result.ErrorType = "invalid_state"
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Err = err
		result.ErrorType = "cancelled"
		return result
	}
	if err := e.validatePlanScope(plan); err != nil {
		result.Err = err
		result.ErrorType = "scope_validation_failed"
		return result
	}
	stale, err := e.staleCheckAll(ctx, plan, &result)
	if err != nil {
		result.Err = errActionFailed
		result.ErrorType = "stale_check_error"
		return result
	}
	status := StepOK
	if stale > 0 {
		status = StepFailed
		result.Err = errStaleDetected
		result.ErrorType = "stale_detected"
	}
	result.Steps = append(result.Steps, AuditStep{
		Name: "dry_run", Status: status,
		Detail: map[string]any{"filesystem_writes": 0, "stale": stale},
	})
	return result
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
//
// When a Journal is configured, each filesystem action is recorded before
// execution (BeginJournal), marked done with its actual target path on
// success (MarkJournalDone), and marked rolled back during rollback
// (MarkJournalRolledBack). Journal errors are non-fatal: execution
// proceeds with the in-memory rollback chain as fallback.
func (e *Executor) Execute(ctx context.Context, plan *domain.OperationPlan) Result {
	result := Result{PlanID: plan.ID, Steps: make([]AuditStep, 0), FinalState: plan.State}

	if plan.State != domain.PlanApproved {
		result.Err = fmt.Errorf("executor: plan state is %s, expected APPROVED", plan.State)
		result.ErrorType = "invalid_state"
		return result
	}
	if err := e.validatePlanScope(plan); err != nil {
		result.Err = err
		result.ErrorType = "scope_validation_failed"
		return result
	}

	// Step 1: stale-check every filesystem-touching action.
	staleCount, err := e.staleCheckAll(ctx, plan, &result)
	if err != nil {
		result.Err = errActionFailed
		result.ErrorType = "stale_check_error"
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
		result.ErrorType = "state_transition_failed"
		return result
	}

	// Step 2: persist pending journal entries before any filesystem write.
	// This must happen BEFORE execution so a crash during execution leaves
	// records that Recover() can inspect.
	if e.journal != nil {
		if jerr := e.journal.BeginJournal(ctx, plan.TaskID, plan.ID, plan.Actions); jerr != nil {
			result.Steps = append(result.Steps, AuditStep{
				Name: "journal_begin", Status: StepFailed,
				Detail: map[string]any{"error_type": "journal_begin_failed"},
			})
			// Non-fatal: proceed with in-memory rollback only.
		}
	}

	// Step 3: execute actions in order.
	_ = Transition(plan, domain.PlanExecuting)
	result.FinalState = plan.State
	var rollbacks []rollbackEntry
	for i, action := range plan.Actions {
		if err := ctx.Err(); err != nil {
			result.Err = err
			result.ErrorType = "cancelled"
			e.rollbackAll(ctx, &result, plan.ID, &rollbacks)
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
		step, actualTarget, rollback, err := e.executeAction(ctx, action)
		result.Steps = append(result.Steps, step)
		if err != nil {
			if e.journal != nil {
				_ = e.journal.MarkJournalFailed(ctx, plan.ID, i)
			}
			result.Err = errActionFailed
			result.ErrorType = "action_failed"
			e.rollbackAll(ctx, &result, plan.ID, &rollbacks)
			_ = Transition(plan, domain.PlanRolledBack)
			result.FinalState = plan.State
			return result
		}
		if e.journal != nil {
			_ = e.journal.MarkJournalDone(ctx, plan.ID, i, actualTarget)
		}
		if rollback != nil {
			rollbacks = append(rollbacks, rollbackEntry{actionIndex: i, fn: rollback})
		}
	}

	// All actions succeeded.
	if err := Transition(plan, domain.PlanVerified); err != nil {
		result.Err = err
		result.ErrorType = "state_transition_failed"
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
// step, the actual target path (for journal recovery), an optional rollback
// function, and an error.
func (e *Executor) executeAction(ctx context.Context, action domain.PlannedAction) (AuditStep, string, func() error, error) {
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
			Detail: map[string]any{"reason": "non-filesystem action"}}, "", nil, nil
	default:
		return AuditStep{Name: string(action.Action), Status: StepFailed,
				Detail: map[string]any{"error_type": "not_implemented"}}, "", nil,
			fmt.Errorf("executor: action %s not implemented", action.Action)
	}
}

// executeQuarantine moves a source file into the quarantine area using
// the copy-verify-delete pipeline. The rollback hook moves the file back
// to its original path if a later action fails. Returns the actual
// quarantine destination path (may differ from nominal due to collision
// resolution).
func (e *Executor) executeQuarantine(action domain.PlannedAction) (AuditStep, string, func() error, error) {
	nominal := e.quarantine.PathFor(action.Path, e.now())
	if err := notSymlinkBelowRoot(nominal, e.quarantine.Root); err != nil {
		return AuditStep{Name: "quarantine", Status: StepFailed,
			Detail: map[string]any{"error_type": "unsafe_quarantine_path"}}, "", nil, err
	}
	dst, err := ResolveCollision(nominal)
	if err != nil {
		return AuditStep{Name: "quarantine", Status: StepFailed,
			Detail: map[string]any{"error_type": "collision_unresolved"}}, "", nil, err
	}
	if err := MoveFile(action.Path, dst, action.File.ContentSHA256); err != nil {
		return AuditStep{Name: "quarantine", Status: StepFailed,
			Detail: map[string]any{"error_type": "move_failed", "bytes": action.File.Size}}, "", nil, err
	}
	rollback := func() error {
		return MoveFile(dst, action.Path, action.File.ContentSHA256)
	}
	return AuditStep{Name: "quarantine", Status: StepOK,
		Detail: map[string]any{"bytes": action.File.Size}}, dst, rollback, nil
}

// executeMove moves a file to action.TargetPath using the copy-verify-delete
// pipeline (MoveFile). The rollback hook moves the file back to its original
// path. After the source is removed, the source directory is cleaned up if
// it became empty (best-effort, non-fatal).
func (e *Executor) executeMove(action domain.PlannedAction) (AuditStep, string, func() error, error) {
	if action.TargetPath == "" {
		return AuditStep{Name: "move", Status: StepFailed,
			Detail: map[string]any{"error_type": "missing_target"}}, "", nil, fmt.Errorf("executor: move requires target_path")
	}
	if err := MoveFile(action.Path, action.TargetPath, action.File.ContentSHA256); err != nil {
		return AuditStep{Name: "move", Status: StepFailed,
			Detail: map[string]any{"error_type": "move_failed", "bytes": action.File.Size}}, "", nil, err
	}
	e.maybeCleanupEmptyDir(filepath.Dir(action.Path))
	rollback := func() error {
		return MoveFile(action.TargetPath, action.Path, action.File.ContentSHA256)
	}
	return AuditStep{Name: "move", Status: StepOK,
		Detail: map[string]any{"bytes": action.File.Size}}, action.TargetPath, rollback, nil
}

// executeCopy copies a file to action.TargetPath and verifies integrity.
// The source is NOT removed — use MOVE for copy+delete. The rollback hook
// removes the copied destination.
func (e *Executor) executeCopy(action domain.PlannedAction) (AuditStep, string, func() error, error) {
	if action.TargetPath == "" {
		return AuditStep{Name: "copy", Status: StepFailed,
			Detail: map[string]any{"error_type": "missing_target"}}, "", nil, fmt.Errorf("executor: copy requires target_path")
	}
	if _, err := CopyAndVerify(action.Path, action.TargetPath, action.File.ContentSHA256); err != nil {
		return AuditStep{Name: "copy", Status: StepFailed,
			Detail: map[string]any{"error_type": "copy_or_verify_failed", "bytes": action.File.Size}}, "", nil, err
	}
	rollback := func() error {
		return SafeRemove(action.TargetPath)
	}
	return AuditStep{Name: "copy", Status: StepOK,
		Detail: map[string]any{"bytes": action.File.Size}}, action.TargetPath, rollback, nil
}

// executeDelete implements DELETE as quarantine, never as permanent removal.
// This keeps every destructive action recoverable as required by AGENTS rule
// 7 and the white paper. Permanent purge belongs to a separate retention
// workflow and is intentionally absent from this executor.
func (e *Executor) executeDelete(action domain.PlannedAction) (AuditStep, string, func() error, error) {
	step, actualTarget, rollback, err := e.executeQuarantine(action)
	step.Name = "delete_to_quarantine"
	if err != nil {
		return step, "", nil, err
	}
	e.maybeCleanupEmptyDir(filepath.Dir(action.Path))
	return step, actualTarget, rollback, nil
}

// executeRename uses the exclusive copy-verify-delete primitive. os.Rename
// cannot be used safely here because it overwrites an existing destination on
// Unix. The rollback hook restores the original path.
func (e *Executor) executeRename(action domain.PlannedAction) (AuditStep, string, func() error, error) {
	if action.TargetPath == "" {
		return AuditStep{Name: "rename", Status: StepFailed,
			Detail: map[string]any{"error_type": "missing_target"}}, "", nil, fmt.Errorf("executor: rename requires target_path")
	}
	if err := MoveFile(action.Path, action.TargetPath, action.File.ContentSHA256); err != nil {
		return AuditStep{Name: "rename", Status: StepFailed,
			Detail: map[string]any{"error_type": "rename_failed", "bytes": action.File.Size}}, "", nil, err
	}
	rollback := func() error {
		return MoveFile(action.TargetPath, action.Path, action.File.ContentSHA256)
	}
	return AuditStep{Name: "rename", Status: StepOK,
		Detail: map[string]any{"bytes": action.File.Size}}, action.TargetPath, rollback, nil
}

// maybeCleanupEmptyDir removes dir if it contains no entries. This is a
// best-effort cleanup after MOVE or DELETE — failures are silently ignored
// because an empty directory is harmless and can be cleaned up later.
func (e *Executor) maybeCleanupEmptyDir(dir string) {
	root, ok := rootFor(dir, e.quarantine.SourceRoots)
	if !ok || filepath.Clean(dir) == filepath.Clean(root) {
		return
	}
	if err := notSymlinkBelowRoot(dir, root); err != nil {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 0 {
		return
	}
	_ = os.Remove(dir) // best-effort; ignore error
}

// rollbackAll runs rollback functions in reverse order, appending one
// audit step per rollback attempt. When a journal is configured, each
// rollback is marked in the journal so Recover() can see the final state.
func (e *Executor) rollbackAll(ctx context.Context, result *Result, planID string, rollbacks *[]rollbackEntry) {
	for i := len(*rollbacks) - 1; i >= 0; i-- {
		entry := (*rollbacks)[i]
		err := entry.fn()
		if e.journal != nil {
			_ = e.journal.MarkJournalRolledBack(ctx, planID, entry.actionIndex, err)
		}
		if err != nil {
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
