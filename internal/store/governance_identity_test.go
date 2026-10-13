package store

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func databaseEvidence(t *testing.T, path string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		bytes, e := os.ReadFile(path + suffix)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			t.Fatal("synthetic database evidence read")
		}
		out[suffix] = bytes
	}
	return out
}

func TestForeignMarkerDatabaseRejectedBeforeWritableOpen(t *testing.T) {
	for _, marker := range []int{-1, 1, 11, 12} {
		t.Run(map[int]string{-1: "empty", 1: "low", 11: "legacy-looking", 12: "current-looking"}[marker], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "foreign-disposable.db")
			db, e := sql.Open("sqlite", path)
			if e != nil {
				t.Fatal("synthetic open")
			}
			defer db.Close()
			if _, e = db.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,applied_at TEXT); CREATE TABLE synthetic_business_records(id INTEGER PRIMARY KEY, value TEXT); INSERT INTO synthetic_business_records VALUES(1,'artificial data');`); e != nil {
				t.Fatal("synthetic schema")
			}
			if marker >= 0 {
				if _, e = db.Exec(`INSERT INTO schema_migrations VALUES(?,'2026-10-13T00:00:00Z')`, marker); e != nil {
					t.Fatal("synthetic marker")
				}
			}
			var n int
			if db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&n) != nil {
				t.Fatal("synthetic read snapshot")
			}
			if os.Chmod(path, 0600) != nil {
				t.Fatal("synthetic secure mode")
			}
			before := databaseEvidence(t, path)
			if st, e := Open(context.Background(), path); e == nil {
				st.Close()
				t.Error("foreign marker-shaped database was accepted")
			}
			after := databaseEvidence(t, path)
			if !reflect.DeepEqual(before, after) {
				t.Error("refusal changed database or SQLite sidecar bytes")
			}
			if db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='storages'`).Scan(&n) != nil || n != 0 {
				t.Error("foreign database received NDG migration")
			}
		})
	}
}

func TestUnsupportedKnownLayoutRefusedWithoutMutation(t *testing.T) {
	cases := []struct {
		name    string
		current bool
		change  string
	}{
		{"legacy-low-marker", false, `INSERT INTO schema_migrations VALUES(1,'synthetic')`},
		{"legacy-marker11", false, `INSERT INTO schema_migrations VALUES(11,'synthetic')`},
		{"legacy-marker12", false, `INSERT INTO schema_migrations VALUES(12,'synthetic')`},
		{"extra-table", false, `CREATE TABLE extra_synthetic_data(id INTEGER)`},
		{"extra-index", false, `CREATE INDEX extra_synthetic_index ON storages(kind)`},
		{"extra-trigger", false, `CREATE TRIGGER extra_synthetic_trigger AFTER INSERT ON storages BEGIN SELECT 1; END`},
		{"extra-view", false, `CREATE VIEW extra_synthetic_view AS SELECT id FROM storages`},
		{"sqlite-prefix-lookalike", false, `CREATE TABLE sqliteXhidden(id INTEGER)`},
		{"missing-index", false, `DROP INDEX idx_files_size_quick`},
		{"current-missing-marker", true, `DELETE FROM schema_migrations`},
		{"current-extra-marker", true, `INSERT INTO schema_migrations VALUES(1,'synthetic')`},
		{"current-wrong-marker", true, `UPDATE schema_migrations SET version=11`},
		{"current-extra-trigger", true, `CREATE TRIGGER extra_synthetic_trigger AFTER INSERT ON storages BEGIN SELECT 1; END`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "disposable-layout.db")
			if tc.current {
				s, e := Open(context.Background(), path)
				if e != nil {
					t.Fatal("current layout creation")
				}
				if s.Close() != nil {
					t.Fatal("current layout close")
				}
			} else {
				db, e := sql.Open("sqlite", path)
				if e != nil {
					t.Fatal("legacy fixture open")
				}
				if createSupportedLegacySchema11(context.Background(), db) != nil {
					t.Fatal("legacy fixture initialization")
				}
				if db.Close() != nil || os.Chmod(path, 0600) != nil {
					t.Fatal("legacy fixture close")
				}
			}
			db, e := sql.Open("sqlite", path)
			if e != nil {
				t.Fatal("fault fixture connection")
			}
			if _, e = db.Exec(tc.change); e != nil {
				t.Fatal("fault fixture mutation", e)
			}
			if db.Close() != nil {
				t.Fatal("fault fixture close")
			}
			before := databaseEvidence(t, path)
			if s, e := Open(context.Background(), path); e == nil {
				s.Close()
				t.Error("unsupported layout admitted")
			}
			if !reflect.DeepEqual(before, databaseEvidence(t, path)) {
				t.Error("refusal changed database or sidecars")
			}
		})
	}
}

func TestIdentityPreflightReadsLiveWALMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "live-wal.db")
	s, e := Open(context.Background(), path)
	if e != nil {
		t.Fatal("current fixture open")
	}
	if s.Close() != nil {
		t.Fatal("current fixture close")
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal("live fixture connection")
	}
	defer db.Close()
	if _, e = db.Exec(`UPDATE schema_migrations SET version=99`); e != nil {
		t.Fatal("WAL marker injection")
	}
	var n int
	if db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&n) != nil {
		t.Fatal("read snapshot")
	}
	before := databaseEvidence(t, path)
	if len(before["-wal"]) == 0 || len(before["-shm"]) == 0 {
		t.Fatal("test requires real existing WAL/SHM")
	}
	if s, e := Open(context.Background(), path); e == nil {
		s.Close()
		t.Error("uncheckpointed unsupported marker admitted")
	}
	if !reflect.DeepEqual(before, databaseEvidence(t, path)) {
		t.Error("WAL refusal changed database/sidecars")
	}
}

func TestIdentityPreflightRefusesAmbiguousSidecars(t *testing.T) {
	for _, suffix := range []string{"-wal", "-shm"} {
		t.Run(suffix, func(t *testing.T) {
			path := legacyGovernanceDatabase(t, false)
			if os.WriteFile(path+suffix, []byte("disposable-incomplete-sidecar"), 0600) != nil {
				t.Fatal("sidecar fixture")
			}
			before := databaseEvidence(t, path)
			if s, e := Open(context.Background(), path); e == nil {
				s.Close()
				t.Error("incomplete sidecar pair accepted")
			}
			if !reflect.DeepEqual(before, databaseEvidence(t, path)) {
				t.Error("sidecar refusal changed evidence")
			}
		})
	}
}

// A subprocess dies without closing its SQLite transaction. This is a genuine
// disposable rollback-journal boundary, not a fake marker file or historical DB.
func TestIdentityHotJournalProcess(t *testing.T) {
	path := os.Getenv("NDG_TEST_IDENTITY_HOT_JOURNAL")
	if path == "" {
		return
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		os.Exit(2)
	}
	if _, e = db.Exec(`PRAGMA journal_mode=DELETE; PRAGMA cache_size=4; BEGIN IMMEDIATE; CREATE TABLE synthetic_incomplete_write(value BLOB)`); e != nil {
		os.Exit(3)
	}
	for i := 0; i < 100; i++ {
		if _, e = db.Exec(`INSERT INTO synthetic_incomplete_write VALUES(zeroblob(4096))`); e != nil {
			os.Exit(4)
		}
	}
	os.Exit(0) // Deliberately bypass DB/transaction cleanup in this child only.
}

func TestIdentityPreflightRefusesHotRollbackJournal(t *testing.T) {
	path := legacyGovernanceDatabase(t, false)
	cmd := exec.Command(os.Args[0], "-test.run=^TestIdentityHotJournalProcess$")
	cmd.Env = append(os.Environ(), "NDG_TEST_IDENTITY_HOT_JOURNAL="+path)
	if e := cmd.Run(); e != nil {
		t.Fatal("disposable journal child failed")
	}
	before := databaseEvidence(t, path)
	j := before["-journal"]
	if len(j) <= 512 || bytes.Equal(j[:8], make([]byte, 8)) {
		t.Fatal("actual hot journal required")
	}
	if _, ok := before["-wal"]; ok {
		t.Fatal("rollback fixture unexpectedly WAL")
	}
	if s, e := Open(context.Background(), path); e == nil {
		s.Close()
		t.Error("ambiguous hot journal accepted")
	}
	if !reflect.DeepEqual(before, databaseEvidence(t, path)) {
		t.Error("refusal changed main/journal or created SQLite sidecars")
	}
}
