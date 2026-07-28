package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// SaveGroupDecision inserts or replaces the review decision for a duplicate
// group. The upsert is keyed on group_id: if a decision already exists for
// the same group, it is replaced. This ensures the latest decision always
// wins without accumulating historical rows (the decision history is
// tracked via updated_at, not via multiple rows per group).
func (s *SQLiteStore) SaveGroupDecision(ctx context.Context, d domain.GroupDecision) error {
	if d.GroupID == "" {
		return fmt.Errorf("store: SaveGroupDecision: group_id is required")
	}
	if d.DecisionType == "" {
		return fmt.Errorf("store: SaveGroupDecision: decision_type is required")
	}
	switch d.DecisionType {
	case domain.DecisionKeepAll,
		domain.DecisionDraftAction,
		domain.DecisionDeferred,
		domain.DecisionRejectedSuggestion,
		domain.DecisionCrossArchive,
		domain.DecisionBackupRelation:
		if d.RetainedFileID != nil {
			return fmt.Errorf("store: SaveGroupDecision: retained_file_id is only valid for PRIMARY_RETENTION")
		}
	case domain.DecisionPrimaryRetention:
		if d.RetainedFileID == nil {
			return fmt.Errorf("store: SaveGroupDecision: PRIMARY_RETENTION requires retained_file_id")
		}
	default:
		return fmt.Errorf("store: SaveGroupDecision: unsupported decision_type %q", d.DecisionType)
	}

	// Auto-generate a deterministic ID if the caller did not provide one.
	// Format: group_id + "-" + decision_type. This is safe because the
	// upsert deletes the existing row for this group_id before inserting,
	// so there is never more than one row per group.
	if d.ID == "" {
		d.ID = d.GroupID + "-" + string(d.DecisionType)
	}

	now := time.Now().UTC()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = now
	}
	d.UpdatedAt = now

	var retainedID any
	if d.RetainedFileID != nil {
		retainedID = *d.RetainedFileID
	}

	// Upsert: delete existing row for this group_id, then insert.
	// This avoids accumulating multiple rows per group and ensures the
	// latest decision is always the one returned by GetGroupDecision.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: SaveGroupDecision begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM group_decisions WHERE group_id = ?`, d.GroupID); err != nil {
		return fmt.Errorf("store: SaveGroupDecision delete old: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO group_decisions(id, group_id, decision_type, retained_file_id, reason, rule_id, created_at, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.GroupID, string(d.DecisionType), retainedID,
		d.Reason, d.RuleID,
		d.CreatedAt.Format(time.RFC3339Nano),
		d.UpdatedAt.Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("store: SaveGroupDecision insert: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: SaveGroupDecision commit: %w", err)
	}
	return nil
}

// GetGroupDecision returns the latest review decision for a duplicate group.
// Returns ErrNotFound when no decision has been recorded.
func (s *SQLiteStore) GetGroupDecision(ctx context.Context, groupID string) (domain.GroupDecision, error) {
	var (
		id, scannedGroupID, decisionType, reason, ruleID, createdAt, updatedAt string
		retainedFileID                                                         sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, group_id, decision_type, retained_file_id, reason, rule_id, created_at, updated_at
		 FROM group_decisions
		 WHERE group_id = ?
		 ORDER BY updated_at DESC
		 LIMIT 1`, groupID).Scan(
		&id, &scannedGroupID, &decisionType, &retainedFileID, &reason, &ruleID,
		&createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return domain.GroupDecision{}, ErrNotFound
	}
	if err != nil {
		return domain.GroupDecision{}, fmt.Errorf("store: GetGroupDecision: %w", err)
	}

	d := domain.GroupDecision{
		ID:           id,
		GroupID:      scannedGroupID,
		DecisionType: domain.ReviewDecisionType(decisionType),
		Reason:       reason,
		RuleID:       ruleID,
	}
	if retainedFileID.Valid {
		v := retainedFileID.Int64
		d.RetainedFileID = &v
	}
	d.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	d.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return d, nil
}

// ListGroupDecisions returns review decisions, optionally filtered by type.
// Results are ordered by updated_at DESC (most recent first).
func (s *SQLiteStore) ListGroupDecisions(ctx context.Context, decisionType domain.ReviewDecisionType) ([]domain.GroupDecision, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if decisionType != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, group_id, decision_type, retained_file_id, reason, rule_id, created_at, updated_at
			 FROM group_decisions
			 WHERE decision_type = ?
			 ORDER BY updated_at DESC`, string(decisionType))
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, group_id, decision_type, retained_file_id, reason, rule_id, created_at, updated_at
			 FROM group_decisions
			 ORDER BY updated_at DESC`)
	}
	if err != nil {
		return nil, fmt.Errorf("store: ListGroupDecisions: %w", err)
	}
	defer rows.Close()

	var result []domain.GroupDecision
	for rows.Next() {
		var (
			id, groupID, dt, reason, ruleID, createdAt, updatedAt string
			retainedFileID                                        sql.NullInt64
		)
		if err := rows.Scan(&id, &groupID, &dt, &retainedFileID, &reason, &ruleID,
			&createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("store: ListGroupDecisions scan: %w", err)
		}
		d := domain.GroupDecision{
			ID:           id,
			GroupID:      groupID,
			DecisionType: domain.ReviewDecisionType(dt),
			Reason:       reason,
			RuleID:       ruleID,
		}
		if retainedFileID.Valid {
			v := retainedFileID.Int64
			d.RetainedFileID = &v
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		d.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		result = append(result, d)
	}
	return result, rows.Err()
}
