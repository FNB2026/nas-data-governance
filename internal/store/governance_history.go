package store

import (
	"context"
	"database/sql"
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
	rows, e := tx.QueryContext(ctx, `SELECT b.id,b.state,b.requested_count,b.created_at,
 COUNT(o.group_id),COALESCE(SUM(CASE WHEN o.status NOT IN ('SAVED','UNCHANGED','CONFLICT','BLOCKED','FAILED') THEN 1 ELSE 0 END),0)
 FROM governance_batches b LEFT JOIN governance_outcomes o ON o.batch_id=b.id
 WHERE `+predicate+` GROUP BY b.id
 ORDER BY substr(b.created_at,1,19) DESC,
 substr(replace(substr(b.created_at,21),'Z','')||'000000000',1,9) DESC,b.id DESC LIMIT ? OFFSET ?`, scope, scope, q.PageSize, (q.Page-1)*q.PageSize)
	if e != nil {
		return v, ErrGovernanceRejected
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.GovernanceBatchSummary
		var invalid int
		if rows.Scan(&item.BatchID, &item.Status, &item.RequestedCount, &item.CreatedAt, &item.RecordedCount, &invalid) != nil {
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
	}
	if rows.Err() != nil || rows.Close() != nil || tx.Commit() != nil {
		return domain.GovernanceBatchPage{}, ErrGovernanceRejected
	}
	return v, nil
}
