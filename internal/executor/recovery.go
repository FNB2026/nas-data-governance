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
	UpdatePlanState(ctx context.Context, planID string, state domain.PlanState) error
}

// RecoveryAction describes what Recover() did with a crashed plan.
type RecoveryAction string

const (
	RecoveryRolledBack      RecoveryAction = "rolled_back"       // done actions were undone
	RecoveryResetToApproved RecoveryAction = "reset_to_approved" // nothing was done, safe to re-run
	RecoverySkipped         RecoveryAction = "skipped"           // not in EXECUTING state
)

// RecoveryResult captures the outcome of recovering one plan.
type RecoveryResult struct {
	PlanID     string         `json:"plan_id"`
	Action     RecoveryAction `json:"action"`
	RolledBack int            `json:"rolled_back"`
	Errors     []string       `json:"errors,omitempty"`
}

// Recover scans for plans left in EXECUTING state (typically after a
// crash or power loss) and brings them to a safe, deterministic state.
//
// Policy (crash-conservative):
//
//   - If ANY filesystem action was marked done in the journal → undo all
//     done actions in reverse order, then transition the plan to
//     ROLLED_BACK. Auto-continuing partial execution is too risky
//     because the executor cannot know which action was about to run.
//
//   - If NO action was done (all pending or no journal entries) → reset
//     the plan to APPROVED so it can be re-executed from scratch. The
//     stale check will catch any files that disappeared.
//
// This method is safe to call repeatedly: it only touches plans in
// EXECUTING state, and journal updates are idempotent.
func (e *Executor) Recover(ctx context.Context, rs RecoveryStore) []RecoveryResult {
	planIDs, err := rs.ListExecutingPlans(ctx)
	if err != nil {
		return []RecoveryResult{{
			PlanID: "",
			Action: RecoverySkipped,
			Errors: []string{fmt.Sprintf("list executing plans: %v", err)},
		}}
	}

	results := make([]RecoveryResult, 0, len(planIDs))
	for _, planID := range planIDs {
		results = append(results, e.recoverPlan(ctx, rs, planID))
	}
	return results
}

func (e *Executor) recoverPlan(ctx context.Context, rs RecoveryStore, planID string) RecoveryResult {
	result := RecoveryResult{PlanID: planID}

	doneEntries, err := rs.ListJournalDone(ctx, planID)
	if err != nil {
		// Cannot read journal — cannot safely recover. Mark as
		// ROLLED_BACK so the plan is not silently re-executed.
		result.Action = RecoveryRolledBack
		result.Errors = []string{fmt.Sprintf("list journal done: %v", err)}
		_ = rs.UpdatePlanState(ctx, planID, domain.PlanRolledBack)
		return result
	}

	if len(doneEntries) == 0 {
		// No filesystem writes were confirmed done before the crash.
		// Safe to reset to APPROVED for re-execution.
		result.Action = RecoveryResetToApproved
		_ = rs.UpdatePlanState(ctx, planID, domain.PlanApproved)
		return result
	}

	// Some actions completed — undo them in reverse order.
	result.Action = RecoveryRolledBack
	for i := len(doneEntries) - 1; i >= 0; i-- {
		entry := doneEntries[i]
		rerr := rollbackJournalEntry(entry)
		// Mark the journal regardless of rollback success, so the
		// final state is visible to operators.
		_ = rs.MarkJournalRolledBack(ctx, planID, entry.ActionIndex, rerr)
		if rerr != nil {
			result.Errors = append(result.Errors,
				fmt.Sprintf("action %d (%s): %v", entry.ActionIndex, entry.ActionType, rerr))
			continue
		}
		result.RolledBack++
	}

	_ = rs.UpdatePlanState(ctx, planID, domain.PlanRolledBack)
	return result
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
		return SafeRemove(entry.TargetPath)
	default:
		// Unknown action type — nothing to undo.
		return nil
	}
}
