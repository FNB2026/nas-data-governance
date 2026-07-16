package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

type failingJournal struct {
	beginErr error
	doneErr  error
}

func (j failingJournal) BeginJournal(context.Context, string, string, []domain.PlannedAction) error {
	return j.beginErr
}
func (j failingJournal) MarkJournalDone(context.Context, string, int, string) error {
	return j.doneErr
}
func (failingJournal) MarkJournalFailed(context.Context, string, int) error { return nil }
func (failingJournal) MarkJournalRolledBack(context.Context, string, int, error) error {
	return nil
}
func (failingJournal) ListJournalDone(context.Context, string) ([]store.JournalEntry, error) {
	return nil, nil
}
func (failingJournal) ListJournalPending(context.Context, string) ([]store.JournalEntry, error) {
	return nil, nil
}
func (failingJournal) ListExecutingPlans(context.Context) ([]string, error) { return nil, nil }

// newTestStore opens a fresh SQLite store in a temp dir for executor tests.
func newTestStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "exec.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// seedPlanWithTask creates a task and saves one approved plan, returning
// the plan and taskID. The plan has the given actions.
func seedPlanWithTask(t *testing.T, st *store.SQLiteStore, planID string, actions []domain.PlannedAction) (domain.OperationPlan, string) {
	t.Helper()
	ctx := context.Background()
	taskID := "task-" + planID
	if err := st.CreateTask(ctx, domain.OperationTask{
		ID: taskID, RootPath: "/", State: "executing", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	plan := domain.OperationPlan{
		ID:      planID,
		TaskID:  taskID,
		State:   domain.PlanApproved,
		Actions: actions,
	}
	if err := st.SavePlans(ctx, taskID, []domain.OperationPlan{plan}); err != nil {
		t.Fatalf("save plans: %v", err)
	}
	return plan, taskID
}

// --- Execute with journal integration tests ---

// TestExecuteWithJournal_Success verifies that a successful execution
// records done entries in the journal with the actual target path.
func TestExecuteWithJournal_Success(t *testing.T) {
	st := newTestStore(t)
	srcDir := t.TempDir()
	qDir := t.TempDir()

	src := filepath.Join(srcDir, "file.txt")
	os.WriteFile(src, []byte("content"), 0o644)
	snap, _ := Snapshot(src, true)
	file := domain.FileInstance{
		Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt,
		Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash,
	}
	actions := []domain.PlannedAction{
		{Path: src, Action: domain.OperationQuarantine, File: file, Reason: "test"},
	}
	plan, taskID := seedPlanWithTask(t, st, "journal-success", actions)

	exec, err := NewWithJournal(QuarantineConfig{
		Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{srcDir},
	}, st)
	if err != nil {
		t.Fatal(err)
	}

	result := exec.Execute(context.Background(), &plan)
	if result.Err != nil {
		t.Fatalf("execute: %v", result.Err)
	}
	if plan.State != domain.PlanVerified {
		t.Fatalf("expected VERIFIED, got %s", plan.State)
	}

	// Journal should have one done entry with the actual quarantine target.
	done, err := st.ListJournalDone(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("list journal done: %v", err)
	}
	if len(done) != 1 {
		t.Fatalf("expected 1 done entry, got %d", len(done))
	}
	if done[0].TargetPath == "" {
		t.Fatal("done entry should have a non-empty target_path")
	}
	if done[0].TaskID != taskID {
		t.Fatalf("expected task_id %s, got %s", taskID, done[0].TaskID)
	}

	// The quarantined file should exist at the recorded target.
	if _, err := os.Stat(done[0].TargetPath); err != nil {
		t.Fatalf("file should exist at journal target: %v", err)
	}
}

func TestExecuteStopsBeforeWriteWhenBeginJournalFails(t *testing.T) {
	srcDir := t.TempDir()
	qDir := t.TempDir()
	src := filepath.Join(srcDir, "file.txt")
	if err := os.WriteFile(src, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, _ := Snapshot(src, true)
	plan := domain.OperationPlan{ID: "journal-begin-fail", TaskID: "task", State: domain.PlanApproved, Actions: []domain.PlannedAction{{
		Path: src, Action: domain.OperationQuarantine,
		File: domain.FileInstance{Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt, Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash},
	}}}
	exec, err := NewWithJournal(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{srcDir}}, failingJournal{beginErr: errors.New("disk full")})
	if err != nil {
		t.Fatal(err)
	}
	result := exec.Execute(context.Background(), &plan)
	if result.ErrorType != "journal_begin_failed" || plan.State != domain.PlanApproved {
		t.Fatalf("result=%#v state=%s", result, plan.State)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source changed despite journal failure: %v", err)
	}
}

func TestExecuteRollsBackWhenJournalDoneFails(t *testing.T) {
	srcDir := t.TempDir()
	qDir := t.TempDir()
	src := filepath.Join(srcDir, "file.txt")
	if err := os.WriteFile(src, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	snap, _ := Snapshot(src, true)
	plan := domain.OperationPlan{ID: "journal-done-fail", TaskID: "task", State: domain.PlanApproved, Actions: []domain.PlannedAction{{
		Path: src, Action: domain.OperationQuarantine,
		File: domain.FileInstance{Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt, Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash},
	}}}
	exec, err := NewWithJournal(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{srcDir}}, failingJournal{doneErr: errors.New("disk full")})
	if err != nil {
		t.Fatal(err)
	}
	result := exec.Execute(context.Background(), &plan)
	if result.ErrorType != "journal_done_failed" || plan.State != domain.PlanRolledBack {
		t.Fatalf("result=%#v state=%s", result, plan.State)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source was not rolled back: %v", err)
	}
}

// TestExecuteWithJournal_FailureMarksRolledBack verifies that when an
// action fails mid-execution, the journal records the first action as
// done (then rolled back) and the failing action as failed.
func TestExecuteWithJournal_FailureMarksRolledBack(t *testing.T) {
	st := newTestStore(t)
	srcDir := t.TempDir()
	qDir := t.TempDir()

	src1 := filepath.Join(srcDir, "a.txt")
	src2 := filepath.Join(srcDir, "b.txt")
	blocker := filepath.Join(srcDir, "blocker.txt")
	os.WriteFile(src1, []byte("file a"), 0o644)
	os.WriteFile(src2, []byte("file b"), 0o644)
	os.WriteFile(blocker, []byte("blocker"), 0o644)

	snap1, _ := Snapshot(src1, true)
	snap2, _ := Snapshot(src2, true)
	file1 := domain.FileInstance{Path: src1, Size: snap1.Size, ModifiedAt: snap1.ModifiedAt, Device: snap1.Device, Inode: snap1.Inode, ContentSHA256: snap1.Hash}
	file2 := domain.FileInstance{Path: src2, Size: snap2.Size, ModifiedAt: snap2.ModifiedAt, Device: snap2.Device, Inode: snap2.Inode, ContentSHA256: snap2.Hash}

	actions := []domain.PlannedAction{
		{Path: src1, Action: domain.OperationQuarantine, File: file1},
		{Path: src2, Action: domain.OperationMove, TargetPath: blocker, File: file2},
	}
	plan, _ := seedPlanWithTask(t, st, "journal-fail", actions)

	exec, err := NewWithJournal(QuarantineConfig{
		Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{srcDir},
	}, st)
	if err != nil {
		t.Fatal(err)
	}

	result := exec.Execute(context.Background(), &plan)
	if result.Err == nil {
		t.Fatal("expected execution failure from MOVE to existing target")
	}
	if plan.State != domain.PlanRolledBack {
		t.Fatalf("expected ROLLED_BACK, got %s", plan.State)
	}

	// Journal: action 0 should be done with rollback_status=done.
	// Action 1 should be failed.
	all, err := st.ListJournalAll(context.Background(), plan.ID)
	if err != nil {
		t.Fatalf("list journal all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 journal entries, got %d", len(all))
	}
	if all[0].Status != "done" {
		t.Fatalf("action 0 status: expected done, got %s", all[0].Status)
	}
	if all[0].RollbackStatus != "done" {
		t.Fatalf("action 0 rollback_status: expected done, got %s", all[0].RollbackStatus)
	}
	if all[1].Status != "failed" {
		t.Fatalf("action 1 status: expected failed, got %s", all[1].Status)
	}

	// Source 1 should be restored (rolled back).
	if _, err := os.Stat(src1); err != nil {
		t.Fatalf("src1 should be restored by rollback: %v", err)
	}
}

// --- BeginJournal idempotency test ---

// TestBeginJournal_Idempotent verifies that calling BeginJournal twice
// does not duplicate entries (UNIQUE constraint + INSERT OR IGNORE).
func TestBeginJournal_Idempotent(t *testing.T) {
	st := newTestStore(t)
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "f.txt")
	os.WriteFile(src, []byte("x"), 0o644)
	snap, _ := Snapshot(src, true)
	file := domain.FileInstance{Path: src, Size: snap.Size, ContentSHA256: snap.Hash}
	actions := []domain.PlannedAction{
		{Path: src, Action: domain.OperationQuarantine, File: file},
	}
	plan, _ := seedPlanWithTask(t, st, "idempotent", actions)
	ctx := context.Background()

	// Call BeginJournal twice.
	if err := st.BeginJournal(ctx, plan.TaskID, plan.ID, actions); err != nil {
		t.Fatalf("first begin: %v", err)
	}
	if err := st.BeginJournal(ctx, plan.TaskID, plan.ID, actions); err != nil {
		t.Fatalf("second begin: %v", err)
	}

	all, err := st.ListJournalAll(ctx, plan.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 entry after double begin, got %d", len(all))
	}
}

// --- Recover tests ---

// TestRecover_RollsBackDoneActions simulates a crash: a plan is in
// EXECUTING state with one done journal entry (file was quarantined).
// Recover() should undo the quarantine and mark the plan ROLLED_BACK.
func TestRecover_RollsBackDoneActions(t *testing.T) {
	st := newTestStore(t)
	srcDir := t.TempDir()
	qDir := t.TempDir()
	ctx := context.Background()

	src := filepath.Join(srcDir, "crashed.txt")
	os.WriteFile(src, []byte("original"), 0o644)
	snap, _ := Snapshot(src, true)
	file := domain.FileInstance{
		Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt,
		Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash,
	}
	actions := []domain.PlannedAction{
		{Path: src, Action: domain.OperationQuarantine, File: file},
	}
	plan, taskID := seedPlanWithTask(t, st, "crash-rollback", actions)

	// Simulate: executor started, quarantined the file, marked journal
	// done, but crashed before transitioning to VERIFIED.
	if err := st.BeginJournal(ctx, taskID, plan.ID, actions); err != nil {
		t.Fatal(err)
	}
	// The file was moved to quarantine.
	qPath := filepath.Join(qDir, "crashed.txt")
	if err := os.MkdirAll(qDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(src, qPath); err != nil {
		t.Fatal(err)
	}
	// Mark done with the actual target.
	if err := st.MarkJournalDone(ctx, plan.ID, 0, qPath); err != nil {
		t.Fatal(err)
	}
	// Set plan to EXECUTING (crash left it here).
	if err := st.UpdatePlanState(ctx, plan.ID, domain.PlanExecuting); err != nil {
		t.Fatal(err)
	}

	// Source should be gone (file is in quarantine).
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source should not exist before recovery, err=%v", err)
	}

	// Recover.
	exec := NewForRecovery()
	results := exec.Recover(ctx, st)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Action != RecoveryRolledBack {
		t.Fatalf("expected rolled_back, got %s", results[0].Action)
	}
	if results[0].RolledBack != 1 {
		t.Fatalf("expected 1 rolled back, got %d", results[0].RolledBack)
	}
	if len(results[0].Errors) > 0 {
		t.Fatalf("unexpected errors: %v", results[0].Errors)
	}

	// Source should be restored.
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("source should be restored: %v", err)
	}
	if string(got) != "original" {
		t.Fatalf("content mismatch: %q", got)
	}
	// Quarantine should be empty.
	entries, _ := os.ReadDir(qDir)
	if len(entries) != 0 {
		t.Fatalf("quarantine should be empty, got %d", len(entries))
	}

	// Plan should be ROLLED_BACK.
	updated, err := st.GetPlan(ctx, plan.ID)
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if updated.State != domain.PlanRolledBack {
		t.Fatalf("expected ROLLED_BACK, got %s", updated.State)
	}

	// Journal should show rollback_status=done.
	done, _ := st.ListJournalDone(ctx, plan.ID)
	if len(done) != 1 {
		t.Fatalf("expected 1 done entry, got %d", len(done))
	}
	if done[0].RollbackStatus != "done" {
		t.Fatalf("expected rollback_status=done, got %s", done[0].RollbackStatus)
	}
}

// TestRecover_ResetsToApprovedWhenNoDoneActions simulates a crash where
// the plan was in EXECUTING but no actions had completed. Recover()
// should reset it to APPROVED for re-execution.
func TestRecover_ResetsToApprovedWhenNoDoneActions(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "notyet.txt")
	os.WriteFile(src, []byte("untouched"), 0o644)
	snap, _ := Snapshot(src, true)
	file := domain.FileInstance{
		Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt,
		Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash,
	}
	actions := []domain.PlannedAction{
		{Path: src, Action: domain.OperationQuarantine, File: file},
	}
	plan, taskID := seedPlanWithTask(t, st, "crash-noop", actions)

	// Simulate: journal was written (pending) but no action completed.
	if err := st.BeginJournal(ctx, taskID, plan.ID, actions); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePlanState(ctx, plan.ID, domain.PlanExecuting); err != nil {
		t.Fatal(err)
	}

	exec := NewForRecovery()
	results := exec.Recover(ctx, st)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Action != RecoveryResetToApproved {
		t.Fatalf("expected reset_to_approved, got %s", results[0].Action)
	}

	// Plan should be APPROVED.
	updated, _ := st.GetPlan(ctx, plan.ID)
	if updated.State != domain.PlanApproved {
		t.Fatalf("expected APPROVED, got %s", updated.State)
	}

	// Source should be untouched.
	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("source should exist: %v", err)
	}
	if string(got) != "untouched" {
		t.Fatalf("content changed: %q", got)
	}
}

// TestRecover_NoJournalEntriesResetsToApproved verifies that a plan in
// EXECUTING with NO journal entries (crash before BeginJournal) is reset
// to APPROVED.
func TestRecover_NoJournalEntriesResetsToApproved(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	actions := []domain.PlannedAction{
		{Path: "/nonexistent/file.txt", Action: domain.OperationQuarantine},
	}
	plan, _ := seedPlanWithTask(t, st, "crash-pre-begin", actions)
	if err := st.UpdatePlanState(ctx, plan.ID, domain.PlanExecuting); err != nil {
		t.Fatal(err)
	}

	exec := NewForRecovery()
	results := exec.Recover(ctx, st)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Action != RecoveryResetToApproved {
		t.Fatalf("expected reset_to_approved, got %s", results[0].Action)
	}
}

// TestRecover_Idempotent verifies that calling Recover() twice is safe:
// the second call finds no EXECUTING plans and does nothing.
func TestRecover_Idempotent(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "idem.txt")
	os.WriteFile(src, []byte("data"), 0o644)
	snap, _ := Snapshot(src, true)
	file := domain.FileInstance{
		Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt,
		Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash,
	}
	actions := []domain.PlannedAction{
		{Path: src, Action: domain.OperationQuarantine, File: file},
	}
	plan, taskID := seedPlanWithTask(t, st, "crash-idem", actions)

	// Set up crash state with one done action.
	if err := st.BeginJournal(ctx, taskID, plan.ID, actions); err != nil {
		t.Fatal(err)
	}
	qPath := filepath.Join(t.TempDir(), "idem.txt")
	os.Rename(src, qPath)
	if err := st.MarkJournalDone(ctx, plan.ID, 0, qPath); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdatePlanState(ctx, plan.ID, domain.PlanExecuting); err != nil {
		t.Fatal(err)
	}

	exec := NewForRecovery()

	// First recovery: should roll back.
	results1 := exec.Recover(ctx, st)
	if len(results1) != 1 || results1[0].Action != RecoveryRolledBack {
		t.Fatalf("first recover: expected 1 rolled_back, got %+v", results1)
	}

	// Second recovery: no EXECUTING plans left.
	results2 := exec.Recover(ctx, st)
	if len(results2) != 0 {
		t.Fatalf("second recover: expected 0 results, got %d", len(results2))
	}
}

// TestRecover_NoExecutingPlansReturnsEmpty verifies that Recover() returns
// an empty list when there are no plans in EXECUTING state.
func TestRecover_NoExecutingPlansReturnsEmpty(t *testing.T) {
	st := newTestStore(t)
	exec := NewForRecovery()
	results := exec.Recover(context.Background(), st)
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

// --- Backward compatibility test ---

// TestExecuteWithoutJournal_BackwardCompatible verifies that Execute()
// still works when no journal is configured (executor created via New()).
func TestExecuteWithoutJournal_BackwardCompatible(t *testing.T) {
	srcDir := t.TempDir()
	qDir := t.TempDir()
	src := filepath.Join(srcDir, "nojournal.txt")
	os.WriteFile(src, []byte("content"), 0o644)
	snap, _ := Snapshot(src, true)
	file := domain.FileInstance{
		Path: src, Size: snap.Size, ModifiedAt: snap.ModifiedAt,
		Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash,
	}
	plan := &domain.OperationPlan{
		ID: "no-journal", State: domain.PlanApproved,
		Actions: []domain.PlannedAction{
			{Path: src, Action: domain.OperationQuarantine, File: file},
		},
	}
	exec, err := New(QuarantineConfig{Root: qDir, Structure: QuarantineFlat, SourceRoots: []string{srcDir}})
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
}
