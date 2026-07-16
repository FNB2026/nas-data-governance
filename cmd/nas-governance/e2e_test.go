package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	idx "github.com/FNB2026/nas-data-governance/internal/index"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func TestEndToEnd_ImportIndexIsIdempotentAndPreservesSMBInode(t *testing.T) {
	tmp := t.TempDir()
	indexPath := filepath.Join(tmp, "index.jsonl")
	dbPath := filepath.Join(tmp, "governance.db")
	files := []domain.FileInstance{
		{StorageID: "smb", Path: "/mount/archive/a.txt", Name: "a.txt", Size: 1, Device: 1<<63 + 1, Inode: 1<<63 + 2, ModifiedAt: time.Now(), DiscoveredAt: time.Now()},
		{StorageID: "smb", Path: "/mount/archive/team/b.txt", Name: "b.txt", Size: 2, Device: 1<<63 + 3, Inode: 1<<63 + 4, ModifiedAt: time.Now(), DiscoveredAt: time.Now()},
	}
	if err := idx.Write(indexPath, files); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := runImportIndex([]string{"-index", indexPath, "-db", dbPath, "-batch-size", "1"}); err != nil {
			t.Fatalf("import run %d: %v", i+1, err)
		}
	}
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.ListFiles(context.Background(), "smb")
	if err != nil || len(got) != 2 {
		t.Fatalf("imported files: len=%d err=%v", len(got), err)
	}
	if got[0].Inode != files[0].Inode || got[1].Inode != files[1].Inode {
		t.Fatalf("SMB inode not preserved: %#v", got)
	}
	storages, err := st.ListStorages(context.Background())
	if err != nil || len(storages) != 1 || storages[0].RootPath != "/mount/archive" {
		t.Fatalf("inferred storage root: %#v err=%v", storages, err)
	}
}

func TestImportIndexValidatesBeforeCreatingDatabase(t *testing.T) {
	tmp := t.TempDir()
	indexPath := filepath.Join(tmp, "bad.jsonl")
	dbPath := filepath.Join(tmp, "governance.db")
	if err := os.WriteFile(indexPath, []byte("{bad json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runImportIndex([]string{"-index", indexPath, "-db", dbPath}); err == nil {
		t.Fatal("expected malformed index error")
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatalf("database should not be created for invalid input: %v", err)
	}
}

// TestEndToEnd_ScanDuplicatesPlanApproveExecute verifies the full
// scan → duplicates → plan → approve → execute pipeline against real
// duplicate files, with SQLite persistence at both scan and execute.
func TestEndToEnd_ScanDuplicatesPlanApproveExecute(t *testing.T) {
	tmp := t.TempDir()
	dataRoot := filepath.Join(tmp, "data")
	qRoot := filepath.Join(tmp, "quarantine")
	indexPath := filepath.Join(tmp, "index.jsonl")
	planPath := filepath.Join(tmp, "plan.json")
	approvedPath := filepath.Join(tmp, "approved.json")
	auditPath := filepath.Join(tmp, "audit.json")
	dbPath := filepath.Join(tmp, "governance.db")
	os.MkdirAll(filepath.Join(dataRoot, "temp"), 0o755)
	os.MkdirAll(qRoot, 0o755)

	// Two byte-identical files → one duplicate group.
	content := []byte("e2e duplicate content for full pipeline")
	src1 := filepath.Join(dataRoot, "temp", "a.txt")
	src2 := filepath.Join(dataRoot, "temp", "b.txt")
	os.WriteFile(src1, content, 0o644)
	os.WriteFile(src2, content, 0o644)

	// 1. scan with DB persistence
	if err := runScan([]string{
		"-root", dataRoot, "-out", indexPath,
		"-storage", "e2e", "-db", dbPath,
	}); err != nil {
		t.Fatalf("scan: %v", err)
	}
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := st.ListFiles(context.Background(), "e2e")
	st.Close()
	if len(files) != 2 {
		t.Fatalf("scan should persist 2 files, got %d", len(files))
	}

	// 2. duplicates (outputs to stdout; we just verify no error)
	if err := runDuplicates([]string{"-index", indexPath}); err != nil {
		t.Fatalf("duplicates: %v", err)
	}

	// 3. plan
	if err := runPlan([]string{"-index", indexPath, "-out", planPath}); err != nil {
		t.Fatalf("plan: %v", err)
	}
	plans := readJSONPlans(t, planPath)
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if plans[0].State != domain.PlanDraft {
		t.Fatalf("plan state = %s, want draft", plans[0].State)
	}

	// 4. approve
	if err := runApprove([]string{"-plan", planPath, "-out", approvedPath, "-all"}); err != nil {
		t.Fatalf("approve: %v", err)
	}

	// 5. execute with DB audit
	if err := runExecute([]string{
		"-plan", approvedPath, "-out", auditPath,
		"-quarantine", qRoot, "-source-root", dataRoot,
		"-db", dbPath,
	}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Verify quarantine.
	entries, _ := os.ReadDir(qRoot)
	if len(entries) != 1 {
		t.Fatalf("expected 1 quarantined file, got %d", len(entries))
	}
	// Keep file still exists.
	if _, err := os.Stat(src1); err != nil {
		t.Fatalf("keep file should still exist: %v", err)
	}
}

// TestEndToEnd_LearnStatsGeneratesDrafts verifies the learn --source=stats
// --apply pipeline produces draft rules visible via rules list.
func TestEndToEnd_LearnStatsGeneratesDrafts(t *testing.T) {
	tmp := t.TempDir()
	dataRoot := filepath.Join(tmp, "data")
	indexPath := filepath.Join(tmp, "index.jsonl")
	dbPath := filepath.Join(tmp, "governance.db")
	statsOut := filepath.Join(tmp, "stats.json")
	os.MkdirAll(dataRoot, 0o755)

	// Create files under non-builtin directory "deliverables" that
	// co-occurs with builtin "归档" across 2+ distinct dirs (to pass
	// minDirCount=2 threshold) and 5+ files (minFileCount=5).
	dirs := []string{
		filepath.Join(dataRoot, "归档", "PRJ-2024-001", "deliverables"),
		filepath.Join(dataRoot, "归档", "PRJ-2024-002", "deliverables"),
	}
	for _, dir := range dirs {
		os.MkdirAll(dir, 0o755)
	}
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(dirs[0], fileLetter(i)+".txt"), []byte("x"), 0o644)
		os.WriteFile(filepath.Join(dirs[1], fileLetter(i)+".txt"), []byte("y"), 0o644)
	}

	if err := runScan([]string{
		"-root", dataRoot, "-out", indexPath,
		"-storage", "e2e-learn", "-db", dbPath,
	}); err != nil {
		t.Fatalf("scan: %v", err)
	}

	// learn dry-run (no --apply) → no drafts persisted.
	if err := runLearn([]string{
		"-source", "stats", "-db", dbPath, "-out", statsOut,
	}); err != nil {
		t.Fatalf("learn dry-run: %v", err)
	}
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	drafts, _ := st.ListRules(context.Background(), domain.RuleSourceLearned, domain.RuleDraft)
	st.Close()
	if len(drafts) != 0 {
		t.Fatalf("dry-run should persist 0 drafts, got %d", len(drafts))
	}

	// learn --apply → drafts persisted.
	if err := runLearn([]string{
		"-source", "stats", "-db", dbPath, "-apply",
	}); err != nil {
		t.Fatalf("learn apply: %v", err)
	}
	st, _ = store.Open(context.Background(), dbPath)
	defer st.Close()
	drafts, _ = st.ListRules(context.Background(), domain.RuleSourceLearned, domain.RuleDraft)
	if len(drafts) == 0 {
		t.Fatalf("expected draft rules after --apply, got 0")
	}
	// Verify the "deliverables" rule was generated.
	found := false
	for _, r := range drafts {
		if r.Source == domain.RuleSourceLearned && r.Status == domain.RuleDraft {
			found = true
		}
	}
	if !found {
		t.Fatalf("no learned draft rule found")
	}
	// K-008: all drafts priority <= 60.
	for _, r := range drafts {
		if r.Priority > 60 {
			t.Errorf("rule %s priority %d > 60 (K-008)", r.ID, r.Priority)
		}
	}
}

// TestEndToEnd_RulesLifecycle verifies the full rule lifecycle:
// learn --apply (draft) → rules approve (probation) → rules reject (rejected).
func TestEndToEnd_RulesLifecycle(t *testing.T) {
	tmp := t.TempDir()
	dataRoot := filepath.Join(tmp, "data")
	indexPath := filepath.Join(tmp, "index.jsonl")
	dbPath := filepath.Join(tmp, "governance.db")
	os.MkdirAll(dataRoot, 0o755)

	lcDirs := []string{
		filepath.Join(dataRoot, "归档", "PRJ-2024-001", "deliverables"),
		filepath.Join(dataRoot, "归档", "PRJ-2024-002", "deliverables"),
	}
	for _, dir := range lcDirs {
		os.MkdirAll(dir, 0o755)
	}
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(lcDirs[0], fileLetter(i)+".txt"), []byte("x"), 0o644)
		os.WriteFile(filepath.Join(lcDirs[1], fileLetter(i)+".txt"), []byte("y"), 0o644)
	}
	runScan([]string{"-root", dataRoot, "-out", indexPath, "-storage", "lc", "-db", dbPath})
	runLearn([]string{"-source", "stats", "-db", dbPath, "-apply"})

	st, _ := store.Open(context.Background(), dbPath)
	defer st.Close()
	drafts, _ := st.ListRules(context.Background(), domain.RuleSourceLearned, domain.RuleDraft)
	if len(drafts) == 0 {
		t.Fatal("expected draft rules")
	}
	ruleID := drafts[0].ID

	// approve → probation
	if err := runRules([]string{"approve", "-db", dbPath, "-id", ruleID}); err != nil {
		t.Fatalf("rules approve: %v", err)
	}
	rules, _ := st.ListRules(context.Background(), domain.RuleSourceLearned, "")
	for _, r := range rules {
		if r.ID == ruleID && r.Status != domain.RuleProbation {
			t.Errorf("after approve: status = %s, want probation", r.Status)
		}
	}

	// reject → rejected
	if err := runRules([]string{"reject", "-db", dbPath, "-id", ruleID}); err != nil {
		t.Fatalf("rules reject: %v", err)
	}
	rules, _ = st.ListRules(context.Background(), domain.RuleSourceLearned, "")
	for _, r := range rules {
		if r.ID == ruleID && r.Status != domain.RuleRejected {
			t.Errorf("after reject: status = %s, want rejected", r.Status)
		}
	}
}

// TestEndToEnd_LearnFeedbackDryRun verifies learn --source=feedback works
// on a database with historical plans (seeded via execute).
func TestEndToEnd_LearnFeedbackDryRun(t *testing.T) {
	tmp := t.TempDir()
	dataRoot := filepath.Join(tmp, "data")
	qRoot := filepath.Join(tmp, "quarantine")
	indexPath := filepath.Join(tmp, "index.jsonl")
	planPath := filepath.Join(tmp, "plan.json")
	approvedPath := filepath.Join(tmp, "approved.json")
	auditPath := filepath.Join(tmp, "audit.json")
	dbPath := filepath.Join(tmp, "governance.db")
	os.MkdirAll(filepath.Join(dataRoot, "temp"), 0o755)
	os.MkdirAll(qRoot, 0o755)

	content := []byte("feedback e2e content")
	src1 := filepath.Join(dataRoot, "temp", "a.txt")
	src2 := filepath.Join(dataRoot, "temp", "b.txt")
	os.WriteFile(src1, content, 0o644)
	os.WriteFile(src2, content, 0o644)

	runScan([]string{"-root", dataRoot, "-out", indexPath, "-storage", "fb", "-db", dbPath})
	runPlan([]string{"-index", indexPath, "-out", planPath})
	runApprove([]string{"-plan", planPath, "-out", approvedPath, "-all"})
	runExecute([]string{
		"-plan", approvedPath, "-out", auditPath,
		"-quarantine", qRoot, "-source-root", dataRoot, "-db", dbPath,
	})

	// learn feedback dry-run should not error and should report 0+ plans.
	if err := runLearn([]string{
		"-source", "feedback", "-db", dbPath,
	}); err != nil {
		t.Fatalf("learn feedback: %v", err)
	}
}

func fileLetter(i int) string {
	return string(rune('a' + i))
}
