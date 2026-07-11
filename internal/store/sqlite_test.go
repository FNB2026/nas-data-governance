package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"nas-data-governance/internal/domain"
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
