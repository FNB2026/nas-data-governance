package store

import (
	"context"
	"database/sql"
	"os"

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

// Reject unsupported layouts before the writable DSN enables WAL or any legacy
// migration runs. A common marker table does not establish NDG identity. Read a
// single SQLite snapshot including WAL; immutable=1 could conceal live markers.
// These hashes identify supported layouts, not database provenance or ownership.
func preflightGovernanceSchema(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err != nil {
		return ErrGovernanceRejected
	}
	// A closed WAL-mode database with no sidecars otherwise creates new WAL/SHM
	// files even through mode=ro. Only the owner-locked, sidecar-free case may
	// use immutable. Existing WAL must always remain visible. Incomplete or
	// nonregular sidecar pairs are ambiguous and refused without opening them.
	wal, walErr := os.Lstat(path + "-wal")
	shm, shmErr := os.Lstat(path + "-shm")
	dsn := "file:" + path + "?mode=ro&_pragma=busy_timeout(5000)"
	if os.IsNotExist(walErr) && os.IsNotExist(shmErr) {
		dsn += "&immutable=1"
	} else if walErr != nil || shmErr != nil || !wal.Mode().IsRegular() || !shm.Mode().IsRegular() {
		return ErrGovernanceRejected
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return ErrGovernanceRejected
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ErrGovernanceRejected
	}
	defer tx.Rollback()
	layout, err := schemaLayoutFingerprint(ctx, tx)
	if err != nil {
		return ErrGovernanceRejected
	}
	if layout == emptySchemaLayout {
		return nil // New disposable/project database, no application objects.
	}
	if layout != legacySchema11Layout && layout != governanceSchema12Layout {
		return ErrGovernanceRejected
	}
	var count, lowest, highest int
	if tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MIN(version),0),COALESCE(MAX(version),0) FROM schema_migrations`).Scan(&count, &lowest, &highest) != nil {
		return ErrGovernanceRejected
	}
	// All frozen beta.1–beta.11 initializers create the complete legacy layout
	// without populating schema_migrations. The additive governance increment
	// is the first to persist a marker, and commits it with the entire layout.
	if (layout == legacySchema11Layout && count == 0) ||
		(layout == governanceSchema12Layout && count == 1 && lowest == 12 && highest == 12) {
		return nil
	}
	return ErrGovernanceRejected
}
