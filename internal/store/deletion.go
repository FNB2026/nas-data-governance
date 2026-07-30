package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// RegisterQuarantinesFromJournal materializes successful QUARANTINE/DELETE
// journal rows as managed M7 lifecycle items. It is idempotent by
// (plan_id, action_index). Protected contexts enter HOLD and can never become
// purge candidates until a separate future hold-release workflow exists.
func (s *SQLiteStore) RegisterQuarantinesFromJournal(
	ctx context.Context,
	planID string,
	quarantinedAt, retainUntil time.Time,
) ([]domain.QuarantineItem, error) {
	if !retainUntil.After(quarantinedAt) {
		return nil, fmt.Errorf("store: retain_until must be after quarantined_at")
	}
	plan, err := s.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	entries, err := s.ListJournalDone(ctx, planID)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: begin quarantine registration: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	registered := make([]domain.QuarantineItem, 0)
	for _, entry := range entries {
		actionType := domain.OperationType(entry.ActionType)
		if actionType != domain.OperationQuarantine && actionType != domain.OperationDelete {
			continue
		}
		if entry.TargetPath == "" || entry.ContentSHA256 == "" {
			return nil, fmt.Errorf("store: completed quarantine journal is incomplete")
		}
		status := domain.QuarantineActive
		holdReason := ""
		if entry.ActionIndex >= 0 && entry.ActionIndex < len(plan.Actions) {
			action := plan.Actions[entry.ActionIndex]
			if action.Context.Protected || protectedRole(action.Context.Role) {
				status = domain.QuarantineHold
				holdReason = "protected_context"
			}
		}
		id := quarantineItemID(planID, entry.ActionIndex, entry.TargetPath)
		item := domain.QuarantineItem{
			ID: id, PlanID: planID, ActionIndex: entry.ActionIndex,
			SourcePath: entry.SourcePath, QuarantinePath: entry.TargetPath,
			ContentSHA256: entry.ContentSHA256, FileSize: entry.FileSize,
			QuarantinedAt: quarantinedAt.UTC(), RetainUntil: retainUntil.UTC(),
			Status: status, HoldReason: holdReason,
		}
		result, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO quarantine_items
			  (id, plan_id, action_index, source_path, quarantine_path,
			   content_sha256, file_size, quarantined_at, retain_until, status, hold_reason)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			item.ID, item.PlanID, item.ActionIndex, item.SourcePath, item.QuarantinePath,
			item.ContentSHA256, item.FileSize, formatTime(item.QuarantinedAt),
			formatTime(item.RetainUntil), string(item.Status), nullIfEmpty(item.HoldReason))
		if err != nil {
			return nil, fmt.Errorf("store: register quarantine item: %w", err)
		}
		if n, _ := result.RowsAffected(); n > 0 {
			registered = append(registered, item)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit quarantine registration: %w", err)
	}
	return registered, nil
}

func protectedRole(role domain.DirectoryRole) bool {
	switch role {
	case domain.RoleSensitive, domain.RoleRaw, domain.RoleBackup, domain.RoleSystem:
		return true
	default:
		return false
	}
}

func quarantineItemID(planID string, actionIndex int, target string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%s", planID, actionIndex, target)))
	return fmt.Sprintf("q-%x", sum[:12])
}

// ListQuarantineItems lists private lifecycle records. Empty status means all.
func (s *SQLiteStore) ListQuarantineItems(ctx context.Context, status domain.QuarantineStatus) ([]domain.QuarantineItem, error) {
	query := `SELECT id, plan_id, action_index, source_path, quarantine_path,
	                 content_sha256, file_size, quarantined_at, retain_until,
	                 status, hold_reason, restored_at, purged_at
	          FROM quarantine_items`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, string(status))
	}
	query += ` ORDER BY quarantined_at, id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list quarantine items: %w", err)
	}
	defer rows.Close()
	items := make([]domain.QuarantineItem, 0)
	for rows.Next() {
		item, err := scanQuarantineItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLiteStore) GetQuarantineItem(ctx context.Context, id string) (domain.QuarantineItem, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, plan_id, action_index, source_path, quarantine_path,
		       content_sha256, file_size, quarantined_at, retain_until,
		       status, hold_reason, restored_at, purged_at
		FROM quarantine_items WHERE id = ?`, id)
	item, err := scanQuarantineItem(row)
	if err == sql.ErrNoRows {
		return item, ErrNotFound
	}
	return item, err
}

// ListPurgeCandidates returns expired, unprotected items without an active
// DRAFT/APPROVED/STAGED purge plan. Re-running `purge plan` is therefore
// idempotent from an operator's perspective.
func (s *SQLiteStore) ListPurgeCandidates(ctx context.Context, now time.Time) ([]domain.QuarantineItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT q.id, q.plan_id, q.action_index, q.source_path, q.quarantine_path,
		       q.content_sha256, q.file_size, q.quarantined_at, q.retain_until,
		       q.status, q.hold_reason, q.restored_at, q.purged_at
		FROM quarantine_items q
		WHERE q.status IN (?, ?)
		  AND q.hold_reason IS NULL
		  AND q.retain_until <= ?
		  AND NOT EXISTS (
		    SELECT 1 FROM purge_plans p
		    WHERE p.item_id = q.id AND p.state IN (?, ?, ?)
		  )
		ORDER BY q.retain_until, q.id`,
		string(domain.QuarantineActive), string(domain.QuarantinePurgeEligible),
		formatTime(now), string(domain.PurgeDraft), string(domain.PurgeApproved),
		string(domain.PurgeStaged))
	if err != nil {
		return nil, fmt.Errorf("store: list purge candidates: %w", err)
	}
	defer rows.Close()
	items := make([]domain.QuarantineItem, 0)
	for rows.Next() {
		item, err := scanQuarantineItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanQuarantineItem(row rowScanner) (domain.QuarantineItem, error) {
	var item domain.QuarantineItem
	var quarantinedAt, retainUntil string
	var status string
	var holdReason, restoredAt, purgedAt sql.NullString
	if err := row.Scan(
		&item.ID, &item.PlanID, &item.ActionIndex, &item.SourcePath,
		&item.QuarantinePath, &item.ContentSHA256, &item.FileSize,
		&quarantinedAt, &retainUntil, &status, &holdReason, &restoredAt, &purgedAt,
	); err != nil {
		return item, err
	}
	item.QuarantinedAt, _ = time.Parse(time.RFC3339Nano, quarantinedAt)
	item.RetainUntil, _ = time.Parse(time.RFC3339Nano, retainUntil)
	item.Status = domain.QuarantineStatus(status)
	item.HoldReason = holdReason.String
	item.RestoredAt = parseNullableTime(restoredAt)
	item.PurgedAt = parseNullableTime(purgedAt)
	return item, nil
}

// SavePurgePlans writes DRAFT plans and promotes their items to
// PURGE_ELIGIBLE in the same transaction. A HOLD item is never changed.
func (s *SQLiteStore) SavePurgePlans(ctx context.Context, plans []domain.PurgePlan) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin purge plans: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	for _, plan := range plans {
		var activePlans int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM purge_plans
			WHERE item_id = ? AND state IN (?, ?, ?)`,
			plan.ItemID, string(domain.PurgeDraft), string(domain.PurgeApproved),
			string(domain.PurgeStaged)).Scan(&activePlans); err != nil {
			return fmt.Errorf("store: inspect active purge plans: %w", err)
		}
		if activePlans > 0 {
			return fmt.Errorf("store: quarantine item already has an active purge plan")
		}
		result, err := tx.ExecContext(ctx, `
			UPDATE quarantine_items
			SET status = ?
			WHERE id = ? AND status IN (?, ?) AND hold_reason IS NULL AND retain_until <= ?`,
			string(domain.QuarantinePurgeEligible), plan.ItemID,
			string(domain.QuarantineActive), string(domain.QuarantinePurgeEligible),
			formatTime(plan.CreatedAt))
		if err != nil {
			return fmt.Errorf("store: mark purge eligible: %w", err)
		}
		if n, _ := result.RowsAffected(); n != 1 {
			return fmt.Errorf("store: quarantine item is not purge eligible")
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purge_plans
			  (id, item_id, state, expected_path, expected_sha256, expected_size,
			   retain_until, approval_digest, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			plan.ID, plan.ItemID, string(plan.State), plan.ExpectedPath,
			plan.ExpectedSHA256, plan.ExpectedSize, formatTime(plan.RetainUntil),
			plan.ApprovalDigest, formatTime(plan.CreatedAt)); err != nil {
			return fmt.Errorf("store: save purge plan: %w", err)
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) GetPurgePlan(ctx context.Context, id string) (domain.PurgePlan, error) {
	var plan domain.PurgePlan
	var state, retainUntil, createdAt string
	var approvedAt, dryRunDigest, dryRunVerifiedAt, purgedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
			SELECT id, item_id, state, expected_path, expected_sha256, expected_size,
			       retain_until, approval_digest, created_at, approved_at,
			       dry_run_digest, dry_run_verified_at, purged_at
			FROM purge_plans WHERE id = ?`, id).Scan(
		&plan.ID, &plan.ItemID, &state, &plan.ExpectedPath, &plan.ExpectedSHA256,
		&plan.ExpectedSize, &retainUntil, &plan.ApprovalDigest, &createdAt,
		&approvedAt, &dryRunDigest, &dryRunVerifiedAt, &purgedAt)
	if err == sql.ErrNoRows {
		return plan, ErrNotFound
	}
	if err != nil {
		return plan, fmt.Errorf("store: get purge plan: %w", err)
	}
	plan.State = domain.PurgePlanState(state)
	plan.RetainUntil, _ = time.Parse(time.RFC3339Nano, retainUntil)
	plan.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	plan.ApprovedAt = parseNullableTime(approvedAt)
	plan.DryRunDigest = dryRunDigest.String
	plan.DryRunVerifiedAt = parseNullableTime(dryRunVerifiedAt)
	plan.PurgedAt = parseNullableTime(purgedAt)
	return plan, nil
}

// ListPurgePlans returns durable purge plans so approved work remains visible
// after a desktop restart.
func (s *SQLiteStore) ListPurgePlans(ctx context.Context) ([]domain.PurgePlan, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, item_id, state, expected_path, expected_sha256, expected_size,
		       retain_until, approval_digest, created_at, approved_at,
		       dry_run_digest, dry_run_verified_at, purged_at
		FROM purge_plans ORDER BY created_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("store: list purge plans: %w", err)
	}
	defer rows.Close()
	out := make([]domain.PurgePlan, 0)
	for rows.Next() {
		var plan domain.PurgePlan
		var state, retainUntil, createdAt string
		var approvedAt, dryRunDigest, dryRunVerifiedAt, purgedAt sql.NullString
		if err := rows.Scan(&plan.ID, &plan.ItemID, &state, &plan.ExpectedPath,
			&plan.ExpectedSHA256, &plan.ExpectedSize, &retainUntil, &plan.ApprovalDigest,
			&createdAt, &approvedAt, &dryRunDigest, &dryRunVerifiedAt, &purgedAt); err != nil {
			return nil, err
		}
		plan.State = domain.PurgePlanState(state)
		plan.RetainUntil, _ = time.Parse(time.RFC3339Nano, retainUntil)
		plan.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		plan.ApprovedAt = parseNullableTime(approvedAt)
		plan.DryRunDigest = dryRunDigest.String
		plan.DryRunVerifiedAt = parseNullableTime(dryRunVerifiedAt)
		plan.PurgedAt = parseNullableTime(purgedAt)
		out = append(out, plan)
	}
	return out, rows.Err()
}

// MarkPurgeDryRunVerified persists the successful validation gate for the
// exact approved digest. A changed or recreated plan cannot reuse it.
func (s *SQLiteStore) MarkPurgeDryRunVerified(ctx context.Context, id, digest string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE purge_plans SET dry_run_digest = ?, dry_run_verified_at = ?
		WHERE id = ? AND state = ? AND approval_digest = ?`,
		digest, formatTime(at), id, string(domain.PurgeApproved), digest)
	if err != nil {
		return fmt.Errorf("store: mark purge dry-run: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return fmt.Errorf("store: purge dry-run gate rejected")
	}
	return nil
}

// ApprovePurgePlan is deliberately single-plan and digest-bound.
func (s *SQLiteStore) ApprovePurgePlan(ctx context.Context, id, digest string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE purge_plans SET state = ?, approved_at = ?
		WHERE id = ? AND state = ? AND approval_digest = ?`,
		string(domain.PurgeApproved), formatTime(at), id, string(domain.PurgeDraft), digest)
	if err != nil {
		return fmt.Errorf("store: approve purge plan: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return fmt.Errorf("store: purge approval rejected")
	}
	return nil
}

func (s *SQLiteStore) BeginPurge(ctx context.Context, plan domain.PurgePlan, item domain.QuarantineItem, stagingPath string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin purge journal: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	var planState, itemStatus string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM purge_plans WHERE id = ?`, plan.ID).Scan(&planState); err != nil {
		return fmt.Errorf("store: read purge plan state: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT status FROM quarantine_items WHERE id = ?`, item.ID).Scan(&itemStatus); err != nil {
		return fmt.Errorf("store: read quarantine item state: %w", err)
	}
	if planState != string(domain.PurgeApproved) || itemStatus != string(domain.QuarantinePurgeEligible) {
		return fmt.Errorf("store: purge state changed before execution")
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO purge_journal
		  (plan_id, item_id, quarantine_path, staging_path, content_sha256,
		   file_size, status, started_at)
		VALUES (?, ?, ?, ?, ?, ?, 'pending', ?)`,
		plan.ID, item.ID, item.QuarantinePath, stagingPath, item.ContentSHA256,
		item.FileSize, formatTime(at)); err != nil {
		return fmt.Errorf("store: insert purge journal: %w", err)
	}
	return tx.Commit()
}

func (s *SQLiteStore) MarkPurgeStaged(ctx context.Context, planID string) error {
	return s.updatePurgeJournalStatus(ctx, planID, "pending", "staged")
}

func (s *SQLiteStore) MarkPurgeCommitPending(ctx context.Context, planID string) error {
	return s.updatePurgeJournalStatus(ctx, planID, "staged", "commit_pending")
}

func (s *SQLiteStore) updatePurgeJournalStatus(ctx context.Context, planID, from, to string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE purge_journal SET status = ? WHERE plan_id = ? AND status = ?`,
		to, planID, from)
	if err != nil {
		return fmt.Errorf("store: update purge journal: %w", err)
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return fmt.Errorf("store: purge journal transition rejected")
	}
	return nil
}

// MarkPurgeCommitted atomically finalizes the private audit state after unlink.
func (s *SQLiteStore) MarkPurgeCommitted(ctx context.Context, planID, itemID string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin purge commit: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx,
		`UPDATE purge_journal SET status = 'committed', completed_at = ? WHERE plan_id = ?`,
		formatTime(at), planID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE purge_plans SET state = ?, purged_at = ? WHERE id = ?`,
		string(domain.PurgeCommitted), formatTime(at), planID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE quarantine_items SET status = ?, purged_at = ? WHERE id = ?`,
		string(domain.QuarantinePurged), formatTime(at), itemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) MarkPurgeRolledBack(ctx context.Context, planID, itemID string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.ExecContext(ctx,
		`UPDATE purge_journal SET status = 'rolled_back', completed_at = ? WHERE plan_id = ?`,
		formatTime(at), planID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE purge_plans SET state = ? WHERE id = ?`,
		string(domain.PurgeRolledBack), planID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE quarantine_items SET status = ? WHERE id = ? AND status <> ?`,
		string(domain.QuarantinePurgeEligible), itemID, string(domain.QuarantinePurged)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListRecoverablePurges(ctx context.Context) ([]domain.PurgeJournalEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT plan_id, item_id, quarantine_path, staging_path, content_sha256,
		       file_size, status, started_at, completed_at
		FROM purge_journal
		WHERE status IN ('pending', 'staged', 'commit_pending')
		ORDER BY started_at, plan_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list recoverable purges: %w", err)
	}
	defer rows.Close()
	out := make([]domain.PurgeJournalEntry, 0)
	for rows.Next() {
		var entry domain.PurgeJournalEntry
		var startedAt string
		var completedAt sql.NullString
		if err := rows.Scan(
			&entry.PlanID, &entry.ItemID, &entry.QuarantinePath, &entry.StagingPath,
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

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseNullableTime(v sql.NullString) *time.Time {
	if !v.Valid {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, v.String)
	if err != nil {
		return nil
	}
	return &t
}
