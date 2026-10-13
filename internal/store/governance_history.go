package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// ListGovernanceBatches is a pure query, including under a recovery lock and
// after readonly reopen. It never resumes, finalizes, or repairs a batch.
func (s *SQLiteStore) ListGovernanceBatches(ctx context.Context, q domain.GovernanceBatchQuery) (domain.GovernanceBatchPage, error) {
	v := domain.GovernanceBatchPage{Items: []domain.GovernanceBatchSummary{}}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.PageSize == 0 {
		q.PageSize = 20
	}
	if q.Page < 1 || q.Page > 1_000_000 || q.PageSize < 1 || q.PageSize > 50 || s.scopeBinding == "" {
		return v, ErrGovernanceRejected
	}
	v.Page, v.PageSize = q.Page, q.PageSize
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return v, ErrGovernanceRejected
	}
	defer tx.Rollback()
	var scope string
	if tx.QueryRowContext(ctx, `SELECT scope FROM governance_project WHERE id=1`).Scan(&scope) != nil || !validKey(scope) {
		return v, ErrGovernanceRejected
	}
	v.ProjectScope = scope + "-" + s.scopeBinding
	// Historical bindings of this same database remain readable after a move or
	// backup reopen, but a different project namespace is never enumerated.
	const predicate = `substr(b.scope,1,length(?)+1)=?||'-'`
	if tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM governance_batches b WHERE `+predicate, scope, scope).Scan(&v.TotalCount) != nil {
		return v, ErrGovernanceRejected
	}
	// Writers store UTC RFC3339Nano. Pad the fractional part for chronological
	// ordering: variable precision and a whole-second trailing Z cannot be
	// compared as raw text. Equal instants have a stable ID tie-breaker.
	rows, e := tx.QueryContext(ctx, `SELECT b.id,b.state,b.requested_count,b.created_at,b.scope,b.preview_token,
 COUNT(o.group_id),COALESCE(SUM(CASE WHEN o.status NOT IN ('SAVED','UNCHANGED','CONFLICT','BLOCKED','FAILED') THEN 1 ELSE 0 END),0)
 FROM governance_batches b LEFT JOIN governance_outcomes o ON o.batch_id=b.id
 WHERE `+predicate+` GROUP BY b.id
 ORDER BY substr(b.created_at,1,19) DESC,
 substr(replace(substr(b.created_at,21),'Z','')||'000000000',1,9) DESC,b.id DESC LIMIT ? OFFSET ?`, scope, scope, q.PageSize, (q.Page-1)*q.PageSize)
	if e != nil {
		return v, ErrGovernanceRejected
	}
	defer rows.Close()
	type binding struct{ scope, token string }
	bindings := []binding{}
	for rows.Next() {
		var item domain.GovernanceBatchSummary
		var invalid int
		var b binding
		if rows.Scan(&item.BatchID, &item.Status, &item.RequestedCount, &item.CreatedAt, &b.scope, &b.token, &item.RecordedCount, &invalid) != nil {
			return domain.GovernanceBatchPage{}, ErrGovernanceRejected
		}
		stamp, err := time.Parse(time.RFC3339Nano, item.CreatedAt)
		if !validKey(item.BatchID) || item.RequestedCount < 1 || item.RequestedCount > 100 || item.RecordedCount < 0 || item.RecordedCount > item.RequestedCount || invalid != 0 || err != nil || !strings.HasSuffix(item.CreatedAt, "Z") || stamp.Format(time.RFC3339Nano) != item.CreatedAt {
			return domain.GovernanceBatchPage{}, ErrGovernanceRejected
		}
		switch item.Status {
		case "COMPLETE":
			if item.RecordedCount != item.RequestedCount {
				return domain.GovernanceBatchPage{}, ErrGovernanceRejected
			}
		case "IN_PROGRESS", "CANCELLED":
		default:
			return domain.GovernanceBatchPage{}, ErrGovernanceRejected
		}
		v.Items = append(v.Items, item)
		bindings = append(bindings, b)
	}
	if rows.Err() != nil || rows.Close() != nil {
		return domain.GovernanceBatchPage{}, ErrGovernanceRejected
	}
	// All page summaries and their durable membership use the same read
	// transaction. A row count cannot prove the requested identities completed.
	for i, item := range v.Items {
		if validateHistoryMembership(ctx, tx, item, bindings[i].scope, bindings[i].token) != nil {
			return domain.GovernanceBatchPage{}, ErrGovernanceRejected
		}
	}
	if tx.Commit() != nil {
		return domain.GovernanceBatchPage{}, ErrGovernanceRejected
	}
	return v, nil
}

func validateHistoryMembership(ctx context.Context, tx *sql.Tx, item domain.GovernanceBatchSummary, scope, token string) error {
	var raw, payload string
	if !validKey(token) || tx.QueryRowContext(ctx, `SELECT preview_json,payload_json FROM governance_previews WHERE token=? AND scope=?`, token, scope).Scan(&raw, &payload) != nil || len(raw) > 131072 || len(payload) > 131072 {
		return ErrGovernanceRejected
	}
	var preview domain.GovernancePreview
	var input domain.PreviewBatchRequest
	if json.Unmarshal([]byte(raw), &preview) != nil || json.Unmarshal([]byte(payload), &input) != nil || preview.ProjectScope != scope || preview.PreviewToken != token || len(preview.Items) != item.RequestedCount || len(input.Items) != item.RequestedCount {
		return ErrGovernanceRejected
	}
	expected := map[string]bool{}
	for i, member := range preview.Items {
		if !validKey(member.GroupID) || expected[member.GroupID] || input.Items[i].GroupID != member.GroupID {
			return ErrGovernanceRejected
		}
		expected[member.GroupID] = true
	}
	rows, e := tx.QueryContext(ctx, `SELECT group_id,status,reason_code FROM governance_outcomes WHERE batch_id=?`, item.BatchID)
	if e != nil {
		return ErrGovernanceRejected
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var group, status, reason string
		if rows.Scan(&group, &status, &reason) != nil || !expected[group] || seen[group] || reason == "NOT_COMMITTED" {
			return ErrGovernanceRejected
		}
		switch status {
		case "SAVED", "UNCHANGED", "CONFLICT", "BLOCKED", "FAILED":
		default:
			return ErrGovernanceRejected
		}
		seen[group] = true
	}
	if rows.Err() != nil || rows.Close() != nil || len(seen) != item.RecordedCount || (item.Status == "COMPLETE" && len(seen) != len(expected)) {
		return ErrGovernanceRejected
	}
	return nil
}
