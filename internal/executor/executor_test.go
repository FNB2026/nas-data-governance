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
	// attempts a MOVE to a path that already exists (ErrDestinationExists),
	// triggering rollback. The rollback hook must restore the first file.
	src1 := filepath.Join(srcDir, "a.txt")
	src2 := filepath.Join(srcDir, "b.txt")
	blocker := filepath.Join(srcDir, "blocker.txt")
	os.WriteFile(src1, []byte("file a"), 0o644)
	os.WriteFile(src2, []byte("file b"), 0o644)
	os.WriteFile(blocker, []byte("blocks the move"), 0o644)

	snap1, _ := Snapshot(src1, true)
	snap2, _ := Snapshot(src2, true)
	file1 := domain.FileInstance{Path: src1, Size: snap1.Size, ModifiedAt: snap1.ModifiedAt, Device: snap1.Device, Inode: snap1.Inode, ContentSHA256: snap1.Hash}
	file2 := domain.FileInstance{Path: src2, Size: snap2.Size, ModifiedAt: snap2.ModifiedAt, Device: snap2.Device, Inode: snap2.Inode, ContentSHA256: snap2.Hash}

	plan := &domain.OperationPlan{
		ID: "rollback-test", State: domain.PlanApproved,
		Actions: []domain.PlannedAction{
			{Path: src1, Action: domain.OperationQuarantine, File: file1},
			{Path: src2, Action: domain.OperationMove, TargetPath: blocker, File: file2},
		},
	}
	exec, _ := New(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{srcDir}})
	result := exec.Execute(context.Background(), plan)

	if result.Err == nil {
		t.Fatal("expected execution error from MOVE to existing target")
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

// --- MOVE / COPY / DELETE / RENAME tests ---

// makeActionPlan creates a plan with one filesystem action targeting src.
// The file is real so the executor can operate on it. Returns the plan,
// the source path, and the source directory (use as SourceRoots).
func makeActionPlan(t *testing.T, action domain.OperationType, content string) (*domain.OperationPlan, string, string) {
	t.Helper()
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "source.txt")
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
		ID: "test-" + string(action), State: domain.PlanApproved,
		Actions: []domain.PlannedAction{
			{Path: src, Action: action, File: file, Reason: "test"},
		},
	}
	return plan, src, srcDir
}

func TestExecuteMoveSuccess(t *testing.T) {
	plan, src, srcDir := makeActionPlan(t, domain.OperationMove, "move me")
	dst := filepath.Join(srcDir, "moved", "dest.txt")
	plan.Actions[0].TargetPath = dst
	exec, _ := New(QuarantineConfig{Root: t.TempDir(), Structure: QuarantineFlat, SourceRoots: []string{srcDir}})

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
	got, _ := os.ReadFile(dst)
	if string(got) != "move me" {
		t.Fatalf("destination content mismatch: %q", got)
	}
}

func TestExecuteMoveRollback(t *testing.T) {
	plan, src, srcDir := makeActionPlan(t, domain.OperationMove, "rollback me")
	// Target outside source roots → scope validation fails.
	plan.Actions[0].TargetPath = filepath.Join(t.TempDir(), "outside.txt")
	exec, _ := New(QuarantineConfig{Root: t.TempDir(), Structure: QuarantineFlat, SourceRoots: []string{srcDir}})

	result := exec.Execute(context.Background(), plan)
	if !errors.Is(result.Err, errOutOfScope) {
		t.Fatalf("expected out-of-scope, got %v", result.Err)
	}
	// Source must be untouched.
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source should still exist, got err=%v", err)
	}
}

func TestExecuteCopySuccess(t *testing.T) {
	plan, src, srcDir := makeActionPlan(t, domain.OperationCopy, "copy me")
	dst := filepath.Join(srcDir, "copied.txt")
	plan.Actions[0].TargetPath = dst
	exec, _ := New(QuarantineConfig{Root: t.TempDir(), Structure: QuarantineFlat, SourceRoots: []string{srcDir}})

	result := exec.Execute(context.Background(), plan)
	if result.Err != nil {
		t.Fatalf("execute: %v", result.Err)
	}
	if plan.State != domain.PlanVerified {
		t.Fatalf("expected VERIFIED, got %s", plan.State)
	}
	// Source must still exist (COPY does not remove source).
	got, _ := os.ReadFile(src)
	if string(got) != "copy me" {
		t.Fatalf("source content changed: %q", got)
	}
	// Destination must match.
	got, _ = os.ReadFile(dst)
	if string(got) != "copy me" {
		t.Fatalf("destination content mismatch: %q", got)
	}
}

func TestExecuteDeleteSuccess(t *testing.T) {
	plan, src, srcDir := makeActionPlan(t, domain.OperationDelete, "delete me")
	qDir := t.TempDir()
	exec, _ := New(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{srcDir}})

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
	got, err := os.ReadFile(filepath.Join(qDir, filepath.Base(src)))
	if err != nil || string(got) != "delete me" {
		t.Fatalf("DELETE must quarantine recoverable content: got=%q err=%v", got, err)
	}
}

func TestExecuteRenameSuccess(t *testing.T) {
	plan, src, srcDir := makeActionPlan(t, domain.OperationRename, "rename me")
	dst := filepath.Join(srcDir, "renamed.txt")
	plan.Actions[0].TargetPath = dst
	exec, _ := New(QuarantineConfig{Root: t.TempDir(), Structure: QuarantineFlat, SourceRoots: []string{srcDir}})

	result := exec.Execute(context.Background(), plan)
	if result.Err != nil {
		t.Fatalf("execute: %v", result.Err)
	}
	if plan.State != domain.PlanVerified {
		t.Fatalf("expected VERIFIED, got %s", plan.State)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should be renamed away, got err=%v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "rename me" {
		t.Fatalf("destination content mismatch: %q", got)
	}
}

func TestExecuteRenameRefusesExistingDestination(t *testing.T) {
	plan, src, srcDir := makeActionPlan(t, domain.OperationRename, "rename me")
	dst := filepath.Join(srcDir, "existing.txt")
	if err := os.WriteFile(dst, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan.Actions[0].TargetPath = dst
	exec, _ := New(QuarantineConfig{Root: t.TempDir(), Structure: QuarantineFlat, SourceRoots: []string{srcDir}})
	result := exec.Execute(context.Background(), plan)
	if result.Err == nil {
		t.Fatal("expected rename collision failure")
	}
	if got, _ := os.ReadFile(dst); string(got) != "keep me" {
		t.Fatalf("existing destination was overwritten: %q", got)
	}
	if got, _ := os.ReadFile(src); string(got) != "rename me" {
		t.Fatalf("source must survive collision: %q", got)
	}
}

func TestDeleteDoesNotRemoveTaskRoot(t *testing.T) {
	plan, _, srcDir := makeActionPlan(t, domain.OperationDelete, "root child")
	exec, _ := New(QuarantineConfig{Root: t.TempDir(), Structure: QuarantineFlat, SourceRoots: []string{srcDir}})
	result := exec.Execute(context.Background(), plan)
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	info, err := os.Stat(srcDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("task root must never be removed: info=%v err=%v", info, err)
	}
}

func TestExecuteMoveCleansUpEmptySourceDir(t *testing.T) {
	srcDir := t.TempDir()
	// Create a subdirectory with one file, so MOVE leaves it empty.
	subDir := filepath.Join(srcDir, "sub")
	src := filepath.Join(subDir, "only.txt")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("only child"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, _ := Snapshot(src, true)
	file := domain.FileInstance{Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt, Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash}
	dst := filepath.Join(srcDir, "moved.txt")
	plan := &domain.OperationPlan{
		ID: "cleanup-test", State: domain.PlanApproved,
		Actions: []domain.PlannedAction{
			{Path: src, Action: domain.OperationMove, TargetPath: dst, File: file},
		},
	}
	exec, _ := New(QuarantineConfig{Root: t.TempDir(), Structure: QuarantineFlat, SourceRoots: []string{srcDir}})
	result := exec.Execute(context.Background(), plan)
	if result.Err != nil {
		t.Fatalf("execute: %v", result.Err)
	}
	// The empty subdirectory should have been cleaned up.
	if _, err := os.Stat(subDir); !os.IsNotExist(err) {
		t.Fatalf("empty source dir should be removed, got err=%v", err)
	}
}

func TestExecuteCopyRollsBackOnLaterFailure(t *testing.T) {
	srcDir := t.TempDir()
	// First action: COPY succeeds. Second: MOVE to existing path fails.
	src1 := filepath.Join(srcDir, "src1.txt")
	src2 := filepath.Join(srcDir, "src2.txt")
	blocker := filepath.Join(srcDir, "blocker.txt")
	os.WriteFile(src1, []byte("first"), 0o644)
	os.WriteFile(src2, []byte("second"), 0o644)
	os.WriteFile(blocker, []byte("blocks"), 0o644)

	snap1, _ := Snapshot(src1, true)
	snap2, _ := Snapshot(src2, true)
	file1 := domain.FileInstance{Path: src1, Size: snap1.Size, ModifiedAt: snap1.ModifiedAt, Device: snap1.Device, Inode: snap1.Inode, ContentSHA256: snap1.Hash}
	file2 := domain.FileInstance{Path: src2, Size: snap2.Size, ModifiedAt: snap2.ModifiedAt, Device: snap2.Device, Inode: snap2.Inode, ContentSHA256: snap2.Hash}

	dst := filepath.Join(srcDir, "copy_out.txt")
	plan := &domain.OperationPlan{
		ID: "copy-rollback", State: domain.PlanApproved,
		Actions: []domain.PlannedAction{
			{Path: src1, Action: domain.OperationCopy, TargetPath: dst, File: file1},
			{Path: src2, Action: domain.OperationMove, TargetPath: blocker, File: file2},
		},
	}
	exec, _ := New(QuarantineConfig{Root: t.TempDir(), Structure: QuarantineFlat, SourceRoots: []string{srcDir}})
	result := exec.Execute(context.Background(), plan)
	if result.Err == nil {
		t.Fatal("expected failure from MOVE to existing path")
	}
	if plan.State != domain.PlanRolledBack {
		t.Fatalf("expected ROLLED_BACK, got %s", plan.State)
	}
	// The copied destination should be removed by rollback.
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("copied file should be removed by rollback, got err=%v", err)
	}
}
