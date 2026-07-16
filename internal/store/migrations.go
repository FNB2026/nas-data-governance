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
func (s *SQLiteStore) Init(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("store: acquire conn: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("store: enable foreign_keys: %w", err)
	}
	for i, m := range schemas.All() {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("store: migration %d failed: %w", i+1, err)
		}
	}
	// Migration 002: add columns to rules table. SQLite's ALTER TABLE ADD
	// COLUMN has no IF NOT EXISTS, so we check pragma table_info first to
	// stay idempotent across re-init.
	if err := addColumnsIfMissing(ctx, conn, "rules", []columnDef{
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
	if err := addColumnsIfMissing(ctx, conn, "file_instances", []columnDef{
		{"file_status", "TEXT NOT NULL DEFAULT 'active'"},
	}); err != nil {
		return fmt.Errorf("store: migration 4 alter file_instances: %w", err)
	}
	return nil
}

type columnDef struct {
	name string
	def  string
}

// addColumnsIfMissing checks existing columns via pragma table_info and only
// issues ALTER TABLE for missing ones. This makes re-init safe.
func addColumnsIfMissing(ctx context.Context, conn *sql.Conn, table string, cols []columnDef) error {
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
