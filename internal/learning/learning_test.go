package learning

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/store"
)

// newLearnStore opens a fresh SQLite store in a temp dir for learning tests.
func newLearnStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "learn.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// seedFiles registers a storage and inserts the given file paths.
func seedFiles(t *testing.T, st *store.SQLiteStore, storageID string, paths []string) {
	t.Helper()
	ctx := context.Background()
	if err := st.RegisterStorage(ctx, domain.Storage{
		ID: storageID, RootPath: "/vol", Kind: "filesystem", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("register storage: %v", err)
	}
	files := make([]domain.FileInstance, 0, len(paths))
	now := time.Now().UTC()
	for i, p := range paths {
		files = append(files, domain.FileInstance{
			StorageID:    storageID,
			Path:         p,
			Name:         filepath.Base(p),
			Size:         int64(100 + i),
			Mode:         0o644,
			ModifiedAt:   now,
			DiscoveredAt: now,
		})
	}
	if _, err := st.UpsertFiles(ctx, files); err != nil {
		t.Fatalf("upsert files: %v", err)
	}
}

func findStat(stats *Stats, name string) (DirNameStat, bool) {
	for _, s := range stats.DirStats {
		if s.Name == name {
			return s, true
		}
	}
	return DirNameStat{}, false
}

// TestLearn_StatsSensitiveSkipAndThreshold verifies Learn() collects directory
// name frequencies, skips sensitive directories (K-009), skips builtin terms,
// and applies threshold filtering.
func TestLearn_StatsSensitiveSkipAndThreshold(t *testing.T) {
	st := newLearnStore(t)
	paths := []string{
		"/vol/归档/PRJ-2024-001/deliverables/a.txt",
		"/vol/归档/PRJ-2024-002/deliverables/b.txt",
		"/vol/临时/PRJ-2024-001/drafts/c.txt",
		"/vol/临时/PRJ-2024-002/drafts/d.txt",
		"/vol/scratch/once.txt",  // below threshold: 1 dir, 1 file
		"/vol/私密/secret.txt",   // sensitive: skipped entirely
	}
	seedFiles(t, st, "local", paths)

	stats, err := Learn(context.Background(), st)
	if err != nil {
		t.Fatalf("Learn: %v", err)
	}
	if stats.TotalFiles != 6 {
		t.Errorf("TotalFiles = %d, want 6", stats.TotalFiles)
	}
	if stats.SensitiveSkipped != 1 {
		t.Errorf("SensitiveSkipped = %d, want 1", stats.SensitiveSkipped)
	}

	// "deliverables" co-occurs with builtin "归档" → formal_archive.
	ds, ok := findStat(stats, "deliverables")
	if !ok {
		t.Fatalf("expected dir stat for 'deliverables'")
	}
	if ds.DirCount != 2 {
		t.Errorf("deliverables DirCount = %d, want 2", ds.DirCount)
	}
	if ds.FileCount != 2 {
		t.Errorf("deliverables FileCount = %d, want 2", ds.FileCount)
	}
	if ds.SuggestedRole != domain.RoleFormalArchive {
		t.Errorf("deliverables SuggestedRole = %s, want formal_archive", ds.SuggestedRole)
	}

	// "drafts" co-occurs with builtin "临时" → temporary.
	ds, ok = findStat(stats, "drafts")
	if !ok {
		t.Fatalf("expected dir stat for 'drafts'")
	}
	if ds.SuggestedRole != domain.RoleTemporary {
		t.Errorf("drafts SuggestedRole = %s, want temporary", ds.SuggestedRole)
	}

	// "scratch" below threshold → filtered out.
	if _, ok := findStat(stats, "scratch"); ok {
		t.Errorf("scratch should be filtered by threshold (1 dir, 1 file)")
	}

	// Builtin terms must not appear as learned dir stats.
	if _, ok := findStat(stats, "归档"); ok {
		t.Errorf("builtin term '归档' must not be tracked as a learned stat")
	}
	if _, ok := findStat(stats, "临时"); ok {
		t.Errorf("builtin term '临时' must not be tracked as a learned stat")
	}

	// Project code pattern generalized and counted.
	if len(stats.ProjectCodes) == 0 {
		t.Fatalf("expected at least one project code pattern")
	}
	pc := stats.ProjectCodes[0]
	if pc.Pattern != "^[A-Z]+-\\d+-\\d+$" {
		t.Errorf("project code pattern = %q, want ^[A-Z]+-\\d+-\\d+$", pc.Pattern)
	}
	if pc.Count < 2 {
		t.Errorf("project code count = %d, want >= 2", pc.Count)
	}
}

// TestGenerateDrafts_PersistsDraftRules verifies GenerateDrafts writes learned
// rules with Status=draft, Source=learned, Priority<=60, and that no rule
// targets the sensitive role.
func TestGenerateDrafts_PersistsDraftRules(t *testing.T) {
	st := newLearnStore(t)
	paths := []string{
		"/vol/归档/PRJ-2024-001/deliverables/a.txt",
		"/vol/归档/PRJ-2024-002/deliverables/b.txt",
		"/vol/临时/PRJ-2024-001/drafts/c.txt",
		"/vol/临时/PRJ-2024-002/drafts/d.txt",
		"/vol/私密/secret.txt",
	}
	seedFiles(t, st, "local", paths)

	stats, err := Learn(context.Background(), st)
	if err != nil {
		t.Fatalf("Learn: %v", err)
	}
	batchID := NewBatchID("stats", time.Date(2026, 7, 12, 14, 30, 0, 0, time.UTC))
	if !strings.HasPrefix(batchID, "learn-stats-20260712-143000") {
		t.Fatalf("NewBatchID = %q", batchID)
	}
	rules, err := GenerateDrafts(context.Background(), st, stats, batchID)
	if err != nil {
		t.Fatalf("GenerateDrafts: %v", err)
	}
	if len(rules) == 0 {
		t.Fatalf("expected at least one draft rule")
	}

	// Find the deliverables rule.
	var deliv *domain.Rule
	for i := range rules {
		if strings.Contains(rules[i].Definition, "deliverables") {
			deliv = &rules[i]
		}
	}
	if deliv == nil {
		t.Fatalf("no draft rule for 'deliverables'")
	}
	if deliv.Source != domain.RuleSourceLearned {
		t.Errorf("Source = %s, want learned", deliv.Source)
	}
	if deliv.Status != domain.RuleDraft {
		t.Errorf("Status = %s, want draft", deliv.Status)
	}
	if deliv.Priority > 60 {
		t.Errorf("Priority = %d, must be <= 60 (K-008)", deliv.Priority)
	}
	if deliv.BatchID != batchID {
		t.Errorf("BatchID = %q, want %q", deliv.BatchID, batchID)
	}
	if deliv.Confidence <= 0 || deliv.Confidence > 1 {
		t.Errorf("Confidence = %f, want (0,1]", deliv.Confidence)
	}
	// Definition must round-trip through parseDirectorySignal shape.
	if !strings.Contains(deliv.Definition, "formal_archive") {
		t.Errorf("Definition missing role formal_archive: %s", deliv.Definition)
	}

	// No rule may target the sensitive role.
	for _, r := range rules {
		if strings.Contains(r.Definition, "sensitive") {
			t.Errorf("rule %s targets sensitive role — not allowed: %s", r.ID, r.Definition)
		}
	}

	// Rules persisted to the store.
	persisted, err := st.ListRules(context.Background(), domain.RuleSourceLearned, domain.RuleDraft)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(persisted) != len(rules) {
		t.Errorf("persisted rules = %d, want %d", len(persisted), len(rules))
	}

	// Learning batch recorded.
	// (SaveLearningBatch upserts; verify via the rule count matching.)
}

// TestGenerateDrafts_PreservesApprovedRules verifies re-running learning does
// not overwrite a rule that already left draft status (human decision kept).
func TestGenerateDrafts_PreservesApprovedRules(t *testing.T) {
	st := newLearnStore(t)
	paths := []string{
		"/vol/归档/PRJ-2024-001/deliverables/a.txt",
		"/vol/归档/PRJ-2024-002/deliverables/b.txt",
	}
	seedFiles(t, st, "local", paths)

	// Pre-create an approved rule for 'deliverables' (same ID GenerateDrafts
	// would produce).
	approved := domain.Rule{
		ID:         "learned-dir-deliverables",
		Version:    1,
		Priority:   60,
		Enabled:    true,
		Source:     domain.RuleSourceLearned,
		BatchID:    "old-batch",
		Confidence: 0.9,
		Status:     domain.RuleApproved,
		Definition: "match:\n  segment_contains: \"deliverables\"\neffect:\n  role: formal_archive\n  authority: 60",
	}
	ctx := context.Background()
	if err := st.SaveRule(ctx, approved); err != nil {
		t.Fatalf("seed approved rule: %v", err)
	}

	stats, err := Learn(ctx, st)
	if err != nil {
		t.Fatalf("Learn: %v", err)
	}
	rules, err := GenerateDrafts(ctx, st, stats, "new-batch")
	if err != nil {
		t.Fatalf("GenerateDrafts: %v", err)
	}
	// The approved rule must NOT appear in the generated drafts.
	for _, r := range rules {
		if r.ID == approved.ID {
			t.Fatalf("approved rule %s was overwritten by GenerateDrafts", r.ID)
		}
	}
	// And it must still be approved in the store.
	persisted, err := st.ListRules(ctx, domain.RuleSourceLearned, "")
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	for _, r := range persisted {
		if r.ID == approved.ID && r.Status != domain.RuleApproved {
			t.Errorf("approved rule %s status changed to %s — must be preserved", r.ID, r.Status)
		}
	}
}

// TestSuggestRole_TieBreakIsDeterministic verifies that when a directory name
// co-occurs equally with a builtin protection term and a builtin low-authority
// term, the higher-authority role wins — and the result is stable across many
// runs despite Go's randomized map iteration.
func TestSuggestRole_TieBreakIsDeterministic(t *testing.T) {
	st := newLearnStore(t)
	// "tietest" co-occurs once with "归档" (formal_archive, auth 90) and once
	// with "tmp" (temporary, auth 20). Equal counts → protection must win.
	paths := []string{
		"/vol/归档/tietest/a.txt",
		"/vol/tmp/tietest/b.txt",
	}
	seedFiles(t, st, "local", paths)
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		stats, err := Learn(ctx, st)
		if err != nil {
			t.Fatalf("Learn run %d: %v", i, err)
		}
		ds, ok := findStat(stats, "tietest")
		if !ok {
			t.Fatalf("run %d: expected stat for tietest", i)
		}
		if ds.SuggestedRole != domain.RoleFormalArchive {
			t.Fatalf("run %d: tietest role = %s, want formal_archive (deterministic tie-break)", i, ds.SuggestedRole)
		}
	}
}

// TestGenerateDrafts_ReRunUpdatesDraft verifies re-running on the same data
// updates existing drafts (same ID) rather than duplicating.
func TestGenerateDrafts_ReRunUpdatesDraft(t *testing.T) {
	st := newLearnStore(t)
	paths := []string{
		"/vol/归档/PRJ-2024-001/deliverables/a.txt",
		"/vol/归档/PRJ-2024-002/deliverables/b.txt",
	}
	seedFiles(t, st, "local", paths)
	ctx := context.Background()

	stats, _ := Learn(ctx, st)
	if _, err := GenerateDrafts(ctx, st, stats, "batch-1"); err != nil {
		t.Fatalf("first GenerateDrafts: %v", err)
	}
	stats2, _ := Learn(ctx, st)
	if _, err := GenerateDrafts(ctx, st, stats2, "batch-2"); err != nil {
		t.Fatalf("second GenerateDrafts: %v", err)
	}
	// Should still have exactly one draft rule for deliverables (updated, not duplicated).
	drafts, err := st.ListRules(ctx, domain.RuleSourceLearned, domain.RuleDraft)
	if err != nil {
		t.Fatalf("list drafts: %v", err)
	}
	count := 0
	for _, r := range drafts {
		if strings.Contains(r.Definition, "deliverables") {
			count++
			if r.BatchID != "batch-2" {
				t.Errorf("deliverables rule BatchID = %q, want batch-2 (updated)", r.BatchID)
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 deliverables draft after re-run, got %d", count)
	}
}
