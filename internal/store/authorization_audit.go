package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// AuthorizationDecision is one durable record of a business-authorization
// decision. It is the audit half of the app-side AuthorizationGate: every
// decision — eligible or refused — is persisted before the caller may act,
// including refusals of plans that do not exist (a forged plan id probe is
// exactly the event worth recording, so this table carries no foreign key).
//
// Content discipline (AGENTS rule 6): opaque identifiers and stable reason
// codes only — no file paths, no content hashes, no sizes.
type AuthorizationDecision struct {
	Operation         string // PURGE / RESTORE / ...
	PlanID            string
	ItemID            string // empty when the plan could not be resolved
	Eligible          bool
	Reason            string // stable authz reason code; empty when eligible
	Detail            string // verifier detail text (no paths, no hashes)
	OwnershipVerified bool   // true when the decision ran under the caller's execution-lock scope
	DecidedAt         time.Time
}

// AppendAuthorizationDecision persists one authorization decision. It is
// write-scoped to this audit table only: the gate's decision itself is
// derived from read-only fact sources and is never modified here.
func (s *SQLiteStore) AppendAuthorizationDecision(ctx context.Context, d AuthorizationDecision) error {
	eligible := 0
	if d.Eligible {
		eligible = 1
	}
	ownership := 0
	if d.OwnershipVerified {
		ownership = 1
	}
	at := d.DecidedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO authorization_audit
		  (created_at, operation, plan_id, item_id, eligible, reason, detail, ownership_verified)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		formatTime(at), d.Operation, d.PlanID, d.ItemID,
		eligible, d.Reason, d.Detail, ownership)
	if err != nil {
		return fmt.Errorf("store: append authorization decision: %w", err)
	}
	return nil
}

// ListAuthorizationDecisions returns the most recent audit records, newest
// first. limit <= 0 means all. Read-only reporting path for the desktop
// governance UI; it never mutates authorization state.
func (s *SQLiteStore) ListAuthorizationDecisions(ctx context.Context, limit int) ([]AuthorizationDecision, error) {
	q := `SELECT created_at, operation, plan_id, item_id, eligible, reason, detail, ownership_verified
	      FROM authorization_audit ORDER BY id DESC`
	args := []any{}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list authorization decisions: %w", err)
	}
	defer rows.Close()
	var out []AuthorizationDecision
	for rows.Next() {
		var d AuthorizationDecision
		var createdAt string
		var eligible, ownership int
		if err := rows.Scan(&createdAt, &d.Operation, &d.PlanID, &d.ItemID,
			&eligible, &d.Reason, &d.Detail, &ownership); err != nil {
			return nil, fmt.Errorf("store: scan authorization decision: %w", err)
		}
		d.Eligible = eligible == 1
		d.OwnershipVerified = ownership == 1
		if ts := parseNullableTime(sql.NullString{String: createdAt, Valid: true}); ts != nil {
			d.DecidedAt = *ts
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
