package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

var ErrPlanStateConflict = errors.New("store: plan state conflict or unresolved journal")

// CompareAndSwapPlanState never overwrites a newer approval or execution.
// A previously journaled plan cannot acquire a new execution reservation.
func (s *SQLiteStore) CompareAndSwapPlanState(ctx context.Context, id string, from, to domain.PlanState) error {
	q := `UPDATE operation_plans SET state=? WHERE id=? AND state=?`
	if from == domain.PlanApproved && to == domain.PlanStaleChecked {
		q += ` AND NOT EXISTS (SELECT 1 FROM execution_journal WHERE plan_id=operation_plans.id)`
	}
	if from == domain.PlanStaleChecked && to == domain.PlanApproved {
		q += ` AND NOT EXISTS (SELECT 1 FROM execution_journal WHERE plan_id=operation_plans.id)`
	}
	if to == domain.PlanRolledBack {
		q += ` AND NOT EXISTS (SELECT 1 FROM execution_journal WHERE plan_id=operation_plans.id AND (rollback_status IS NULL OR rollback_status <> 'done'))`
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback() //nolint:errcheck
	r, e := tx.ExecContext(ctx, q, string(to), id, string(from))
	if e != nil {
		return fmt.Errorf("store: conditional plan state: %w", e)
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return ErrPlanStateConflict
	}
	if to == domain.PlanDraft || to == domain.PlanRolledBack {
		errorType := "approval_invalidated"
		if from == domain.PlanApproved {
			errorType = "stale_detected"
		}
		event := "approval_invalidated"
		status := "failed"
		if to == domain.PlanRolledBack {
			status = "ok"
			errorType = "recovery_rolled_back"
			event = "recovery"
		}
		detail, _ := json.Marshal(map[string]any{"status": status, "final_state": string(to), "error_type": errorType})
		if _, e := tx.ExecContext(ctx, `INSERT INTO operation_logs(plan_id,event_type,detail_json,created_at) VALUES(?,?,?,?)`, id, event, string(detail), formatTime(time.Now().UTC())); e != nil {
			return e
		}
	}
	return tx.Commit()
}

// CompletePlanExecution commits terminal state, lifecycle items and audit
// together. Failure leaves the durable reservation and journal recoverable.
func (s *SQLiteStore) CompletePlanExecution(ctx context.Context, plan domain.OperationPlan, final domain.PlanState, logs []domain.OperationLog, at, retainUntil time.Time) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback() //nolint:errcheck
	if final != domain.PlanVerified && final != domain.PlanRolledBack {
		return ErrPlanStateConflict
	}
	r, e := tx.ExecContext(ctx, `UPDATE operation_plans SET state=? WHERE id=? AND state='EXECUTING'`, string(final), plan.ID)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return ErrPlanStateConflict
	}
	rows, e := tx.QueryContext(ctx, `SELECT plan_id, task_id, action_index, action_type, source_path, target_path, content_sha256, file_size, status, rollback_status, started_at, completed_at FROM execution_journal WHERE plan_id=? ORDER BY action_index`, plan.ID)
	if e != nil {
		return e
	}
	entries, e := scanJournalEntries(rows)
	if e != nil {
		return e
	}
	if final == domain.PlanRolledBack {
		for _, entry := range entries {
			if entry.RollbackStatus != "done" {
				return errors.New("store: rollback is not durably confirmed")
			}
		}
	}
	if final == domain.PlanVerified {
		want := 0
		for _, action := range plan.Actions {
			if journalTouchesFilesystem(action.Action) {
				want++
			}
		}
		if len(entries) != want {
			return errors.New("store: incomplete execution journal")
		}
		for _, entry := range entries {
			if entry.ActionIndex < 0 || entry.ActionIndex >= len(plan.Actions) {
				return errors.New("store: journal identity mismatch")
			}
			action := plan.Actions[entry.ActionIndex]
			if entry.TaskID != plan.TaskID || entry.ActionType != string(action.Action) || entry.SourcePath != action.Path || entry.ContentSHA256 != action.File.ContentSHA256 || entry.FileSize != action.File.Size {
				return errors.New("store: journal evidence mismatch")
			}
			if entry.Status != "done" || entry.RollbackStatus != "" {
				return errors.New("store: unresolved execution journal")
			}
		}
		if _, e = registerQuarantinesTx(ctx, tx, plan, entries, at, retainUntil); e != nil {
			return e
		}
	}
	for _, l := range logs {
		b, e := json.Marshal(l.Detail)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO operation_logs(plan_id,event_type,detail_json,created_at) VALUES(?,?,?,?)`, plan.ID, l.EventType, string(b), formatTime(at)); e != nil {
			return e
		}
	}
	return tx.Commit()
}
