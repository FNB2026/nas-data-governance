package store

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestLegacySchemaElevenDatabaseUpgrades proves that a project database written
// by an older application build, with the older SQLite driver, still opens,
// migrates and keeps its rows under the current driver.
//
// The fixture is not synthesised here: it has to be produced by the code that
// actually shipped that layout, otherwise the test only proves that today's
// code agrees with itself. scripts/qa/make-legacy-database.sh builds it from the
// git history at 3563831 (schema 11, modernc.org/sqlite v1.56.0) and exports
// NDG_COMPAT_DB. Without that variable the test skips, so ordinary runs are
// unaffected.
func TestLegacySchemaElevenDatabaseUpgrades(t *testing.T) {
	fixture := os.Getenv("NDG_COMPAT_DB")
	if fixture == "" {
		t.Skip("NDG_COMPAT_DB not set; run scripts/qa/make-legacy-database.sh first")
	}
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "ndg-compat-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(dir)
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod temp dir: %v", err)
	}
	target := filepath.Join(dir, "legacy.db")
	copyDatabase(t, fixture, target)

	if got := layoutOf(t, ctx, target); got != legacySchema11Layout {
		t.Fatalf("fixture is not the schema-11 layout: %s", got)
	}

	s, err := Open(ctx, target)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if err := s.Init(ctx); err != nil {
		s.Close()
		t.Fatalf("migrate legacy database: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close after migration: %v", err)
	}
	if got := layoutOf(t, ctx, target); got != governanceSchema12Layout {
		t.Fatalf("post-migration layout is %s, want schema 12", got)
	}

	s, err = Open(ctx, target)
	if err != nil {
		t.Fatalf("reopen migrated database: %v", err)
	}
	defer s.Close()

	var files, storages, rules int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM file_instances`).Scan(&files); err != nil {
		t.Fatalf("count file_instances: %v", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM storages`).Scan(&storages); err != nil {
		t.Fatalf("count storages: %v", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM rules`).Scan(&rules); err != nil {
		t.Fatalf("count rules: %v", err)
	}
	if files != 2 || storages != 1 || rules != 1 {
		t.Fatalf("legacy rows changed: files=%d storages=%d rules=%d", files, storages, rules)
	}
	var name string
	if err := s.db.QueryRowContext(ctx, `SELECT name FROM file_instances WHERE path='/mnt/legacy/a.txt'`).Scan(&name); err != nil {
		t.Fatalf("read legacy row: %v", err)
	}
	if name != "a.txt" {
		t.Fatalf("legacy row content changed: %q", name)
	}

	for table, columns := range map[string][]string{
		"file_instances":     {"file_status", "physical_reliable", "link_count"},
		"purge_plans":        {"dry_run_digest", "dry_run_verified_at"},
		"directory_contexts": {"role", "protected", "business_anchor", "authority_level", "branch_point", "privacy_level"},
	} {
		existing := tableColumns(t, ctx, s.db, table)
		for _, column := range columns {
			if !existing[column] {
				t.Fatalf("%s.%s missing after migration", table, column)
			}
		}
	}
	for _, table := range []string{"governance_project", "governance_previews",
		"governance_batches", "governance_outcomes", "governance_decision_audit"} {
		if !tableExists(t, ctx, s.db, table) {
			t.Fatalf("%s missing after migration", table)
		}
	}

	var integrity string
	if err := s.db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&integrity); err != nil {
		t.Fatalf("quick_check: %v", err)
	}
	if integrity != "ok" {
		t.Fatalf("quick_check reported %q", integrity)
	}
	rows, err := s.db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	if rows.Next() {
		rows.Close()
		t.Fatalf("foreign_key_check reported violations")
	}
	rows.Close()

	if _, err := s.db.ExecContext(ctx, `INSERT INTO file_instances
		(id, storage_id, path, name, size, mode, mtime, device, inode, quick_hash, content_sha256, discovered_at, file_status)
		VALUES (3,'storage-legacy','/mnt/legacy/c.txt','c.txt',32,420,'2026-11-20T00:00:00Z',1,13,'qh-c','sha-c','2026-11-20T00:00:00Z','active')`); err != nil {
		t.Fatalf("insert into migrated database: %v", err)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM file_instances`).Scan(&total); err != nil {
		t.Fatalf("count after insert: %v", err)
	}
	if total != 3 {
		t.Fatalf("insert did not persist: %d", total)
	}

	backup := filepath.Join(dir, "backup.db")
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, backup); err != nil {
		t.Fatalf("backup: %v", err)
	}
	backupDB, err := sql.Open("sqlite", "file:"+backup+"?mode=ro")
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer backupDB.Close()
	var backedUp int
	if err := backupDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM file_instances`).Scan(&backedUp); err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if backedUp != 3 {
		t.Fatalf("backup holds %d rows, want 3", backedUp)
	}
}

func copyDatabase(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("create copy: %v", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
}

func layoutOf(t *testing.T, ctx context.Context, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open for layout: %v", err)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("begin for layout: %v", err)
	}
	defer tx.Rollback()
	layout, err := schemaLayoutFingerprint(ctx, tx)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	return layout
}

func tableColumns(t *testing.T, ctx context.Context, db *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		t.Fatalf("table_info %s: %v", table, err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, kind string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &kind, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info %s: %v", table, err)
		}
		columns[name] = true
	}
	return columns
}

func tableExists(t *testing.T, ctx context.Context, db *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("lookup table %s: %v", table, err)
	}
	return name == table
}
