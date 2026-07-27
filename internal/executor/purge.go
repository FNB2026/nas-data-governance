package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// PurgeStore is the durable private state required by the irreversible M7
// executor. The source-directory Executor never implements PURGE.
type PurgeStore interface {
	BeginPurge(context.Context, domain.PurgePlan, domain.QuarantineItem, string, time.Time) error
	MarkPurgeStaged(context.Context, string) error
	MarkPurgeCommitPending(context.Context, string) error
	MarkPurgeCommitted(context.Context, string, string, time.Time) error
	MarkPurgeRolledBack(context.Context, string, string, time.Time) error
	ListRecoverablePurges(context.Context) ([]domain.PurgeJournalEntry, error)
}

type PurgeResult struct {
	PlanID     string                `json:"plan_id"`
	FinalState domain.PurgePlanState `json:"final_state"`
	Status     StepStatus            `json:"status"`
	ErrorType  string                `json:"error_type,omitempty"`
	Err        error                 `json:"-"`
}

// PurgeExecutor permanently removes only expired managed quarantine items.
// Its commit point is SafeRemove(stagingPath). Before that point every failure
// attempts to rename the staged item back.
type PurgeExecutor struct {
	root  string
	store PurgeStore
	now   func() time.Time
}

func NewPurgeExecutor(root string, store PurgeStore) (*PurgeExecutor, error) {
	if store == nil {
		return nil, fmt.Errorf("purge executor: journal store is required")
	}
	if root == "" || !filepath.IsAbs(root) {
		return nil, fmt.Errorf("purge executor: absolute quarantine root is required")
	}
	root = filepath.Clean(root)
	if err := validateRealDirectory(root, "quarantine root"); err != nil {
		return nil, fmt.Errorf("purge executor: invalid quarantine root")
	}
	return &PurgeExecutor{root: root, store: store, now: time.Now}, nil
}

// ValidatePurge is a read-only stale and boundary check.
func (e *PurgeExecutor) ValidatePurge(ctx context.Context, plan domain.PurgePlan, item domain.QuarantineItem) PurgeResult {
	result := PurgeResult{PlanID: plan.ID, FinalState: plan.State, Status: StepFailed}
	if err := ctx.Err(); err != nil {
		result.Err, result.ErrorType = err, "cancelled"
		return result
	}
	if err := e.validatePurgeSnapshot(plan, item); err != nil {
		result.Err, result.ErrorType = err, "preflight_failed"
		return result
	}
	result.Status = StepOK
	return result
}

func (e *PurgeExecutor) ExecutePurge(ctx context.Context, plan *domain.PurgePlan, item *domain.QuarantineItem) PurgeResult {
	result := e.ValidatePurge(ctx, *plan, *item)
	if result.Err != nil {
		return result
	}
	if filepath.Base(plan.ID) != plan.ID {
		result.Err, result.ErrorType, result.Status = fmt.Errorf("purge executor: unsafe plan id"), "preflight_failed", StepFailed
		return result
	}
	stageDir := filepath.Join(e.root, ".purge-staging")
	stagePath := filepath.Join(stageDir, plan.ID+".stage")
	if err := notSymlinkBelowRoot(stagePath, e.root); err != nil {
		result.Err, result.ErrorType, result.Status = errActionFailed, "unsafe_staging_path", StepFailed
		return result
	}
	if _, err := os.Lstat(stagePath); !errors.Is(err, os.ErrNotExist) {
		result.Err, result.ErrorType, result.Status = fmt.Errorf("purge executor: staging path is not empty"), "staging_collision", StepFailed
		return result
	}
	started := e.now().UTC()
	if err := e.store.BeginPurge(ctx, *plan, *item, stagePath, started); err != nil {
		result.Err, result.ErrorType, result.Status = fmt.Errorf("purge executor: journal begin failed"), "journal_begin_failed", StepFailed
		return result
	}
	if err := ensurePrivateRealDirectory(stageDir); err != nil {
		_ = e.store.MarkPurgeRolledBack(ctx, plan.ID, item.ID, e.now().UTC())
		result.Err, result.ErrorType, result.Status = errActionFailed, "staging_directory_failed", StepFailed
		plan.State = domain.PurgeRolledBack
		result.FinalState = plan.State
		return result
	}
	if err := os.Rename(item.QuarantinePath, stagePath); err != nil {
		_ = e.store.MarkPurgeRolledBack(ctx, plan.ID, item.ID, e.now().UTC())
		result.Err, result.ErrorType, result.Status = errActionFailed, "stage_move_failed", StepFailed
		plan.State = domain.PurgeRolledBack
		result.FinalState = plan.State
		return result
	}
	if err := e.store.MarkPurgeStaged(ctx, plan.ID); err != nil {
		e.rollbackStaged(ctx, plan, item, stagePath)
		result.Err, result.ErrorType, result.Status = fmt.Errorf("purge executor: journal staged failed"), "journal_staged_failed", StepFailed
		result.FinalState = plan.State
		return result
	}
	plan.State = domain.PurgeStaged
	result.FinalState = plan.State

	staged, err := Snapshot(stagePath, true)
	if err != nil || staged.Size != item.FileSize || staged.Hash != item.ContentSHA256 {
		e.rollbackStaged(ctx, plan, item, stagePath)
		result.Err, result.ErrorType, result.Status = errStaleDetected, "staged_verification_failed", StepFailed
		result.FinalState = plan.State
		return result
	}
	if err := e.store.MarkPurgeCommitPending(ctx, plan.ID); err != nil {
		e.rollbackStaged(ctx, plan, item, stagePath)
		result.Err, result.ErrorType, result.Status = fmt.Errorf("purge executor: commit journal failed"), "journal_commit_pending_failed", StepFailed
		result.FinalState = plan.State
		return result
	}

	// Irreversible commit point. The file has already passed the second
	// full-hash check while isolated in the same-volume staging directory.
	if err := SafeRemove(stagePath); err != nil {
		e.rollbackStaged(ctx, plan, item, stagePath)
		result.Err, result.ErrorType, result.Status = errActionFailed, "purge_remove_failed", StepFailed
		result.FinalState = plan.State
		return result
	}
	committedAt := e.now().UTC()
	if err := e.store.MarkPurgeCommitted(ctx, plan.ID, item.ID, committedAt); err != nil {
		result.Err, result.ErrorType, result.Status = fmt.Errorf("purge executor: commit reconciliation required"), "journal_commit_failed", StepFailed
		return result
	}
	plan.State, item.Status, item.PurgedAt = domain.PurgeCommitted, domain.QuarantinePurged, &committedAt
	plan.PurgedAt = &committedAt
	result.FinalState, result.Status = plan.State, StepOK
	return result
}

func (e *PurgeExecutor) validatePurgeSnapshot(plan domain.PurgePlan, item domain.QuarantineItem) error {
	if plan.State != domain.PurgeApproved {
		return fmt.Errorf("purge executor: plan must be APPROVED")
	}
	if item.Status != domain.QuarantinePurgeEligible || item.HoldReason != "" {
		return fmt.Errorf("purge executor: item is not purge eligible")
	}
	if e.now().Before(item.RetainUntil) || e.now().Before(plan.RetainUntil) {
		return fmt.Errorf("purge executor: retention period has not expired")
	}
	if plan.ItemID != item.ID || plan.ExpectedPath != item.QuarantinePath ||
		plan.ExpectedSHA256 != item.ContentSHA256 || plan.ExpectedSize != item.FileSize ||
		!plan.RetainUntil.Equal(item.RetainUntil) {
		return errStaleDetected
	}
	if item.QuarantinePath == "" || !filepath.IsAbs(item.QuarantinePath) {
		return errOutOfScope
	}
	if _, ok := rootFor(item.QuarantinePath, []string{e.root}); !ok {
		return errOutOfScope
	}
	if err := notSymlinkBelowRoot(item.QuarantinePath, e.root); err != nil {
		return err
	}
	snapshot, err := Snapshot(item.QuarantinePath, true)
	if err != nil {
		return err
	}
	rootInfo, err := os.Stat(e.root)
	if err != nil {
		return err
	}
	rootDevice, _ := deviceAndInode(rootInfo)
	if rootDevice != 0 && snapshot.Device != 0 && rootDevice != snapshot.Device {
		return fmt.Errorf("purge executor: nested mount point is not allowed")
	}
	if snapshot.Size != item.FileSize || snapshot.Hash != item.ContentSHA256 {
		return errStaleDetected
	}
	return nil
}

func ensurePrivateRealDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrSymlinkRefused
	}
	return os.Chmod(path, 0o700)
}

func (e *PurgeExecutor) rollbackStaged(ctx context.Context, plan *domain.PurgePlan, item *domain.QuarantineItem, stagePath string) {
	if _, err := os.Lstat(stagePath); err == nil {
		if _, originalErr := os.Lstat(item.QuarantinePath); errors.Is(originalErr, os.ErrNotExist) {
			_ = os.Rename(stagePath, item.QuarantinePath)
		}
	}
	_ = e.store.MarkPurgeRolledBack(ctx, plan.ID, item.ID, e.now().UTC())
	plan.State = domain.PurgeRolledBack
}

// RecoverPurges reconciles crashes around the irreversible commit point.
// commit_pending + missing stage means unlink completed and is finalized;
// every other recoverable state is rolled back to the quarantine path.
func (e *PurgeExecutor) RecoverPurges(ctx context.Context) []PurgeResult {
	entries, err := e.store.ListRecoverablePurges(ctx)
	if err != nil {
		return []PurgeResult{{Status: StepFailed, ErrorType: "journal_read_failed", Err: err}}
	}
	results := make([]PurgeResult, 0, len(entries))
	for _, entry := range entries {
		result := PurgeResult{PlanID: entry.PlanID, Status: StepFailed}
		if _, ok := rootFor(entry.QuarantinePath, []string{e.root}); !ok {
			result.ErrorType, result.Err = "scope_validation_failed", errOutOfScope
			results = append(results, result)
			continue
		}
		if _, ok := rootFor(entry.StagingPath, []string{e.root}); !ok {
			result.ErrorType, result.Err = "scope_validation_failed", errOutOfScope
			results = append(results, result)
			continue
		}
		if err := notSymlinkBelowRoot(entry.StagingPath, e.root); err != nil {
			result.ErrorType, result.Err = "scope_validation_failed", errActionFailed
			results = append(results, result)
			continue
		}
		stageInfo, stageErr := os.Lstat(entry.StagingPath)
		_, originalErr := os.Lstat(entry.QuarantinePath)
		if stageErr != nil && !errors.Is(stageErr, os.ErrNotExist) {
			result.ErrorType, result.Err = "recovery_inspection_failed", errActionFailed
			results = append(results, result)
			continue
		}
		if originalErr != nil && !errors.Is(originalErr, os.ErrNotExist) {
			result.ErrorType, result.Err = "recovery_inspection_failed", errActionFailed
			results = append(results, result)
			continue
		}
		if stageErr == nil && stageInfo.Mode()&os.ModeSymlink != 0 {
			result.ErrorType, result.Err = "scope_validation_failed", errActionFailed
			results = append(results, result)
			continue
		}
		stageExists := stageErr == nil
		originalMissing := errors.Is(originalErr, os.ErrNotExist)
		if entry.Status == "commit_pending" && !stageExists && originalMissing {
			now := e.now().UTC()
			if err := e.store.MarkPurgeCommitted(ctx, entry.PlanID, entry.ItemID, now); err != nil {
				result.ErrorType, result.Err = "journal_commit_failed", err
			} else {
				result.Status, result.FinalState = StepOK, domain.PurgeCommitted
			}
			results = append(results, result)
			continue
		}
		if entry.Status == "commit_pending" && !originalMissing {
			// The staged object reached the irreversible boundary, but a path
			// now exists at the old quarantine name. It may have been recreated
			// after the crash; never infer authority from matching content.
			result.ErrorType, result.Err = "ambiguous_recovery_state", errActionFailed
			results = append(results, result)
			continue
		}
		if stageExists && originalMissing {
			if err := os.Rename(entry.StagingPath, entry.QuarantinePath); err != nil {
				result.ErrorType, result.Err = "rollback_failed", errActionFailed
				results = append(results, result)
				continue
			}
		} else if !stageExists && !originalMissing {
			// No filesystem mutation survived; only journal state needs reset.
		} else {
			result.ErrorType, result.Err = "ambiguous_recovery_state", errActionFailed
			results = append(results, result)
			continue
		}
		now := e.now().UTC()
		if err := e.store.MarkPurgeRolledBack(ctx, entry.PlanID, entry.ItemID, now); err != nil {
			result.ErrorType, result.Err = "journal_rollback_failed", err
		} else {
			result.Status, result.FinalState = StepOK, domain.PurgeRolledBack
		}
		results = append(results, result)
	}
	return results
}
