package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

const (
	QuarantinePurge   = "QUARANTINE_PURGE"
	SourceDirectPurge = "SOURCE_DIRECT_PURGE"
)

// DualPurgePlan is independent of legacy quarantine and ordinary operation
// plans. EvidenceJSON is worker-private and must not be returned as an API model.
type DualPurgePlan struct {
	ID, Operation, SubjectID, Digest, EvidenceJSON, State string
	ExpiresAt, UpdatedAt                                  time.Time
}

func (s *SQLiteStore) InsertDualPurgePlan(ctx context.Context, plan DualPurgePlan) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	_, err = tx.ExecContext(ctx, `INSERT INTO dual_purge_plans
		(id, operation, subject_id, digest, evidence_json, state, expires_at, updated_at)
		VALUES(?, ?, ?, ?, ?, 'DRAFT', ?, ?)`,
		plan.ID, plan.Operation, plan.SubjectID, plan.Digest, plan.EvidenceJSON, formatTime(plan.ExpiresAt), formatTime(plan.UpdatedAt))
	if err != nil {
		return err
	}
	if err := recordDualPurgeEvent(ctx, tx, plan, "DRAFT", plan.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) ReadDualPurgePlan(ctx context.Context, id string) (DualPurgePlan, error) {
	var p DualPurgePlan
	var expires, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id, operation, subject_id, digest, evidence_json, state, expires_at, updated_at
		FROM dual_purge_plans WHERE id = ?`, id).Scan(&p.ID, &p.Operation, &p.SubjectID, &p.Digest, &p.EvidenceJSON, &p.State, &expires, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	p.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		return p, ErrGovernanceRejected
	}
	p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return p, ErrGovernanceRejected
	}
	return p, nil
}

func dualPurgeBusy(ctx context.Context, r governanceReader) (bool, error) {
	var present bool
	if err := r.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name='dual_purge_plans')`).Scan(&present); err != nil || !present {
		return false, err
	}
	var busy bool
	err := r.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM dual_purge_plans WHERE state = 'EXECUTING')`).Scan(&busy)
	return busy, err
}

func (s *SQLiteStore) DualPurgeBusy(ctx context.Context) (bool, error) {
	return dualPurgeBusy(ctx, s.db)
}

func verifyQuarantineSubject(ctx context.Context, tx *sql.Tx, item domain.QuarantineItem, now time.Time) error {
	var matches int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM quarantine_items WHERE id = ? AND quarantine_path = ?
		AND content_sha256 = ? AND file_size = ? AND retain_until = ? AND retain_until <= ?
		AND status IN ('QUARANTINED','PURGE_ELIGIBLE') AND COALESCE(hold_reason,'') = ''`,
		item.ID, item.QuarantinePath, item.ContentSHA256, item.FileSize, formatTime(item.RetainUntil), formatTime(now)).Scan(&matches); err != nil || matches != 1 {
		return ErrGovernanceRejected
	}
	return nil
}

func (s *SQLiteStore) AdvanceQuarantinePurge(ctx context.Context, p DualPurgePlan, item domain.QuarantineItem, next string, now time.Time) error {
	allowed := p.State == "DRAFT" && next == "APPROVED" || p.State == "APPROVED" && next == "DRY_RUN" ||
		p.State == "DRY_RUN" && next == "EXECUTING" ||
		(p.State == "DRAFT" || p.State == "APPROVED" || p.State == "DRY_RUN") && next == "CANCELLED"
	if !allowed || p.Operation != QuarantinePurge || p.SubjectID != item.ID || !now.Before(p.ExpiresAt) {
		return ErrGovernanceRejected
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if next != "CANCELLED" {
		recovery, scan, err := governanceGuard(ctx, tx)
		if err != nil || recovery || scan {
			return ErrGovernanceRejected
		}
		if err := verifyQuarantineSubject(ctx, tx, item, now); err != nil {
			return err
		}
	}
	if !time.Now().UTC().Before(p.ExpiresAt) {
		return ErrGovernanceRejected
	}
	result, err := tx.ExecContext(ctx, `UPDATE dual_purge_plans SET state = ?, updated_at = ?
		WHERE id = ? AND operation = ? AND subject_id = ? AND digest = ? AND evidence_json = ? AND state = ? AND expires_at = ?`,
		next, formatTime(now), p.ID, p.Operation, p.SubjectID, p.Digest, p.EvidenceJSON, p.State, formatTime(p.ExpiresAt))
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return ErrGovernanceRejected
	}
	if err := recordDualPurgeEvent(ctx, tx, p, next, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) FinishQuarantinePurge(ctx context.Context, p DualPurgePlan, item domain.QuarantineItem, resultState string, now time.Time) error {
	if p.Operation != QuarantinePurge || p.SubjectID != item.ID || (resultState != "PURGED" && resultState != "ROLLED_BACK") {
		return ErrGovernanceRejected
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	result, err := tx.ExecContext(ctx, `UPDATE dual_purge_plans SET state = ?, updated_at = ?
		WHERE id = ? AND operation = ? AND subject_id = ? AND digest = ? AND evidence_json = ? AND state = 'EXECUTING'`,
		resultState, formatTime(now), p.ID, p.Operation, p.SubjectID, p.Digest, p.EvidenceJSON)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return ErrGovernanceRejected
	}
	if resultState == "PURGED" {
		if err := verifyQuarantineSubject(ctx, tx, item, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE quarantine_items SET status = 'PURGED', purged_at = ? WHERE id = ?`, formatTime(now), item.ID); err != nil {
			return err
		}
	}
	if err := recordDualPurgeEvent(ctx, tx, p, resultState, now); err != nil {
		return err
	}
	return tx.Commit()
}

// Only opaque identifiers and a fixed transition code enter the audit stream.
func recordDualPurgeEvent(ctx context.Context, tx *sql.Tx, plan DualPurgePlan, state string, at time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO authorization_audit
		(created_at,operation,plan_id,item_id,eligible,detail,ownership_verified)
		VALUES(?,?,?,?,1,?,1)`, formatTime(at), plan.Operation, plan.ID, plan.SubjectID, state)
	return err
}
