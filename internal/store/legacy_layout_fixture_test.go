// Frozen synthetic layout fixture from main 15328e0054c1c3e476dd756436ce52d6f3c50c1a.
// Its DDL source matches beta.1–beta.11; an isolated beta.11 initializer also
// produced the frozen fingerprint. This is a test fixture, not product import.
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/FNB2026/nas-data-governance/schemas"
)

// Init applies migrations inside a single connection. SQLite's "CREATE
// TABLE" without IF NOT EXISTS would error on re-run; the schema file
// already uses IF NOT EXISTS / PRAGMA so this is safe to call repeatedly.
func createSupportedLegacySchema11(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("store: acquire conn: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("store: enable foreign_keys: %w", err)
	}
	for i, m := range []string{schemas.Schema001, schemas.Schema002, schemas.Schema003, schemas.Schema004, schemas.Schema005, schemas.Schema006, schemas.Schema007, schemas.Schema008, schemas.Schema009} {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("store: migration %d failed: %w", i+1, err)
		}
	}
	// Migration 002: add columns to rules table. SQLite's ALTER TABLE ADD
	// COLUMN has no IF NOT EXISTS, so we check pragma table_info first to
	// stay idempotent across re-init.
	if err := addSupportedLegacyColumnsIfMissing(ctx, conn, "rules", []supportedLegacyColumnDef{
		{"source", "TEXT NOT NULL DEFAULT 'builtin'"},
		{"batch_id", "TEXT"},
		{"confidence", "REAL NOT NULL DEFAULT 1.0"},
		{"status", "TEXT NOT NULL DEFAULT 'approved'"},
		{"approved_at", "TEXT"},
		{"expires_at", "TEXT"},
	}); err != nil {
		return fmt.Errorf("store: migration 2 alter rules: %w", err)
	}
	// Migration 004: add status column to file_instances for tracking
	// missing (deleted) files during incremental scans. 'active' is the
	// default for all pre-existing rows.
	if err := addSupportedLegacyColumnsIfMissing(ctx, conn, "file_instances", []supportedLegacyColumnDef{
		{"file_status", "TEXT NOT NULL DEFAULT 'active'"},
		{"physical_reliable", "INTEGER NOT NULL DEFAULT 0"},
	}); err != nil {
		return fmt.Errorf("store: migration 4 alter file_instances: %w", err)
	}
	// Migration 011: bind a successful purge dry-run to the approved plan
	// digest. Real permanent deletion is rejected until these fields are set.
	if err := addSupportedLegacyColumnsIfMissing(ctx, conn, "purge_plans", []supportedLegacyColumnDef{
		{"dry_run_digest", "TEXT"},
		{"dry_run_verified_at", "TEXT"},
	}); err != nil {
		return fmt.Errorf("store: migration 11 alter purge_plans: %w", err)
	}
	// Migration 010: persist filesystem link_count (nlink) captured at
	// scan time so the desktop UI can display hardlink evidence per file.
	if err := addSupportedLegacyColumnsIfMissing(ctx, conn, "file_instances", []supportedLegacyColumnDef{
		{"link_count", "INTEGER NOT NULL DEFAULT 0"},
	}); err != nil {
		return fmt.Errorf("store: migration 10 alter file_instances: %w", err)
	}
	// Migration 007: denormalize directory_contexts query fields for fast
	// desktop filtering by role, protected flag, business anchor, authority
	// level, branch point and privacy level. context_json remains the full
	// evidence blob; these columns are for indexed query only.
	if err := addSupportedLegacyColumnsIfMissing(ctx, conn, "directory_contexts", []supportedLegacyColumnDef{
		{"role", "TEXT NOT NULL DEFAULT 'unknown'"},
		{"protected", "INTEGER NOT NULL DEFAULT 0"},
		{"business_anchor", "TEXT NOT NULL DEFAULT ''"},
		{"authority_level", "INTEGER NOT NULL DEFAULT 0"},
		{"branch_point", "TEXT NOT NULL DEFAULT ''"},
		{"privacy_level", "TEXT NOT NULL DEFAULT ''"},
	}); err != nil {
		return fmt.Errorf("store: migration 7 alter directory_contexts: %w", err)
	}
	// Migration 007 indexes: created after columns exist. CREATE INDEX IF
	// NOT EXISTS is idempotent, safe across re-init. These reference columns
	// added by Go code (role/protected/business_anchor via migration 7,
	// file_status via migration 4), so they cannot live in the .sql file
	// which executes before addSupportedLegacyColumnsIfMissing.
	_, err = conn.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS idx_dirctx_role
  ON directory_contexts(role);
CREATE INDEX IF NOT EXISTS idx_dirctx_protected
  ON directory_contexts(protected)
  WHERE protected = 1;
CREATE INDEX IF NOT EXISTS idx_dirctx_anchor
  ON directory_contexts(business_anchor)
  WHERE business_anchor <> '';
CREATE INDEX IF NOT EXISTS idx_files_active_content
  ON file_instances(storage_id, content_sha256, size)
  WHERE file_status = 'active'
    AND content_sha256 IS NOT NULL
    AND content_sha256 <> '';
`)
	if err != nil {
		return fmt.Errorf("store: migration 7 indexes: %w", err)
	}
	return nil
}

type supportedLegacyColumnDef struct {
	name string
	def  string
}

// addSupportedLegacyColumnsIfMissing checks existing columns via pragma table_info and only
// issues ALTER TABLE for missing ones. This makes re-init safe.
func addSupportedLegacyColumnsIfMissing(ctx context.Context, conn *sql.Conn, table string, cols []supportedLegacyColumnDef) error {
	existing := map[string]bool{}
	rows, err := conn.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("pragma table_info: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan table_info: %w", err)
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range cols {
		if existing[c.name] {
			continue
		}
		_, err := conn.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, c.name, c.def))
		if err != nil {
			return fmt.Errorf("add column %s: %w", c.name, err)
		}
	}
	return nil
}
