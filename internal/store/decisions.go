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
	s.governanceMu.Lock()
	defer s.governanceMu.Unlock()
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
	if guardedGovernanceWrite(ctx, s, tx) != nil {
		return ErrGovernanceRejected
	}
	var old string
	var revision int64
	err = tx.QueryRowContext(ctx, `SELECT decision_type,revision FROM group_decisions WHERE group_id=? ORDER BY updated_at DESC,id DESC LIMIT 1`, d.GroupID).Scan(&old, &revision)
	if err != nil && err != sql.ErrNoRows {
		return ErrGovernanceRejected
	}
	if d.Revision != revision {
		return ErrGovernanceRejected
	}
	oldSnapshot, _, snapshotErr := decisionAuditSnapshot(ctx, tx, d.GroupID)
	if snapshotErr != nil {
		return snapshotErr
	}
	revision++

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM group_decisions WHERE group_id = ?`, d.GroupID); err != nil {
		return fmt.Errorf("store: SaveGroupDecision delete old: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO group_decisions(id, group_id, decision_type, retained_file_id, reason, rule_id, created_at, updated_at,revision)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.GroupID, string(d.DecisionType), retainedID,
		d.Reason, d.RuleID,
		d.CreatedAt.Format(time.RFC3339Nano),
		d.UpdatedAt.Format(time.RFC3339Nano),
		revision,
	); err != nil {
		return fmt.Errorf("store: SaveGroupDecision insert: %w", err)
	}
	if governanceAudit(ctx, tx, "", d.GroupID, oldSnapshot, string(d.DecisionType), "", revision, d.Reason, "legacy_store", "SAVED", "RECORDED_ONLY", now.Format(time.RFC3339Nano)) != nil {
		return ErrGovernanceRejected
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
		revision                                                               int64
	)
	expr, exprErr := decisionRevisionExpression(ctx, s.db)
	if exprErr != nil {
		return domain.GroupDecision{}, exprErr
	}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, group_id, decision_type, retained_file_id, COALESCE(reason,''), COALESCE(rule_id,''), created_at, updated_at,`+expr+`
		 FROM group_decisions
		 WHERE group_id = ?
		 ORDER BY `+decisionTimestampOrderFunction+`(updated_at) DESC, id DESC
		 LIMIT 1`, groupID).Scan(
		&id, &scannedGroupID, &decisionType, &retainedFileID, &reason, &ruleID,
		&createdAt, &updatedAt, &revision)
	if err == sql.ErrNoRows {
		return domain.GroupDecision{}, ErrNotFound
	}
	if err != nil {
		return domain.GroupDecision{}, fmt.Errorf("store: GetGroupDecision: %w", err)
	}

	d := domain.GroupDecision{
		Revision:     revision,
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
	d.CreatedAt, err = parseDecisionTimestamp(createdAt)
	if err != nil {
		return domain.GroupDecision{}, fmt.Errorf("store: GetGroupDecision created_at: %w", err)
	}
	d.UpdatedAt, err = parseDecisionTimestamp(updatedAt)
	if err != nil {
		return domain.GroupDecision{}, fmt.Errorf("store: GetGroupDecision updated_at: %w", err)
	}
	return d, nil
}

// ListGroupDecisions returns review decisions, optionally filtered by type.
// Results are ordered by parsed chronological updated_at DESC (most recent
// first), with ID as a stable tie-breaker for equal instants.
func (s *SQLiteStore) ListGroupDecisions(ctx context.Context, decisionType domain.ReviewDecisionType) ([]domain.GroupDecision, error) {
	expr, exprErr := decisionRevisionExpression(ctx, s.db)
	if exprErr != nil {
		return nil, exprErr
	}
	var (
		rows *sql.Rows
		err  error
	)
	if decisionType != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, group_id, decision_type, retained_file_id, COALESCE(reason,''), COALESCE(rule_id,''), created_at, updated_at,`+expr+`
			 FROM group_decisions
			 WHERE decision_type = ?
			 ORDER BY `+decisionTimestampOrderFunction+`(updated_at) DESC, id DESC`, string(decisionType))
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, group_id, decision_type, retained_file_id, COALESCE(reason,''), COALESCE(rule_id,''), created_at, updated_at,`+expr+`
			 FROM group_decisions
			 ORDER BY `+decisionTimestampOrderFunction+`(updated_at) DESC, id DESC`)
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
			revision                                              int64
		)
		if err := rows.Scan(&id, &groupID, &dt, &retainedFileID, &reason, &ruleID,
			&createdAt, &updatedAt, &revision); err != nil {
			return nil, fmt.Errorf("store: ListGroupDecisions scan: %w", err)
		}
		d := domain.GroupDecision{
			Revision:     revision,
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
		d.CreatedAt, err = parseDecisionTimestamp(createdAt)
		if err != nil {
			return nil, fmt.Errorf("store: ListGroupDecisions created_at: %w", err)
		}
		d.UpdatedAt, err = parseDecisionTimestamp(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("store: ListGroupDecisions updated_at: %w", err)
		}
		result = append(result, d)
	}
	return result, rows.Err()
}
