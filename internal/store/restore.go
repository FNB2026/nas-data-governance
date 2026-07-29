package store

import (
	"context"
	"database/sql"
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
	var planState, itemStatus string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM restore_plans WHERE id = ?`, plan.ID).Scan(&planState); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT status FROM quarantine_items WHERE id = ?`, item.ID).Scan(&itemStatus); err != nil {
		return err
	}
	if planState != string(domain.RestoreApproved) || !restorableStatus(domain.QuarantineStatus(itemStatus)) {
		return fmt.Errorf("store: restore state changed before execution")
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

func restorableStatus(status domain.QuarantineStatus) bool {
	switch status {
	case domain.QuarantineActive, domain.QuarantineHold, domain.QuarantinePurgeEligible:
		return true
	default:
		return false
	}
}

func (s *SQLiteStore) MarkRestoreCompleted(ctx context.Context, planID, itemID string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx,
		`UPDATE restore_journal SET status = 'done', completed_at = ? WHERE plan_id = ?`,
		formatTime(at), planID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE restore_plans SET state = ?, restored_at = ? WHERE id = ?`,
		string(domain.RestoreCompleted), formatTime(at), planID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE quarantine_items SET status = ?, restored_at = ? WHERE id = ?`,
		string(domain.QuarantineRestored), formatTime(at), itemID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE purge_plans SET state = ?
		 WHERE item_id = ? AND state = ?`,
		string(domain.PurgeRolledBack), itemID, string(domain.PurgeDraft)); err != nil {
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
	if _, err := tx.ExecContext(ctx,
		`UPDATE restore_journal SET status = 'rolled_back', completed_at = ? WHERE plan_id = ?`,
		formatTime(at), planID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE restore_plans SET state = ? WHERE id = ?`,
		string(domain.RestoreRolledBack), planID); err != nil {
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
