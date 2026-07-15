package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/executor"
	"nas-data-governance/internal/store"
)

func TestScanErrorSummariesOmitSensitivePaths(t *testing.T) {
	var out bytes.Buffer
	reportHashErrors(&out, []error{fmt.Errorf("open /private/client/secret.wav: denied")})
	reportScanErrors(&out, 2)
	if bytes.Contains(out.Bytes(), []byte("/private")) || bytes.Contains(out.Bytes(), []byte("secret.wav")) {
		t.Fatalf("sensitive path leaked: %s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("paths omitted")) {
		t.Fatalf("expected explicit omission notice: %s", out.String())
	}
}

func TestAnalyzePersistenceErrorSummariesOmitSensitivePaths(t *testing.T) {
	var out bytes.Buffer
	reportAnalyzePersistenceErrors(&out, 2, 3)
	if bytes.Contains(out.Bytes(), []byte("secret")) || !bytes.Contains(out.Bytes(), []byte("source paths omitted")) {
		t.Fatalf("unexpected persistence warning: %s", out.String())
	}
}

// TestApproveThenExecuteEndToEnd verifies the full CLI pipeline:
// plan.json → approve → execute → audit.json
func TestApproveThenExecuteEndToEnd(t *testing.T) {
	tmp := t.TempDir()
	dataRoot := filepath.Join(tmp, "data")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(dataRoot, 0o755)
	os.MkdirAll(qRoot, 0o755)

	// Create two duplicate files in a temp/cache directory.
	content := []byte("duplicate bytes for e2e test")
	src1 := filepath.Join(dataRoot, "temp", "a.txt")
	src2 := filepath.Join(dataRoot, "temp", "b.txt")
	os.MkdirAll(filepath.Dir(src1), 0o755)
	os.WriteFile(src1, content, 0o644)
	os.WriteFile(src2, content, 0o644)

	// Build a plan directly (skipping scan/duplicates for this test).
	snap1, _ := executor.Snapshot(src1, true)
	snap2, _ := executor.Snapshot(src2, true)
	file1 := domain.FileInstance{Path: src1, Size: snap1.Size, ModifiedAt: snap1.ModifiedAt, Device: snap1.Device, Inode: snap1.Inode, ContentSHA256: snap1.Hash}
	file2 := domain.FileInstance{Path: src2, Size: snap2.Size, ModifiedAt: snap2.ModifiedAt, Device: snap2.Device, Inode: snap2.Inode, ContentSHA256: snap2.Hash}
	plans := []domain.OperationPlan{{
		ID: "dup-e2etest01", State: domain.PlanDraft, ContentSHA256: snap1.Hash,
		Size: snap1.Size, Risk: domain.RiskMedium, RetainPath: src1,
		Actions: []domain.PlannedAction{
			{Path: src1, Action: domain.OperationKeep, File: file1, Reason: "retain"},
			{Path: src2, Action: domain.OperationQuarantine, File: file2, Reason: "quarantine duplicate"},
		},
		Evidence: []string{"e2e test"},
	}}
	planPath := filepath.Join(tmp, "plan.json")
	writeJSON(t, planPath, plans)

	// Step 1: approve
	approvedPath := filepath.Join(tmp, "approved.json")
	if err := runApprove([]string{"-plan", planPath, "-out", approvedPath, "-all"}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	approved := readJSONPlans(t, approvedPath)
	if len(approved) != 1 || approved[0].State != domain.PlanApproved {
		t.Fatalf("expected 1 approved plan, got %d plans, state=%s", len(approved), approved[0].State)
	}

	// Step 2: execute
	auditPath := filepath.Join(tmp, "audit.json")
	dbPath := filepath.Join(tmp, "audit.db")
	if err := runExecute([]string{
		"-plan", approvedPath,
		"-out", auditPath,
		"-quarantine", qRoot,
		"-source-root", dataRoot,
		"-db", dbPath,
	}); err != nil {
		t.Fatalf("execute: %v", err)
	}

	// Verify source file removed, quarantined file exists.
	if _, err := os.Stat(src2); !os.IsNotExist(err) {
		t.Fatalf("src2 should be quarantined (removed), got err=%v", err)
	}
	entries, _ := os.ReadDir(qRoot)
	if len(entries) != 1 {
		t.Fatalf("expected 1 quarantined file, got %d", len(entries))
	}
	// Keep file should still exist.
	if _, err := os.Stat(src1); err != nil {
		t.Fatalf("src1 (keep) should still exist: %v", err)
	}

	// Verify audit JSON.
	var results []executor.Result
	readJSONFile(t, auditPath, &results)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].FinalState != domain.PlanVerified {
		t.Fatalf("expected VERIFIED, got %s", results[0].FinalState)
	}

	// Verify SQLite audit logs.
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	logs, err := st.ListLogs(context.Background(), "dup-e2etest01")
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) < 2 {
		t.Fatalf("expected at least 2 audit logs in db, got %d", len(logs))
	}
}

func TestScanPersistsFilesForAnalyzeWorkflow(t *testing.T) {
	tmp := t.TempDir()
	dataRoot := filepath.Join(tmp, "data")
	if err := os.MkdirAll(filepath.Join(dataRoot, "归档"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "归档", "sample.pdf"), []byte("%PDF-1.4\n/Count 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(tmp, "index.jsonl")
	dbPath := filepath.Join(tmp, "governance.db")
	if err := runScan([]string{"-root", dataRoot, "-out", indexPath, "-storage", "test", "-db", dbPath}); err != nil {
		t.Fatalf("scan with db: %v", err)
	}
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	files, err := st.ListFiles(context.Background(), "test")
	_ = st.Close()
	if err != nil || len(files) != 1 {
		t.Fatalf("persisted files: len=%d err=%v", len(files), err)
	}
	if err := runAnalyze([]string{"-index", indexPath, "-out", filepath.Join(tmp, "formats.json"), "-db", dbPath, "-storage-id", "test"}); err != nil {
		t.Fatalf("analyze after scan --db: %v", err)
	}
	info, err := os.Stat(filepath.Join(tmp, "formats.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != privateFileMode {
		t.Fatalf("format report mode=%o, want %o", info.Mode().Perm(), privateFileMode)
	}
}

func TestAnalyzeResumeReusesDatabaseWithoutSourceRead(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "sample.pdf")
	if err := os.WriteFile(source, []byte("%PDF-1.4\n/Count 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(tmp, "index.jsonl")
	dbPath := filepath.Join(tmp, "governance.db")
	if err := runScan([]string{"--root", root, "--out", indexPath, "--storage", "resume-test", "--db", dbPath}); err != nil {
		t.Fatal(err)
	}
	if err := runAnalyze([]string{"--index", indexPath, "--out", filepath.Join(tmp, "first.json"), "--db", dbPath, "--batch-size", "1"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	resumedReport := filepath.Join(tmp, "resumed.json")
	if err := runAnalyze([]string{"--index", indexPath, "--out", resumedReport, "--db", dbPath, "--resume"}); err != nil {
		t.Fatalf("resume should not read removed source: %v", err)
	}
	var entries []formatReportEntry
	readJSONFile(t, resumedReport, &entries)
	if len(entries) != 1 || entries[0].Format.Format != "pdf" || entries[0].Error != "" {
		t.Fatalf("unexpected resumed report: %#v", entries)
	}
}

func TestAnalyzeResumeRequiresDatabase(t *testing.T) {
	if err := runAnalyze([]string{"--resume"}); err == nil {
		t.Fatal("expected --resume without --db to fail")
	}
	if err := runAnalyze([]string{"--refresh-metadata"}); err == nil {
		t.Fatal("expected --refresh-metadata without --resume to fail")
	}
}

func TestNeedsMetadataRefreshOnlyRetriesSupportedGaps(t *testing.T) {
	if !needsMetadataRefresh(domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio}) {
		t.Fatal("wav duration gap should refresh")
	}
	if !needsMetadataRefresh(domain.FormatInfo{Format: "mp4", Category: domain.CategoryVideo, Duration: 2}) {
		t.Fatal("mp4 dimension gap should refresh")
	}
	if needsMetadataRefresh(domain.FormatInfo{Format: "mpeg", Category: domain.CategoryVideo, Width: 1920, Height: 1080}) {
		t.Fatal("mpeg with supported dimensions filled must not retry for unsupported duration")
	}
	if needsMetadataRefresh(domain.FormatInfo{Format: "flv", Category: domain.CategoryVideo}) {
		t.Fatal("unsupported flv metadata must not retry forever")
	}
}

func TestAnalyzeRefreshMetadataUpdatesPersistedMediaOnly(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "root")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	wav := make([]byte, 44)
	copy(wav[:4], "RIFF")
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint32(wav[24:28], 48000)
	binary.LittleEndian.PutUint32(wav[28:32], 96000)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], 192000)
	source := filepath.Join(root, "sample.wav")
	if err := os.WriteFile(source, wav, 0o600); err != nil {
		t.Fatal(err)
	}
	indexPath, dbPath := filepath.Join(tmp, "index.jsonl"), filepath.Join(tmp, "governance.db")
	if err := runScan([]string{"--root", root, "--out", indexPath, "--storage", "metadata", "--db", dbPath}); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.FileID(context.Background(), "metadata", source)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveFormat(context.Background(), id, domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	if err := runAnalyze([]string{"--index", indexPath, "--out", filepath.Join(tmp, "formats.json"), "--db", dbPath, "--resume", "--refresh-metadata"}); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	records, err := st.ListFormats(context.Background(), "metadata")
	_ = st.Close()
	if err != nil || len(records) != 1 || records[0].Info.Duration != 2 || records[0].Info.Codec != "pcm" {
		t.Fatalf("records=%#v err=%v", records, err)
	}
}

// TestApproveRejectsNonDraftPlan verifies the approve command enforces
// state machine rules.
func TestApproveRejectsNonDraftPlan(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "plan.json")
	plans := []domain.OperationPlan{{ID: "p1", State: domain.PlanApproved}}
	writeJSON(t, planPath, plans)
	err := runApprove([]string{"-plan", planPath, "-out", filepath.Join(tmp, "out.json"), "-all"})
	if err == nil {
		t.Fatal("expected error approving non-draft plan")
	}
}

// TestExecuteRejectsMissingRequiredFlags verifies execute enforces
// required flags.
func TestExecuteRejectsMissingRequiredFlags(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "plan.json")
	writeJSON(t, planPath, []domain.OperationPlan{})
	cases := [][]string{
		{"-plan", planPath},                          // missing quarantine + source-root
		{"-plan", planPath, "-quarantine", "/q"},     // missing source-root
		{"-plan", planPath, "-source-root", "/data"}, // missing quarantine
	}
	for i, args := range cases {
		if err := runExecute(args); err == nil {
			t.Errorf("case %d: expected error for missing required flag", i)
		}
	}
}

// TestExecuteDryRunSkipsFilesystem verifies --dry-run does not move files.
func TestExecuteDryRunSkipsFilesystem(t *testing.T) {
	tmp := t.TempDir()
	dataRoot := filepath.Join(tmp, "data")
	qRoot := filepath.Join(tmp, "quarantine")
	os.MkdirAll(dataRoot, 0o755)
	os.MkdirAll(qRoot, 0o755)

	content := []byte("dry run content")
	src := filepath.Join(dataRoot, "temp", "f.txt")
	os.MkdirAll(filepath.Dir(src), 0o755)
	os.WriteFile(src, content, 0o644)
	snap, _ := executor.Snapshot(src, true)
	file := domain.FileInstance{Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt, Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash}
	plans := []domain.OperationPlan{{
		ID: "dry-run-test", State: domain.PlanApproved,
		Actions: []domain.PlannedAction{{Path: src, Action: domain.OperationQuarantine, File: file}},
	}}
	planPath := filepath.Join(tmp, "plan.json")
	writeJSON(t, planPath, plans)

	auditPath := filepath.Join(tmp, "audit.json")
	if err := runExecute([]string{
		"-plan", planPath, "-out", auditPath,
		"-quarantine", qRoot, "-source-root", dataRoot, "-dry-run",
	}); err != nil {
		t.Fatalf("execute dry-run: %v", err)
	}
	// Source must still exist.
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should still exist after dry-run: %v", err)
	}
	// Quarantine must be empty.
	entries, _ := os.ReadDir(qRoot)
	if len(entries) != 0 {
		t.Fatalf("quarantine should be empty after dry-run, got %d", len(entries))
	}
	f, err := os.Open(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var results []executor.Result
	if err := json.NewDecoder(f).Decode(&results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || len(results[0].Steps) < 2 || results[0].Steps[0].Name != "stale_check" || results[0].Steps[1].Name != "dry_run" {
		t.Fatalf("dry-run must perform read-only preflight: %#v", results)
	}
}

// TestExecuteSkipsNonApprovedPlans verifies execute skips plans that are
// not in APPROVED state.
func TestExecuteSkipsNonApprovedPlans(t *testing.T) {
	tmp := t.TempDir()
	dataRoot := filepath.Join(tmp, "data")
	qRoot := filepath.Join(tmp, "q")
	os.MkdirAll(dataRoot, 0o755)
	os.MkdirAll(qRoot, 0o755)
	src := filepath.Join(dataRoot, "f.txt")
	os.WriteFile(src, []byte("x"), 0o644)
	snap, _ := executor.Snapshot(src, true)
	file := domain.FileInstance{Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt, Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash}
	plans := []domain.OperationPlan{{
		ID: "draft-only", State: domain.PlanDraft,
		Actions: []domain.PlannedAction{{Path: src, Action: domain.OperationQuarantine, File: file}},
	}}
	planPath := filepath.Join(tmp, "plan.json")
	writeJSON(t, planPath, plans)

	auditPath := filepath.Join(tmp, "audit.json")
	if err := runExecute([]string{
		"-plan", planPath, "-out", auditPath,
		"-quarantine", qRoot, "-source-root", dataRoot,
	}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	// Source must still exist (plan was skipped).
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should still exist: %v", err)
	}
}

func TestApproveSelectsByPlanID(t *testing.T) {
	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "plan.json")
	outPath := filepath.Join(tmp, "approved.json")
	plans := []domain.OperationPlan{
		{ID: "p1", State: domain.PlanDraft},
		{ID: "p2", State: domain.PlanDraft},
		{ID: "p3", State: domain.PlanDraft},
	}
	writeJSON(t, planPath, plans)
	if err := runApprove([]string{"-plan", planPath, "-out", outPath, "-plan-id", "p2"}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	approved := readJSONPlans(t, outPath)
	if len(approved) != 1 || approved[0].ID != "p2" {
		t.Fatalf("expected only p2 approved, got %#v", approved)
	}
}

// --- helpers ---

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readJSONPlans(t *testing.T, path string) []domain.OperationPlan {
	t.Helper()
	var plans []domain.OperationPlan
	readJSONFile(t, path, &plans)
	return plans
}

func readJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(fmt.Errorf("open %s: %w", path, err))
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(v); err != nil {
		t.Fatal(fmt.Errorf("decode %s: %w", path, err))
	}
}
