package executor

import (
	"context"
	"fmt"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// RecoveryStore is the store subset needed by Recover(). It extends
// Journal with plan lookup and state update so the executor can inspect
// crashed plans and persist their final state. store.SQLiteStore
// satisfies this interface structurally.
type RecoveryStore interface {
	Journal
	GetPlan(ctx context.Context, planID string) (domain.OperationPlan, error)
	CompareAndSwapPlanState(ctx context.Context, planID string, from, to domain.PlanState) error
	ListJournalAll(ctx context.Context, planID string) ([]store.JournalEntry, error)
	AcquireExecutionLock() (func(), error)
}

// RecoveryAction describes what Recover() did with a crashed plan.
type RecoveryAction string

const (
	RecoveryRolledBack      RecoveryAction = "rolled_back"       // done actions were undone
	RecoveryResetToDraft    RecoveryAction = "reset_to_draft"    // no journal, requires fresh review
	RecoveryResetToApproved RecoveryAction = "reset_to_approved" // legacy API value; never emitted
	RecoverySkipped         RecoveryAction = "skipped"           // not in EXECUTING state
)

// RecoveryResult captures the outcome of recovering one plan.
type RecoveryResult struct {
	PlanID     string         `json:"plan_id"`
	Action     RecoveryAction `json:"action"`
	RolledBack int            `json:"rolled_back"`
	Errors     []string       `json:"errors,omitempty"`
}

// Recover excludes live execution and keeps uncertain journal outcomes locked.
// Only a reservation with no journal can return to DRAFT; pending entries
// are not proof that no filesystem write occurred.
func (e *Executor) Recover(ctx context.Context, rs RecoveryStore) []RecoveryResult {
	unlock, err := rs.AcquireExecutionLock()
	if err != nil {
		return []RecoveryResult{{Action: RecoverySkipped, Errors: []string{"recovery: execution owner active or unavailable"}}}
	}
	defer unlock()
	ids, err := rs.ListExecutingPlans(ctx)
	if err != nil {
		return []RecoveryResult{{Action: RecoverySkipped, Errors: []string{"recovery: cannot read reserved plans"}}}
	}
	results := make([]RecoveryResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, e.recoverPlan(ctx, rs, id))
	}
	return results
}

func (e *Executor) recoverPlan(ctx context.Context, rs RecoveryStore, id string) RecoveryResult {
	r := RecoveryResult{PlanID: id, Action: RecoverySkipped}
	fail := func(message string) RecoveryResult { r.Errors = append(r.Errors, message); return r }
	p, err := rs.GetPlan(ctx, id)
	if err != nil {
		return fail("recovery: cannot read plan")
	}
	// Legacy APPROVED plus journal has ambiguous lifecycle registration.
	// Keep it blocked for explicit reconciliation instead of undoing a success.
	if p.State != domain.PlanExecuting && p.State != domain.PlanStaleChecked {
		return fail("recovery: legacy journal requires reconciliation")
	}
	entries, err := rs.ListJournalAll(ctx, id)
	if err != nil {
		return fail("recovery: cannot read journal")
	}
	if len(entries) == 0 {
		if err := rs.CompareAndSwapPlanState(ctx, id, p.State, domain.PlanDraft); err != nil {
			return fail("recovery: cannot persist review state")
		}
		r.Action = RecoveryResetToDraft
		return r
	}
	for _, entry := range entries {
		if entry.RollbackStatus == "done" {
			continue
		}
		if entry.Status != "done" || entry.RollbackStatus != "" {
			return fail("recovery: uncertain action outcome; manual reconciliation required")
		}
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if entry.RollbackStatus == "done" {
			continue
		}
		rollbackErr := rollbackJournalEntry(entry)
		persistErr := rs.MarkJournalRolledBack(ctx, id, entry.ActionIndex, rollbackErr)
		if rollbackErr != nil || persistErr != nil {
			return fail("recovery: rollback not durably confirmed")
		}
		r.RolledBack++
	}
	if err := rs.CompareAndSwapPlanState(ctx, id, p.State, domain.PlanRolledBack); err != nil {
		return fail("recovery: cannot persist rollback state")
	}
	r.Action = RecoveryRolledBack
	return r
}

// rollbackJournalEntry undoes one completed action using the journal's
// recorded paths. The operations mirror the executeAction handlers:
//
//   - MOVE / RENAME / QUARANTINE / DELETE: move the file from target
//     back to source, verifying the content hash.
//   - COPY: remove the copied destination (source was never moved).
//
// Non-filesystem actions (KEEP / SKIP / REVIEW) never appear in the
// journal, so they are not handled here.
func rollbackJournalEntry(entry store.JournalEntry) error {
	switch entry.ActionType {
	case string(domain.OperationMove),
		string(domain.OperationRename),
		string(domain.OperationQuarantine),
		string(domain.OperationDelete):
		if entry.TargetPath == "" {
			return fmt.Errorf("recovery: done entry has empty target_path (action %d)", entry.ActionIndex)
		}
		return MoveFile(entry.TargetPath, entry.SourcePath, entry.ContentSHA256)
	case string(domain.OperationCopy):
		if entry.TargetPath == "" {
			return fmt.Errorf("recovery: done copy entry has empty target_path (action %d)", entry.ActionIndex)
		}
		snapshot, err := Snapshot(entry.TargetPath, true)
		if err != nil || snapshot.Hash != entry.ContentSHA256 || snapshot.Size != entry.FileSize {
			return fmt.Errorf("recovery: copy target changed; manual reconciliation required")
		}
		return SafeRemove(entry.TargetPath)
	default:
		return fmt.Errorf("recovery: unsupported journal action")
	}
}
