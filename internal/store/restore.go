package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func (s *SQLiteStore) SaveRestorePlan(ctx context.Context, plan domain.RestorePlan) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO restore_plans
		  (id, item_id, state, quarantine_path, restore_path, expected_sha256,
		   expected_size, approval_digest, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		plan.ID, plan.ItemID, string(plan.State), plan.QuarantinePath,
		plan.RestorePath, plan.ExpectedSHA256, plan.ExpectedSize,
		plan.ApprovalDigest, formatTime(plan.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: save restore plan: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetRestorePlan(ctx context.Context, id string) (domain.RestorePlan, error) {
	var plan domain.RestorePlan
	var state, createdAt string
	var approvedAt, restoredAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, item_id, state, quarantine_path, restore_path, expected_sha256,
		       expected_size, approval_digest, created_at, approved_at, restored_at
		FROM restore_plans WHERE id = ?`, id).Scan(
		&plan.ID, &plan.ItemID, &state, &plan.QuarantinePath, &plan.RestorePath,
		&plan.ExpectedSHA256, &plan.ExpectedSize, &plan.ApprovalDigest,
		&createdAt, &approvedAt, &restoredAt)
	if err == sql.ErrNoRows {
		return plan, ErrNotFound
	}
	if err != nil {
		return plan, fmt.Errorf("store: get restore plan: %w", err)
	}
	plan.State = domain.RestorePlanState(state)
	plan.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	plan.ApprovedAt = parseNullableTime(approvedAt)
	plan.RestoredAt = parseNullableTime(restoredAt)
	return plan, nil
}

// ListRestorePlans returns durable restore plans so desktop workflows survive
// navigation and application restarts.
func (s *SQLiteStore) ListRestorePlans(ctx context.Context) ([]domain.RestorePlan, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, item_id, state, quarantine_path, restore_path, expected_sha256,
		       expected_size, approval_digest, created_at, approved_at, restored_at
		FROM restore_plans ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list restore plans: %w", err)
	}
	defer rows.Close()
	out := make([]domain.RestorePlan, 0)
	for rows.Next() {
		var plan domain.RestorePlan
		var state, createdAt string
		var approvedAt, restoredAt sql.NullString
		if err := rows.Scan(&plan.ID, &plan.ItemID, &state, &plan.QuarantinePath,
			&plan.RestorePath, &plan.ExpectedSHA256, &plan.ExpectedSize,
			&plan.ApprovalDigest, &createdAt, &approvedAt, &restoredAt); err != nil {
			return nil, err
		}
		plan.State = domain.RestorePlanState(state)
		plan.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		plan.ApprovedAt = parseNullableTime(approvedAt)
		plan.RestoredAt = parseNullableTime(restoredAt)
		out = append(out, plan)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ApproveRestorePlan(ctx context.Context, id, digest string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE restore_plans SET state = ?, approved_at = ?
		WHERE id = ? AND state = ? AND approval_digest = ?`,
		string(domain.RestoreApproved), formatTime(at), id, string(domain.RestoreDraft), digest)
	if err != nil {
		return fmt.Errorf("store: approve restore plan: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return fmt.Errorf("store: restore approval rejected")
	}
	return nil
}

func (s *SQLiteStore) BeginRestore(ctx context.Context, plan domain.RestorePlan, item domain.QuarantineItem, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin restore journal: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var identity int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM restore_plans p JOIN quarantine_items q ON q.id=p.item_id
	 WHERE p.id=? AND p.state='APPROVED' AND p.item_id=? AND p.quarantine_path=? AND p.restore_path=? AND p.expected_sha256=? AND p.expected_size=? AND p.approval_digest=?
	 AND q.quarantine_path=p.quarantine_path AND q.source_path=p.restore_path AND q.content_sha256=p.expected_sha256 AND q.file_size=p.expected_size
	 AND q.status IN ('QUARANTINED','HOLD','PURGE_ELIGIBLE')`, plan.ID, item.ID, item.QuarantinePath, item.SourcePath, item.ContentSHA256, item.FileSize, plan.ApprovalDigest).Scan(&identity); err != nil {
		return err
	}
	if identity != 1 || plan.State != domain.RestoreApproved || plan.ItemID != item.ID || plan.QuarantinePath != item.QuarantinePath || plan.RestorePath != item.SourcePath || plan.ExpectedSHA256 != item.ContentSHA256 || plan.ExpectedSize != item.FileSize {
		return errRestoreStateConflict
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM restore_journal WHERE item_id=? AND status='pending'`, item.ID).Scan(&pending); err != nil {
		return err
	}
	if pending != 0 {
		return errRestoreStateConflict
	}
	var approvedPurges int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM purge_plans
		WHERE item_id = ? AND state IN (?, ?)`,
		item.ID, string(domain.PurgeApproved), string(domain.PurgeStaged)).Scan(&approvedPurges); err != nil {
		return err
	}
	if approvedPurges > 0 {
		return fmt.Errorf("store: approved purge blocks restore")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO restore_journal
		  (plan_id, item_id, quarantine_path, restore_path, content_sha256,
		   file_size, status, started_at)
		VALUES (?, ?, ?, ?, ?, ?, 'pending', ?)`,
		plan.ID, item.ID, item.QuarantinePath, item.SourcePath,
		item.ContentSHA256, item.FileSize, formatTime(at)); err != nil {
		return fmt.Errorf("store: insert restore journal: %w", err)
	}
	return tx.Commit()
}

var errRestoreStateConflict = errors.New("store: restore state or identity conflict")

// A terminal restore consumes exactly one matching pending reservation.
// Journal, plan, lifecycle and sanitized audit commit together or not at all.
func restoreReservation(ctx context.Context, tx *sql.Tx, planID string) (itemID, parentPlanID string, err error) {
	err = tx.QueryRowContext(ctx, `SELECT j.item_id,q.plan_id FROM restore_journal j
 JOIN restore_plans p ON p.id=j.plan_id JOIN quarantine_items q ON q.id=j.item_id
 WHERE j.plan_id=? AND j.status='pending' AND p.state='APPROVED'
 AND p.item_id=j.item_id AND p.quarantine_path=j.quarantine_path AND p.restore_path=j.restore_path
 AND p.expected_sha256=j.content_sha256 AND p.expected_size=j.file_size
 AND q.quarantine_path=j.quarantine_path AND q.source_path=j.restore_path
 AND q.content_sha256=j.content_sha256 AND q.file_size=j.file_size
 AND q.status IN ('QUARANTINED','HOLD','PURGE_ELIGIBLE')`, planID).Scan(&itemID, &parentPlanID)
	if err == sql.ErrNoRows {
		err = errRestoreStateConflict
	}
	return
}

func restoreUpdateOne(ctx context.Context, tx *sql.Tx, query string, args ...any) error {
	r, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errRestoreStateConflict
	}
	return nil
}

func restoreAudit(ctx context.Context, tx *sql.Tx, parent, event, status, state, kind string, at time.Time) error {
	detail, err := json.Marshal(map[string]string{"status": status, "final_state": state, "error_type": kind})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO operation_logs(plan_id,event_type,detail_json,created_at) VALUES(?,?,?,?)`, parent, event, string(detail), formatTime(at))
	return err
}

func (s *SQLiteStore) MarkRestoreCompleted(ctx context.Context, planID, itemID string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	canonical, parent, err := restoreReservation(ctx, tx, planID)
	if err != nil {
		return err
	}
	if canonical != itemID {
		return errRestoreStateConflict
	}
	if err = restoreUpdateOne(ctx, tx, `UPDATE restore_journal SET status='done',completed_at=? WHERE plan_id=? AND status='pending'`, formatTime(at), planID); err != nil {
		return err
	}
	if err = restoreUpdateOne(ctx, tx, `UPDATE restore_plans SET state='RESTORED',restored_at=? WHERE id=? AND state='APPROVED'`, formatTime(at), planID); err != nil {
		return err
	}
	if err = restoreUpdateOne(ctx, tx, `UPDATE quarantine_items SET status='RESTORED',restored_at=? WHERE id=? AND status IN ('QUARANTINED','HOLD','PURGE_ELIGIBLE')`, formatTime(at), itemID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE purge_plans SET state=? WHERE item_id=? AND state=?`, string(domain.PurgeRolledBack), itemID, string(domain.PurgeDraft)); err != nil {
		return err
	}
	if err = restoreAudit(ctx, tx, parent, "restore_execution", "ok", string(domain.RestoreCompleted), "", at); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) MarkRestoreRolledBack(ctx context.Context, planID string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	_, parent, err := restoreReservation(ctx, tx, planID)
	if err != nil {
		return err
	}
	if err = restoreUpdateOne(ctx, tx, `UPDATE restore_journal SET status='rolled_back',completed_at=? WHERE plan_id=? AND status='pending'`, formatTime(at), planID); err != nil {
		return err
	}
	if err = restoreUpdateOne(ctx, tx, `UPDATE restore_plans SET state='ROLLED_BACK',approval_digest='',approved_at=NULL WHERE id=? AND state='APPROVED'`, planID); err != nil {
		return err
	}
	if err = restoreAudit(ctx, tx, parent, "restore_recovery", "ok", string(domain.RestoreRolledBack), "recovery_rolled_back", at); err != nil {
		return err
	}
	return tx.Commit()
}

// RecordRestoreBlocked records static error classes without file paths. It does
// not terminalize the reservation; an audit failure leaves pending unchanged.
func (s *SQLiteStore) RecordRestoreBlocked(ctx context.Context, planID, kind string, at time.Time) error {
	switch kind {
	case "cancelled", "scope_validation_failed", "recovery_evidence_unavailable", "manual_reconciliation_required", "journal_rollback_failed", "journal_complete_failed":
	default:
		return errRestoreStateConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	_, parent, err := restoreReservation(ctx, tx, planID)
	if err != nil {
		return err
	}
	if err = restoreAudit(ctx, tx, parent, "restore_recovery", "failed", string(domain.RestoreApproved), kind, at); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListPendingRestores(ctx context.Context) ([]domain.RestoreJournalEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT plan_id, item_id, quarantine_path, restore_path, content_sha256,
		       file_size, status, started_at, completed_at
		FROM restore_journal WHERE status = 'pending' ORDER BY started_at, plan_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list pending restores: %w", err)
	}
	defer rows.Close()
	out := make([]domain.RestoreJournalEntry, 0)
	for rows.Next() {
		var entry domain.RestoreJournalEntry
		var startedAt string
		var completedAt sql.NullString
		if err := rows.Scan(
			&entry.PlanID, &entry.ItemID, &entry.QuarantinePath, &entry.RestorePath,
			&entry.ContentSHA256, &entry.FileSize, &entry.Status, &startedAt,
			&completedAt,
		); err != nil {
			return nil, err
		}
		entry.StartedAt, _ = time.Parse(time.RFC3339Nano, startedAt)
		entry.CompletedAt = parseNullableTime(completedAt)
		out = append(out, entry)
	}
	return out, rows.Err()
}
