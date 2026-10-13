package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/schemas"
)

func legacyGovernanceDatabase(t *testing.T, broken bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy-synthetic.db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(schemas.Schema001 + schemas.Schema008); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO group_decisions(id,group_id,decision_type,retained_file_id,reason,rule_id,created_at,updated_at) VALUES('primary','primary','PRIMARY_RETENTION',42,'retain synthetic','old-rule','2026-10-12T00:00:00Z','2026-10-12T00:00:00Z'),('unknown','unknown','FUTURE_HISTORICAL',NULL,NULL,NULL,'2026-10-12T00:00:00Z','2026-10-12T00:00:00Z')`); e != nil {
		t.Fatal(e)
	}
	if broken {
		if _, e = db.Exec(`CREATE TABLE governance_outcomes(broken TEXT)`); e != nil {
			t.Fatal(e)
		}
	}
	if db.Close() != nil {
		t.Fatal("legacy close")
	}
	if os.Chmod(path, 0600) != nil {
		t.Fatal("secure synthetic fixture")
	}
	return path
}
func TestLegacyReadOnlyDecisionsPreserveUnknownAndPrimary(t *testing.T) {
	path := legacyGovernanceDatabase(t, false)
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	s, e := OpenReadOnly(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	primary, e := s.GetGroupDecision(context.Background(), "primary")
	if e != nil || primary.DecisionType != domain.DecisionPrimaryRetention || primary.RetainedFileID == nil || *primary.RetainedFileID != 42 || primary.RuleID != "old-rule" {
		t.Fatal("primary history lost", e)
	}
	unknown, e := s.GetGroupDecision(context.Background(), "unknown")
	if e != nil || unknown.DecisionType != "FUTURE_HISTORICAL" {
		t.Fatal("unknown legacy enum lost", e)
	}
	all, e := s.ListGroupDecisions(context.Background(), "")
	if e != nil || len(all) != 2 {
		t.Fatal("legacy listing", e)
	}
	s.Close()
	after, _ := os.ReadFile(path)
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("read-only legacy database changed")
	}
	s, e = Open(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	primary, e = s.GetGroupDecision(context.Background(), "primary")
	if e != nil || primary.Revision != 1 || *primary.RetainedFileID != 42 {
		t.Fatal("migration remapped legacy decision", e)
	}
	var n int
	if s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=12`).Scan(&n) != nil || n != 1 {
		t.Fatal("migration version missing")
	}
}
func TestGovernanceMigrationFailureRollsBackAdditiveIncrement(t *testing.T) {
	path := legacyGovernanceDatabase(t, true)
	if s, e := Open(context.Background(), path); e == nil {
		s.Close()
		t.Fatal("malformed governance table accepted")
	}
	db, e := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	var n int
	for _, q := range []string{`SELECT COUNT(*) FROM pragma_table_info('group_decisions') WHERE name='revision'`, `SELECT COUNT(*) FROM schema_migrations WHERE version=12`, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='governance_project'`} {
		if db.QueryRow(q).Scan(&n) != nil || n != 0 {
			t.Fatal("partial governance migration persisted")
		}
	}
	if db.QueryRow(`SELECT COUNT(*) FROM group_decisions WHERE (decision_type='PRIMARY_RETENTION' AND retained_file_id=42) OR decision_type='FUTURE_HISTORICAL'`).Scan(&n) != nil || n != 2 {
		t.Fatal("legacy records changed on migration failure")
	}
}

func TestUnknownFutureSchemaRejectedBeforeDatabaseMutation(t *testing.T) {
	path := legacyGovernanceDatabase(t, false)
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(99,'2026-10-13T00:00:00Z')`); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if st, err := Open(context.Background(), path); err == nil {
		st.Close()
		t.Fatal("future schema accepted")
	}
	after, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("unsupported schema was mutated before refusal")
	}
}

func TestUnidentifiedDatabaseRejectedBeforeMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unidentified.db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`CREATE TABLE synthetic_business_records(id INTEGER PRIMARY KEY)`); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(path, 0600); e != nil {
		t.Fatal(e)
	}
	before, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if st, err := Open(context.Background(), path); err == nil {
		st.Close()
		t.Fatal("unidentified database accepted")
	}
	after, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("unidentified database mutated")
	}
}
