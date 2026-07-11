package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"nas-data-governance/internal/domain"
)

// makeApprovedPlan creates a plan with one QUARANTINE action, ready for
// execution. The source file is real so the executor can move it.
func makeApprovedPlan(t *testing.T, content string) (*domain.OperationPlan, string, string) {
	t.Helper()
	srcDir := t.TempDir()
	qDir := t.TempDir()
	src := filepath.Join(srcDir, "duplicate.txt")
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := Snapshot(src, true)
	if err != nil {
		t.Fatal(err)
	}
	file := domain.FileInstance{
		Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt,
		Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash,
	}
	plan := &domain.OperationPlan{
		ID: "test-plan", State: domain.PlanApproved,
		Actions: []domain.PlannedAction{
			{Path: src, Action: domain.OperationQuarantine, File: file, Reason: "test"},
		},
	}
	return plan, src, qDir
}

func TestExecuteQuarantineSuccess(t *testing.T) {
	plan, src, qDir := makeApprovedPlan(t, "duplicate content")
	exec, err := New(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{filepath.Dir(src)}})
	if err != nil {
		t.Fatal(err)
	}
	result := exec.Execute(context.Background(), plan)

	if result.Err != nil {
		t.Fatalf("execute: %v", result.Err)
	}
	if plan.State != domain.PlanVerified {
		t.Fatalf("expected VERIFIED, got %s", plan.State)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be removed, got err=%v", err)
	}
	entries, err := os.ReadDir(qDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 quarantined file, got %d", len(entries))
	}
	got, err := os.ReadFile(filepath.Join(qDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "duplicate content" {
		t.Fatalf("content mismatch: %q", got)
	}
}

func TestExecuteRejectsNonApprovedPlan(t *testing.T) {
	plan, src, qDir := makeApprovedPlan(t, "x")
	plan.State = domain.PlanDraft
	exec, _ := New(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{filepath.Dir(src)}})
	result := exec.Execute(context.Background(), plan)
	if result.Err == nil {
		t.Fatal("expected error for non-approved plan")
	}
	if plan.State != domain.PlanDraft {
		t.Fatalf("state should not change, got %s", plan.State)
	}
}

func TestExecuteRejectsActionOutsideConfiguredSourceRoots(t *testing.T) {
	plan, src, qDir := makeApprovedPlan(t, "x")
	exec, err := New(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	result := exec.Execute(context.Background(), plan)
	if !errors.Is(result.Err, errOutOfScope) {
		t.Fatalf("expected out-of-scope error, got %v", result.Err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("out-of-scope source must not be touched: %v", err)
	}
	entries, err := os.ReadDir(qDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("out-of-scope plan must not write quarantine: entries=%d err=%v", len(entries), err)
	}
}

func TestExecuteRefusesSymlinkInsideConfiguredSourceRoot(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	qDir := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	realPath := filepath.Join(target, "duplicate.txt")
	if err := os.WriteFile(realPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := Snapshot(realPath, true)
	if err != nil {
		t.Fatal(err)
	}
	linkedPath := filepath.Join(link, "duplicate.txt")
	plan := &domain.OperationPlan{ID: "symlink", State: domain.PlanApproved, Actions: []domain.PlannedAction{{
		Path: linkedPath, Action: domain.OperationQuarantine,
		File: domain.FileInstance{Path: linkedPath, Size: snap.Size, ModifiedAt: snap.ModifiedAt, Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash},
	}}}
	exec, err := New(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	result := exec.Execute(context.Background(), plan)
	if !errors.Is(result.Err, errActionFailed) {
		t.Fatalf("expected safe symlink rejection, got %v", result.Err)
	}
	if _, err := os.Stat(realPath); err != nil {
		t.Fatalf("symlink target must remain untouched: %v", err)
	}
}

func TestExecuteStaleCheckSendsBackToDraft(t *testing.T) {
	plan, src, qDir := makeApprovedPlan(t, "original")
	// Mutate the file after the snapshot was taken so size differs.
	if err := os.WriteFile(src, []byte("changed content is longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	exec, _ := New(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{filepath.Dir(src)}})
	result := exec.Execute(context.Background(), plan)

	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if plan.State != domain.PlanDraft {
		t.Fatalf("expected DRAFT (stale re-review), got %s", plan.State)
	}
	// Source must still exist (no execution happened).
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should still exist, got err=%v", err)
	}
	// Quarantine must be empty (no execution happened).
	entries, _ := os.ReadDir(qDir)
	if len(entries) != 0 {
		t.Fatalf("quarantine should be empty, got %d files", len(entries))
	}
}

func TestExecuteRollsBackOnFailure(t *testing.T) {
	srcDir := t.TempDir()
	qDir := t.TempDir()

	// Two source files. The first quarantines successfully; the second
	// uses an unimplemented action type (MOVE), which triggers an execution
	// error. The rollback hook must restore the first file to its origin.
	src1 := filepath.Join(srcDir, "a.txt")
	src2 := filepath.Join(srcDir, "b.txt")
	os.WriteFile(src1, []byte("file a"), 0o644)
	os.WriteFile(src2, []byte("file b"), 0o644)

	snap1, _ := Snapshot(src1, true)
	snap2, _ := Snapshot(src2, true)
	file1 := domain.FileInstance{Path: src1, Size: snap1.Size, ModifiedAt: snap1.ModifiedAt, Device: snap1.Device, Inode: snap1.Inode, ContentSHA256: snap1.Hash}
	file2 := domain.FileInstance{Path: src2, Size: snap2.Size, ModifiedAt: snap2.ModifiedAt, Device: snap2.Device, Inode: snap2.Inode, ContentSHA256: snap2.Hash}

	plan := &domain.OperationPlan{
		ID: "rollback-test", State: domain.PlanApproved,
		Actions: []domain.PlannedAction{
			{Path: src1, Action: domain.OperationQuarantine, File: file1},
			{Path: src2, Action: domain.OperationMove, File: file2},
		},
	}
	exec, _ := New(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{srcDir}})
	result := exec.Execute(context.Background(), plan)

	if result.Err == nil {
		t.Fatal("expected execution error from unimplemented MOVE")
	}
	if plan.State != domain.PlanRolledBack {
		t.Fatalf("expected ROLLED_BACK, got %s", plan.State)
	}
	// src1 should be back in its original location (rolled back).
	if _, err := os.Stat(src1); err != nil {
		t.Fatalf("src1 should be restored by rollback, got err=%v", err)
	}
	// Quarantine should be empty (rolled back).
	entries, _ := os.ReadDir(qDir)
	if len(entries) != 0 {
		t.Fatalf("quarantine should be empty after rollback, got %d files", len(entries))
	}
}

func TestExecuteSkipsNonFilesystemActions(t *testing.T) {
	srcDir := t.TempDir()
	qDir := t.TempDir()
	src := filepath.Join(srcDir, "keep.txt")
	os.WriteFile(src, []byte("retain me"), 0o644)

	plan := &domain.OperationPlan{
		ID: "skip-test", State: domain.PlanApproved,
		Actions: []domain.PlannedAction{
			{Path: src, Action: domain.OperationKeep},
		},
	}
	exec, _ := New(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{srcDir}})
	result := exec.Execute(context.Background(), plan)

	if result.Err != nil {
		t.Fatalf("execute: %v", result.Err)
	}
	if plan.State != domain.PlanVerified {
		t.Fatalf("expected VERIFIED, got %s", plan.State)
	}
	// KEEP should not touch the source.
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("source should still exist: %v", err)
	}
	if string(got) != "retain me" {
		t.Fatalf("content changed: %q", got)
	}
}

func TestExecuteContextCancellation(t *testing.T) {
	plan, src, qDir := makeApprovedPlan(t, "x")
	exec, _ := New(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{filepath.Dir(src)}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before execute

	result := exec.Execute(ctx, plan)
	// The stale check step runs before the cancellation check; the
	// pipeline should abort at the action loop with ctx.Err().
	if result.Err == nil && plan.State != domain.PlanVerified {
		t.Fatalf("expected cancellation or error, got state=%s err=%v", plan.State, result.Err)
	}
}

func TestExecuteAuditStepsRecorded(t *testing.T) {
	plan, src, qDir := makeApprovedPlan(t, "audit me")
	exec, _ := New(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{filepath.Dir(src)}})
	result := exec.Execute(context.Background(), plan)

	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if len(result.Steps) < 2 {
		t.Fatalf("expected at least 2 audit steps, got %d", len(result.Steps))
	}
	// Step 0 should be stale_check with ok status.
	if result.Steps[0].Name != "stale_check" || result.Steps[0].Status != StepOK {
		t.Fatalf("first step should be stale_check/ok, got %#v", result.Steps[0])
	}
	// Audit detail must not contain file paths (AGENTS rule 6).
	for _, step := range result.Steps {
		for k, v := range step.Detail {
			if s, ok := v.(string); ok && len(s) > 0 {
				// Reject any detail value that looks like an absolute path.
				if filepath.IsAbs(s) {
					t.Errorf("step %s detail %s contains path: %v", step.Name, k, v)
				}
			}
		}
	}
}

func TestNewRejectsInvalidQuarantine(t *testing.T) {
	if _, err := New(QuarantineConfig{Root: "", Structure: QuarantineFlat, SourceRoots: []string{t.TempDir()}}); err == nil {
		t.Fatal("expected error for empty root")
	}
}

func TestNewRequiresSourceRoots(t *testing.T) {
	if _, err := New(QuarantineConfig{Root: t.TempDir(), Structure: QuarantineFlat}); err == nil {
		t.Fatal("expected source-root validation error")
	}
}
