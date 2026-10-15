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
// Its commit point is SafeRemove(stagingPath). Any failure after BeginPurge
// retains the active journal for manual reconciliation. Legacy path-only
// records cannot prove the authority needed for automatic rollback or commit.
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
	if err := ctx.Err(); err != nil {
		result.Err, result.ErrorType, result.Status = err, "cancelled", StepFailed
		return result
	}
	if err := ensurePrivateRealDirectory(stageDir); err != nil {
		result.Err, result.ErrorType, result.Status = errActionFailed, "staging_directory_failed", StepFailed
		result.FinalState = plan.State
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Err, result.ErrorType, result.Status = err, "cancelled", StepFailed
		return result
	}
	if err := os.Rename(item.QuarantinePath, stagePath); err != nil {
		result.Err, result.ErrorType, result.Status = errActionFailed, "stage_move_failed", StepFailed
		result.FinalState = plan.State
		return result
	}
	if err := e.store.MarkPurgeStaged(ctx, plan.ID); err != nil {
		result.Err, result.ErrorType, result.Status = fmt.Errorf("purge executor: journal staged failed"), "journal_staged_failed", StepFailed
		result.FinalState = plan.State
		return result
	}
	plan.State = domain.PurgeStaged
	result.FinalState = plan.State

	staged, err := Snapshot(stagePath, true)
	if err != nil || staged.Size != item.FileSize || staged.Hash != item.ContentSHA256 {
		result.Err, result.ErrorType, result.Status = errStaleDetected, "staged_verification_failed", StepFailed
		result.FinalState = plan.State
		return result
	}
	if err := e.store.MarkPurgeCommitPending(ctx, plan.ID); err != nil {
		result.Err, result.ErrorType, result.Status = fmt.Errorf("purge executor: commit journal failed"), "journal_commit_pending_failed", StepFailed
		result.FinalState = plan.State
		return result
	}

	// Legacy irreversible commit path, still disabled by release policy.
	// A full hash does not bind the object/mount across these path operations;
	// positive execution requires a separately proven descriptor-based design.
	if err := ctx.Err(); err != nil {
		result.Err, result.ErrorType, result.Status = err, "cancelled", StepFailed
		return result
	}
	if err := SafeRemove(stagePath); err != nil {
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

// RecoverPurges reads unresolved legacy journals without changing files or
// terminal state. These records contain paths and hashes, but no durable proof
// binding the mount/root, staged object, rollback or irreversible unlink. Missing
// names or matching bytes cannot supply that proof. Keep the recovery lock until
// a separately reviewed reconciliation protocol can establish authority.
func (e *PurgeExecutor) RecoverPurges(ctx context.Context) []PurgeResult {
	entries, err := e.store.ListRecoverablePurges(ctx)
	if err != nil {
		return []PurgeResult{{Status: StepFailed, ErrorType: "journal_read_failed", Err: errActionFailed}}
	}
	results := make([]PurgeResult, 0, len(entries))
	for _, entry := range entries {
		results = append(results, PurgeResult{
			PlanID:    entry.PlanID,
			Status:    StepFailed,
			ErrorType: "manual_reconciliation_required",
			Err:       errActionFailed,
		})
	}
	return results
}
