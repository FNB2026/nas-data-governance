package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestInitIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	// Re-running Init on the same store must not error; the schema uses
	// IF NOT EXISTS precisely so re-runs are safe.
	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("re-init: %v", err)
	}
}

func TestOpenTightensDatabasePermissions(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("POSIX mode assertion")
	}
	dir := filepath.Join(t.TempDir(), "var")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dir, "governance.db")
	if err := os.WriteFile(dbPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	dirInfo, _ := os.Stat(dir)
	dbInfo, _ := os.Stat(dbPath)
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory mode = %o, want 700", got)
	}
	if got := dbInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("database mode = %o, want 600", got)
	}
}

func TestOpenReadOnlyAllowsQueriesAndRejectsWrites(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "只读 database.db")
	writable, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := writable.RegisterStorage(ctx, domain.Storage{ID: "s1", RootPath: "/source", Kind: "test", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := writable.Close(); err != nil {
		t.Fatal(err)
	}

	readOnly, err := OpenReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatalf("open read-only: %v", err)
	}
	defer readOnly.Close()
	storages, err := readOnly.ListStorages(ctx)
	if err != nil || len(storages) != 1 || storages[0].ID != "s1" {
		t.Fatalf("read from read-only store: storages=%#v err=%v", storages, err)
	}
	if err := readOnly.RegisterStorage(ctx, domain.Storage{ID: "s2", RootPath: "/other", Kind: "test", CreatedAt: time.Now()}); err == nil {
		t.Fatal("read-only store unexpectedly allowed a write")
	}
}

func TestOpenReadOnlyRejectsURIPathDelimiters(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "unsafe#name.db")
	if _, err := OpenReadOnly(context.Background(), dbPath); err == nil {
		t.Fatal("expected URI delimiter path to be rejected")
	}
}

func TestOpenReadOnlyRejectsDatabaseSymlink(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "real.db")
	writable, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := writable.Close(); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "linked.db")
	if err := os.Symlink(dbPath, linkPath); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenReadOnly(ctx, linkPath); err == nil {
		t.Fatal("expected database symlink to be rejected")
	}
}

func TestOpenReadOnlyDoesNotCreateMissingDatabase(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "missing.db")
	if _, err := OpenReadOnly(ctx, dbPath); err == nil {
		t.Fatal("expected missing read-only database to fail")
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("read-only open created a database: %v", err)
	}
}

func TestRegisterAndListStorages(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	want := domain.Storage{ID: "nas-1", RootPath: "/Volumes/NAS", Kind: "local", CreatedAt: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)}
	if err := s.RegisterStorage(ctx, want); err != nil {
		t.Fatalf("register: %v", err)
	}
	// Re-register with a different root; upsert should update, not error.
	want.RootPath = "/Volumes/NAS2"
	if err := s.RegisterStorage(ctx, want); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	got, err := s.ListStorages(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || got[0].ID != "nas-1" || got[0].RootPath != "/Volumes/NAS2" {
		t.Fatalf("unexpected storages: %#v", got)
	}
}

func TestUpsertFilesReturnsStableIDs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.RegisterStorage(ctx, domain.Storage{ID: "s1", RootPath: "/", Kind: "local", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "/a/b.txt", Name: "b.txt", Size: 12, Mode: 0644, ModifiedAt: time.Unix(1000, 0), Device: 1, Inode: 2, QuickHash: "q", ContentSHA256: "c", DiscoveredAt: time.Unix(2000, 0)},
		{StorageID: "s1", Path: "/a/c.txt", Name: "c.txt", Size: 34, Mode: 0644, ModifiedAt: time.Unix(1001, 0), Device: 1, Inode: 3, QuickHash: "q2", ContentSHA256: "c2", DiscoveredAt: time.Unix(2001, 0)},
	}
	ids, err := s.UpsertFiles(ctx, files)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}
	// Re-upsert the same path with a new size; ID must stay stable so
	// attached directory_contexts keep pointing at the same row.
	files[0].Size = 99
	ids2, err := s.UpsertFiles(ctx, files)
	if err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if ids2[0] != ids[0] || ids2[1] != ids[1] {
		t.Fatalf("ids not stable: first=%v second=%v", ids, ids2)
	}
	got, err := s.ListFiles(ctx, "s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 || got[0].Size != 99 {
		t.Fatalf("unexpected files: %#v", got)
	}
}

func TestListFilesWithoutStorageReturnsAllStorages(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, id := range []string{"s1", "s2"} {
		if err := s.RegisterStorage(ctx, domain.Storage{ID: id, RootPath: "/" + id, Kind: "test", CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.UpsertFiles(ctx, []domain.FileInstance{{StorageID: id, Path: "/" + id + "/a", Name: "a", ModifiedAt: time.Now(), DiscoveredAt: time.Now()}}); err != nil {
			t.Fatal(err)
		}
	}
	files, err := s.ListFiles(ctx, "")
	if err != nil || len(files) != 2 || files[0].StorageID != "s1" || files[1].StorageID != "s2" {
		t.Fatalf("files=%#v err=%v", files, err)
	}
}

func TestUpsertFilesPreservesHighBitSMBIdentifiers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.RegisterStorage(ctx, domain.Storage{ID: "smb", RootPath: "/share", Kind: "smb", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	wantDevice := uint64(1<<63) + 17
	wantInode := uint64(1<<63) + 99
	files := []domain.FileInstance{{
		StorageID: "smb", Path: "/share/a", Name: "a", Size: 1,
		Device: wantDevice, Inode: wantInode, ModifiedAt: time.Now(), DiscoveredAt: time.Now(),
	}}
	if _, err := s.UpsertFiles(ctx, files); err != nil {
		t.Fatalf("upsert high-bit SMB identifiers: %v", err)
	}
	got, err := s.ListFiles(ctx, "smb")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Device != wantDevice || got[0].Inode != wantInode {
		t.Fatalf("identifier round trip: got %#v", got)
	}
	meta, err := s.ListFileMetadata(ctx, "smb")
	if err != nil || len(meta) != 1 || meta[0].Device != wantDevice || meta[0].Inode != wantInode {
		t.Fatalf("metadata identifier round trip: %#v err=%v", meta, err)
	}
}

func TestUpsertFilesErrorOmitsSensitivePath(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER reject_file BEFORE INSERT ON file_instances BEGIN SELECT RAISE(ABORT, 'rejected'); END`); err != nil {
		t.Fatal(err)
	}
	secretPath := "/private/client/phone-number-recording.mp3"
	_, err := s.UpsertFiles(ctx, []domain.FileInstance{{
		StorageID: "s", Path: secretPath, Name: "phone-number-recording.mp3",
		ModifiedAt: time.Now(), DiscoveredAt: time.Now(),
	}})
	if err == nil {
		t.Fatal("expected forced upsert error")
	}
	if strings.Contains(err.Error(), secretPath) || strings.Contains(err.Error(), "phone-number") {
		t.Fatalf("sensitive path leaked in store error: %v", err)
	}
}

func TestFileIDNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.RegisterStorage(ctx, domain.Storage{ID: "s1", RootPath: "/", Kind: "local", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FileID(ctx, "s1", "/missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSaveAndReplacePlans(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.RegisterStorage(ctx, domain.Storage{ID: "s1", RootPath: "/", Kind: "local", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	taskID := "task-1"
	if err := s.CreateTask(ctx, domain.OperationTask{ID: taskID, RootPath: "/", State: "scanned", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	plans := []domain.OperationPlan{
		{
			ID: "dup-aaaaaaaaaaaa", State: domain.PlanDraft, ContentSHA256: "abc", Size: 10,
			Risk: domain.RiskMedium, RetainPath: "/tmp/a",
			RetainScore: domain.RetentionScore{Total: 50, Reasons: []string{"x"}},
			Actions: []domain.PlannedAction{
				{Path: "/tmp/a", Action: domain.OperationKeep, Reason: "retain"},
				{Path: "/tmp/b", Action: domain.OperationQuarantine, Reason: "dup"},
			},
			Evidence: []string{"e1", "e2"},
		},
	}
	if err := s.SavePlans(ctx, taskID, plans); err != nil {
		t.Fatalf("save plans: %v", err)
	}
	got, err := s.ListPlans(ctx, taskID)
	if err != nil {
		t.Fatalf("list plans: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(got))
	}
	p := got[0]
	if p.ID != plans[0].ID || p.RetainPath != plans[0].RetainPath || p.Risk != plans[0].Risk {
		t.Fatalf("plan round-trip mismatch: %#v", p)
	}
	if len(p.Actions) != 2 || p.Actions[0].Action != domain.OperationKeep {
		t.Fatalf("actions round-trip mismatch: %#v", p.Actions)
	}
	if p.RetainScore.Total != 50 || len(p.RetainScore.Reasons) != 1 {
		t.Fatalf("retain score mismatch: %#v", p.RetainScore)
	}

	// The normalized state column is authoritative after transitions; list
	// methods must not return the stale state embedded in evidence_json.
	if err := s.UpdatePlanState(ctx, plans[0].ID, domain.PlanApproved); err != nil {
		t.Fatalf("update plan state: %v", err)
	}
	got, err = s.ListPlans(ctx, taskID)
	if err != nil || len(got) != 1 || got[0].State != domain.PlanApproved {
		t.Fatalf("ListPlans did not expose authoritative state: %#v, %v", got, err)
	}
	all, err := s.ListAllPlans(ctx)
	if err != nil || len(all) != 1 || all[0].State != domain.PlanApproved {
		t.Fatalf("ListAllPlans did not expose authoritative state: %#v, %v", all, err)
	}

	// Replace with a smaller plan set; the old plan must disappear.
	replacement := []domain.OperationPlan{
		{ID: "dup-bbbbbbbbbbbb", State: domain.PlanDraft, Risk: domain.RiskHigh, Actions: []domain.PlannedAction{{Path: "/x", Action: domain.OperationReview}}, Evidence: []string{"new"}},
	}
	if err := s.SavePlans(ctx, taskID, replacement); err != nil {
		t.Fatalf("save replacement: %v", err)
	}
	got, _ = s.ListPlans(ctx, taskID)
	if len(got) != 1 || got[0].ID != "dup-bbbbbbbbbbbb" {
		t.Fatalf("expected replacement only, got %#v", got)
	}
}

func TestAppendAndListLogs(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.RegisterStorage(ctx, domain.Storage{ID: "s1", RootPath: "/", Kind: "local", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTask(ctx, domain.OperationTask{ID: "t1", RootPath: "/", State: "x", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePlans(ctx, "t1", []domain.OperationPlan{{ID: "p1", State: domain.PlanDraft, Risk: domain.RiskLow, Actions: []domain.PlannedAction{{Path: "/a", Action: domain.OperationKeep}}}}); err != nil {
		t.Fatal(err)
	}
	detail := map[string]any{"actor": "executor", "bytes_copied": float64(1024)}
	if err := s.AppendLog(ctx, "p1", "STALE_CHECK", detail); err != nil {
		t.Fatalf("append log: %v", err)
	}
	logs, err := s.ListLogs(ctx, "p1")
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 || logs[0].EventType != "STALE_CHECK" || logs[0].Detail["actor"] != "executor" {
		t.Fatalf("unexpected logs: %#v", logs)
	}
}

func TestSaveContextRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.RegisterStorage(ctx, domain.Storage{ID: "s1", RootPath: "/", Kind: "local", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	ids, err := s.UpsertFiles(ctx, []domain.FileInstance{
		{StorageID: "s1", Path: "/家庭/医疗/报告.pdf", Name: "报告.pdf", Size: 1, Mode: 0644, ModifiedAt: time.Unix(100, 0), DiscoveredAt: time.Unix(200, 0)},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	wantCtx := domain.DirectoryContext{
		Role: domain.RoleSensitive, AuthorityLevel: 100, PrivacyLevel: "high", Protected: true,
		MatchedTerms: []string{"医疗"},
		ParentChain: []domain.ChainNode{
			{Path: "/家庭", Name: "家庭", Role: domain.RoleUnknown, Authority: 50},
			{Path: "/家庭/医疗", Name: "医疗", Role: domain.RoleSensitive, Authority: 100},
		},
		BranchPoint:    "/家庭/医疗",
		BusinessAnchor: "",
	}
	if err := s.SaveContext(ctx, ids[0], wantCtx, "v1"); err != nil {
		t.Fatalf("save context: %v", err)
	}
	// Re-save with new rule version; upsert should replace, not error.
	wantCtx.BusinessAnchor = "2024"
	if err := s.SaveContext(ctx, ids[0], wantCtx, "v2"); err != nil {
		t.Fatalf("re-save context: %v", err)
	}
	// There is no public GetContext in the interface yet (it's planned for M3);
	// query the table directly to verify round-trip.
	var blob, ruleVer string
	err = s.db.QueryRowContext(ctx,
		`SELECT context_json, rule_version FROM directory_contexts WHERE file_id = ?`, ids[0]).Scan(&blob, &ruleVer)
	if err != nil {
		t.Fatalf("direct query: %v", err)
	}
	if ruleVer != "v2" {
		t.Fatalf("rule version = %q, want v2", ruleVer)
	}
	var got domain.DirectoryContext
	if err := json.Unmarshal([]byte(blob), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Role != domain.RoleSensitive || !got.Protected || len(got.ParentChain) != 2 || got.BusinessAnchor != "2024" {
		t.Fatalf("round-trip mismatch: %#v", got)
	}
}

func TestSaveFormatsByPathAndListFormats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.RegisterStorage(ctx, domain.Storage{ID: "s1", RootPath: "/", Kind: "local", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	_, err := s.UpsertFiles(ctx, []domain.FileInstance{
		{StorageID: "s1", Path: "/a.aif", Name: "a.aif", ModifiedAt: time.Now(), DiscoveredAt: time.Now()},
		{StorageID: "s1", Path: "/b.xmp", Name: "b.xmp", ModifiedAt: time.Now(), DiscoveredAt: time.Now()},
	})
	if err != nil {
		t.Fatal(err)
	}
	records := []FormatRecord{
		{StorageID: "s1", Path: "/a.aif", Info: domain.FormatInfo{Format: "aiff", Category: domain.CategoryAudio}},
		{StorageID: "s1", Path: "/b.xmp", Info: domain.FormatInfo{Format: "xmp", Category: domain.CategoryOther, Role: domain.FormatRoleMetadataSidecar, Protected: true}},
		{StorageID: "s1", Path: "/missing", Info: domain.FormatInfo{Format: "unknown", Category: domain.CategoryUnknown}},
	}
	saved, missing, err := s.SaveFormatsByPath(ctx, records)
	if err != nil || saved != 2 || missing != 1 {
		t.Fatalf("saved=%d missing=%d err=%v", saved, missing, err)
	}
	got, err := s.ListFormats(ctx, "s1")
	if err != nil || len(got) != 2 {
		t.Fatalf("got=%#v err=%v", got, err)
	}
	if got[1].Info.Role != domain.FormatRoleMetadataSidecar || !got[1].Info.Protected {
		t.Fatalf("policy metadata did not round trip: %#v", got[1])
	}
	// Idempotent update in one transaction.
	records[0].Info.MIME = "audio/aiff"
	if saved, missing, err := s.SaveFormatsByPath(ctx, records[:1]); err != nil || saved != 1 || missing != 0 {
		t.Fatalf("repeat saved=%d missing=%d err=%v", saved, missing, err)
	}
}

// TestLinkCountMigrationAndRoundTrip verifies two scenarios:
//  1. Old database migration: a database created before migration 010
//     (without the link_count column) is opened, and Init adds the column
//     with a default of 0. Pre-existing rows must read back with LinkCount=0.
//  2. New scan round-trip: a file upserted with Physical.LinkCount > 1
//     (hardlink) must persist and read back with the exact value via
//     ListFiles, and a re-upsert must update the value.
func TestLinkCountMigrationAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "linkcount-migration.db")

	// --- Phase 1: simulate an old database without link_count ---
	// Create the database with raw SQL using only the original schema
	// (schema 001, before migrations 004/010 added file_status,
	// physical_reliable, and link_count).
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	// Create a minimal old-schema file_instances table — no link_count,
	// no file_status, no physical_reliable.
	oldSchema := `
CREATE TABLE IF NOT EXISTS storages (id TEXT PRIMARY KEY, root_path TEXT NOT NULL, kind TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS file_instances (
  id INTEGER PRIMARY KEY, storage_id TEXT NOT NULL REFERENCES storages(id), path TEXT NOT NULL,
  name TEXT NOT NULL, size INTEGER NOT NULL, mode INTEGER NOT NULL, mtime TEXT NOT NULL,
  device INTEGER, inode INTEGER, quick_hash TEXT, content_sha256 TEXT, discovered_at TEXT NOT NULL,
  verified_at TEXT, UNIQUE(storage_id, path)
);`
	if _, err := rawDB.ExecContext(ctx, oldSchema); err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	// Insert a storage and a file row the old way (no link_count).
	if _, err := rawDB.ExecContext(ctx,
		`INSERT INTO storages(id, root_path, kind, created_at) VALUES('s1', '/old', 'local', ?)`,
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert old storage: %v", err)
	}
	if _, err := rawDB.ExecContext(ctx,
		`INSERT INTO file_instances(storage_id, path, name, size, mode, mtime, device, inode, quick_hash, content_sha256, discovered_at)
		 VALUES('s1', '/old/file.txt', 'file.txt', 100, 0644, ?, 16777220, 12345, '', '', ?)`,
		time.Now().UTC().Format(time.RFC3339Nano),
		time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("insert old file: %v", err)
	}
	// Verify the column does NOT exist yet.
	var colCount int
	if err := rawDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_table_info('file_instances') WHERE name='link_count'").Scan(&colCount); err != nil {
		t.Fatalf("check old schema: %v", err)
	}
	if colCount != 0 {
		t.Fatalf("link_count column should not exist in old schema, got count=%d", colCount)
	}
	if err := rawDB.Close(); err != nil {
		t.Fatalf("close raw db: %v", err)
	}

	// --- Phase 2: open with Open(), which runs Init() and applies migrations ---
	s, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("open with migration: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// The migration must have added link_count with DEFAULT 0.
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM pragma_table_info('file_instances') WHERE name='link_count'").Scan(&colCount); err != nil {
		t.Fatalf("check migrated schema: %v", err)
	}
	if colCount != 1 {
		t.Fatalf("link_count column should exist after migration, got count=%d", colCount)
	}

	// The pre-existing old file must read back with LinkCount=0 (default).
	oldFiles, err := s.ListFiles(ctx, "s1")
	if err != nil {
		t.Fatalf("list old files: %v", err)
	}
	if len(oldFiles) != 1 {
		t.Fatalf("expected 1 old file, got %d", len(oldFiles))
	}
	if oldFiles[0].Physical.LinkCount != 0 {
		t.Fatalf("old file LinkCount = %d, want 0 (migration default)", oldFiles[0].Physical.LinkCount)
	}

	// --- Phase 3: round-trip a new scan with hardlink evidence ---
	// Upsert a file with LinkCount=3 (a hardlink with 3 names).
	wantLinkCount := uint64(3)
	newFiles := []domain.FileInstance{{
		StorageID: "s1", Path: "/old/hardlink.txt", Name: "hardlink.txt",
		Size: 200, Mode: 0644, ModifiedAt: time.Unix(1000, 0),
		Device: 16777220, Inode: 99999, DiscoveredAt: time.Unix(2000, 0),
		Physical: domain.PhysicalIdentity{
			Device:    16777220,
			Inode:     99999,
			LinkCount: wantLinkCount,
			Reliable:  true,
		},
	}}
	if _, err := s.UpsertFiles(ctx, newFiles); err != nil {
		t.Fatalf("upsert hardlink file: %v", err)
	}

	// Read back and verify LinkCount round-trips exactly.
	gotFiles, err := s.ListFiles(ctx, "s1")
	if err != nil {
		t.Fatalf("list after upsert: %v", err)
	}
	var hardlinkFile *domain.FileInstance
	for i := range gotFiles {
		if gotFiles[i].Path == "/old/hardlink.txt" {
			hardlinkFile = &gotFiles[i]
			break
		}
	}
	if hardlinkFile == nil {
		t.Fatalf("hardlink file not found in list")
	}
	if hardlinkFile.Physical.LinkCount != wantLinkCount {
		t.Fatalf("hardlink LinkCount = %d, want %d", hardlinkFile.Physical.LinkCount, wantLinkCount)
	}
	if !hardlinkFile.Physical.Reliable {
		t.Fatalf("hardlink Physical.Reliable = false, want true")
	}

	// --- Phase 4: re-upsert updates link_count ---
	updatedLinkCount := uint64(1)
	newFiles[0].Physical.LinkCount = updatedLinkCount
	if _, err := s.UpsertFiles(ctx, newFiles); err != nil {
		t.Fatalf("re-upsert hardlink file: %v", err)
	}
	gotFiles, err = s.ListFiles(ctx, "s1")
	if err != nil {
		t.Fatalf("list after re-upsert: %v", err)
	}
	for i := range gotFiles {
		if gotFiles[i].Path == "/old/hardlink.txt" {
			if gotFiles[i].Physical.LinkCount != updatedLinkCount {
				t.Fatalf("re-upserted LinkCount = %d, want %d", gotFiles[i].Physical.LinkCount, updatedLinkCount)
			}
			break
		}
	}

	// --- Phase 5: high-bit LinkCount (edge case) ---
	highBitLinkCount := uint64(1<<63 + 7)
	edgeFiles := []domain.FileInstance{{
		StorageID: "s1", Path: "/old/edge.bin", Name: "edge.bin",
		Size: 1, Mode: 0644, ModifiedAt: time.Unix(1000, 0),
		Device: 16777220, Inode: 88888, DiscoveredAt: time.Unix(2000, 0),
		Physical: domain.PhysicalIdentity{
			Device:    16777220,
			Inode:     88888,
			LinkCount: highBitLinkCount,
			Reliable:  true,
		},
	}}
	if _, err := s.UpsertFiles(ctx, edgeFiles); err != nil {
		t.Fatalf("upsert high-bit link_count: %v", err)
	}
	gotFiles, err = s.ListFiles(ctx, "s1")
	if err != nil {
		t.Fatalf("list after edge upsert: %v", err)
	}
	for i := range gotFiles {
		if gotFiles[i].Path == "/old/edge.bin" {
			if gotFiles[i].Physical.LinkCount != highBitLinkCount {
				t.Fatalf("high-bit LinkCount = %d, want %d", gotFiles[i].Physical.LinkCount, highBitLinkCount)
			}
			break
		}
	}
}
