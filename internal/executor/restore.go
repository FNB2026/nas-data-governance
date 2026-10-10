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

type RestoreStore interface {
	BeginRestore(context.Context, domain.RestorePlan, domain.QuarantineItem, time.Time) error
	MarkRestoreCompleted(context.Context, string, string, time.Time) error
	MarkRestoreRolledBack(context.Context, string, time.Time) error
	ListPendingRestores(context.Context) ([]domain.RestoreJournalEntry, error)
}

type RestoreResult struct {
	PlanID     string                  `json:"plan_id"`
	FinalState domain.RestorePlanState `json:"final_state"`
	Status     StepStatus              `json:"status"`
	ErrorType  string                  `json:"error_type,omitempty"`
	Err        error                   `json:"-"`
}

// RestoreExecutor is separate from both source execution and permanent purge.
// It restores exactly one managed item and refuses destination overwrite.
type RestoreExecutor struct {
	quarantineRoot string
	sourceRoots    []string
	store          RestoreStore
	now            func() time.Time
}

func NewRestoreExecutor(quarantineRoot string, sourceRoots []string, store RestoreStore) (*RestoreExecutor, error) {
	if store == nil {
		return nil, fmt.Errorf("restore executor: journal store is required")
	}
	cfg := QuarantineConfig{
		Root: quarantineRoot, Structure: QuarantineFlat, SourceRoots: sourceRoots,
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("restore executor: invalid root configuration")
	}
	return &RestoreExecutor{
		quarantineRoot: filepath.Clean(quarantineRoot),
		sourceRoots:    sourceRoots, store: store, now: time.Now,
	}, nil
}

func (e *RestoreExecutor) ValidateRestore(ctx context.Context, plan domain.RestorePlan, item domain.QuarantineItem) RestoreResult {
	result := RestoreResult{PlanID: plan.ID, FinalState: plan.State, Status: StepFailed}
	if err := ctx.Err(); err != nil {
		result.Err, result.ErrorType = err, "cancelled"
		return result
	}
	if plan.State != domain.RestoreApproved {
		result.Err, result.ErrorType = fmt.Errorf("restore executor: plan must be APPROVED"), "invalid_state"
		return result
	}
	switch item.Status {
	case domain.QuarantineActive, domain.QuarantineHold, domain.QuarantinePurgeEligible:
	default:
		result.Err, result.ErrorType = fmt.Errorf("restore executor: item is not restorable"), "invalid_item_state"
		return result
	}
	if plan.ItemID != item.ID || plan.QuarantinePath != item.QuarantinePath ||
		plan.RestorePath != item.SourcePath || plan.ExpectedSHA256 != item.ContentSHA256 ||
		plan.ExpectedSize != item.FileSize {
		result.Err, result.ErrorType = errStaleDetected, "stale_detected"
		return result
	}
	if _, ok := rootFor(item.QuarantinePath, []string{e.quarantineRoot}); !ok {
		result.Err, result.ErrorType = errOutOfScope, "scope_validation_failed"
		return result
	}
	sourceRoot, ok := rootFor(item.SourcePath, e.sourceRoots)
	if !ok {
		result.Err, result.ErrorType = errOutOfScope, "scope_validation_failed"
		return result
	}
	if err := notSymlinkBelowRoot(item.QuarantinePath, e.quarantineRoot); err != nil {
		result.Err, result.ErrorType = err, "symlink_refused"
		return result
	}
	if err := notSymlinkBelowRoot(filepath.Dir(item.SourcePath), sourceRoot); err != nil {
		result.Err, result.ErrorType = err, "symlink_refused"
		return result
	}
	if _, err := os.Lstat(item.SourcePath); !errors.Is(err, os.ErrNotExist) {
		result.Err, result.ErrorType = fmt.Errorf("restore executor: destination exists"), "destination_exists"
		return result
	}
	snapshot, err := Snapshot(item.QuarantinePath, true)
	if err != nil || snapshot.Size != item.FileSize || snapshot.Hash != item.ContentSHA256 {
		result.Err, result.ErrorType = errStaleDetected, "stale_detected"
		return result
	}
	result.Status = StepOK
	return result
}

func (e *RestoreExecutor) ExecuteRestore(ctx context.Context, plan *domain.RestorePlan, item *domain.QuarantineItem) RestoreResult {
	result := e.ValidateRestore(ctx, *plan, *item)
	if result.Err != nil {
		return result
	}
	if err := e.store.BeginRestore(ctx, *plan, *item, e.now().UTC()); err != nil {
		result.Err, result.ErrorType, result.Status = fmt.Errorf("restore executor: journal begin failed"), "journal_begin_failed", StepFailed
		return result
	}
	if err := MoveFile(item.QuarantinePath, item.SourcePath, item.ContentSHA256); err != nil {
		result.Err, result.ErrorType, result.Status = errActionFailed, "restore_move_failed", StepFailed
		// A failed move is not proof of rollback. Preserve pending unless the
		// complete quarantine and confirmed absent destination are reverified.
		entry := domain.RestoreJournalEntry{PlanID: plan.ID, QuarantinePath: item.QuarantinePath, RestorePath: item.SourcePath, ContentSHA256: item.ContentSHA256, FileSize: item.FileSize}
		rollback := e.reconcileRestore(ctx, entry)
		if rollback.Status == StepOK {
			plan.State = domain.RestoreRolledBack
		} else {
			result.ErrorType = rollback.ErrorType
		}
		result.FinalState = plan.State
		return result
	}
	completedAt := e.now().UTC()
	if err := e.store.MarkRestoreCompleted(ctx, plan.ID, item.ID, completedAt); err != nil {
		// Completion persistence failed after the filesystem move. There is
		// no durable destination identity authorizing its deletion/move back.
		// Leave the journal pending and full destination intact for review.
		result.Err, result.ErrorType, result.Status = errActionFailed, "journal_complete_failed", StepFailed
		result.FinalState = plan.State
		if audit, ok := e.store.(interface {
			RecordRestoreBlocked(context.Context, string, string, time.Time) error
		}); ok {
			if audit.RecordRestoreBlocked(ctx, plan.ID, "journal_complete_failed", e.now().UTC()) != nil {
				result.ErrorType = "recovery_audit_failed"
			}
		}
		return result
	}
	plan.State, plan.RestoredAt = domain.RestoreCompleted, &completedAt
	item.Status, item.RestoredAt = domain.QuarantineRestored, &completedAt
	result.FinalState, result.Status = plan.State, StepOK
	return result
}

// RecoverRestores reconciles only a proven unchanged pre-operation state.
// Unknown output identity is never authority to delete or move either file.
func (e *RestoreExecutor) RecoverRestores(ctx context.Context) []RestoreResult {
	entries, err := e.store.ListPendingRestores(ctx)
	if err != nil {
		return []RestoreResult{{Status: StepFailed, ErrorType: "journal_read_failed", Err: errActionFailed}}
	}
	results := make([]RestoreResult, 0, len(entries))
	for _, entry := range entries {
		results = append(results, e.reconcileRestore(ctx, entry))
	}
	return results
}

func (e *RestoreExecutor) reconcileRestore(ctx context.Context, entry domain.RestoreJournalEntry) RestoreResult {
	result := RestoreResult{PlanID: entry.PlanID, FinalState: domain.RestoreApproved, Status: StepFailed}
	blocked := func(kind string) RestoreResult {
		result.ErrorType, result.Err = kind, errActionFailed
		if audit, ok := e.store.(interface {
			RecordRestoreBlocked(context.Context, string, string, time.Time) error
		}); ok {
			if err := audit.RecordRestoreBlocked(ctx, entry.PlanID, kind, e.now().UTC()); err != nil {
				result.ErrorType = "recovery_audit_failed"
			}
		}
		return result
	}
	if ctx.Err() != nil {
		return blocked("cancelled")
	}
	qRoot, qOK := rootFor(entry.QuarantinePath, []string{e.quarantineRoot})
	rRoot, rOK := rootFor(entry.RestorePath, e.sourceRoots)
	if !qOK || !rOK {
		return blocked("scope_validation_failed")
	}
	_, qMatches, qErr := probeRestorePath(qRoot, entry.QuarantinePath, entry.ContentSHA256, entry.FileSize)
	rExists, _, rErr := probeRestorePath(rRoot, entry.RestorePath, entry.ContentSHA256, entry.FileSize)
	if qErr != nil || rErr != nil {
		return blocked("recovery_evidence_unavailable")
	}
	if !qMatches || rExists {
		// Even a full matching destination may be an externally recreated
		// instance. The old journal contains no durable output inode identity.
		return blocked("manual_reconciliation_required")
	}
	// Recheck both live bindings immediately before terminal persistence.
	_, qMatches, qErr = probeRestorePath(qRoot, entry.QuarantinePath, entry.ContentSHA256, entry.FileSize)
	rExists, _, rErr = probeRestorePath(rRoot, entry.RestorePath, entry.ContentSHA256, entry.FileSize)
	if qErr != nil || rErr != nil || !qMatches || rExists {
		return blocked("recovery_evidence_unavailable")
	}
	if err := e.store.MarkRestoreRolledBack(ctx, entry.PlanID, e.now().UTC()); err != nil {
		return blocked("journal_rollback_failed")
	}
	result.Status, result.FinalState = StepOK, domain.RestoreRolledBack
	return result
}
