package store

import (
	"context"
	"database/sql"

	"github.com/FNB2026/nas-data-governance/schemas"
)

// This additive increment commits new tables, revision and marker together.
// Historical decisions/approvals are neither deleted nor remapped. The older
// initialization protocol is preserved and is not claimed globally atomic.
func applyGovernanceMigration(ctx context.Context, conn *sql.Conn) error {
	if _, e := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); e != nil {
		return ErrGovernanceRejected
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	var maxVersion int
	if conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&maxVersion) != nil || maxVersion > 12 {
		return ErrGovernanceRejected
	}
	if _, e := conn.ExecContext(ctx, schemas.Schema012); e != nil {
		return ErrGovernanceRejected
	}
	if e := addColumnsIfMissing(ctx, conn, "group_decisions", []columnDef{{"revision", "INTEGER NOT NULL DEFAULT 1"}}); e != nil {
		return ErrGovernanceRejected
	}
	required := map[string][]string{
		"governance_project": {"id", "scope"}, "governance_previews": {"token", "scope", "expires_at", "payload_json", "preview_json"},
		"governance_batches":        {"id", "scope", "idempotency_key", "request_digest", "preview_token", "requested_count", "state", "created_at"},
		"governance_outcomes":       {"batch_id", "group_id", "status", "reason_code", "decision_revision"},
		"governance_decision_audit": {"id", "batch_id", "group_id", "old_decision", "new_decision", "subject_version", "decision_revision", "reason", "source", "result", "reason_code", "actor", "created_at"},
	}
	for table, columns := range required {
		for _, column := range columns {
			var n int
			if conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`, table, column).Scan(&n) != nil || n != 1 {
				return ErrGovernanceRejected
			}
		}
	}
	if _, e := conn.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(12,strftime('%Y-%m-%dT%H:%M:%fZ','now'))`); e != nil {
		return ErrGovernanceRejected
	}
	if _, e := conn.ExecContext(ctx, `COMMIT`); e != nil {
		return ErrGovernanceRejected
	}
	committed = true
	return nil
}
