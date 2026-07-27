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

func seedPurgeExecution(t *testing.T) (*store.SQLiteStore, string, domain.QuarantineItem, domain.PurgePlan, time.Time) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	root := t.TempDir()
	path := filepath.Join(root, "expired.dat")
	if err := os.WriteFile(path, []byte("permanent test"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := Snapshot(path, true)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "purge.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	taskID, operationPlanID := "task-purge", "operation-purge"
	if err := st.CreateTask(ctx, domain.OperationTask{
		ID: taskID, RootPath: "/synthetic", State: "completed", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	action := domain.PlannedAction{
		Path: "/synthetic/expired.dat", Action: domain.OperationQuarantine,
		Context: domain.DirectoryContext{Role: domain.RoleTemporary},
		File: domain.FileInstance{
			Path: "/synthetic/expired.dat", Size: snapshot.Size, ContentSHA256: snapshot.Hash,
		},
	}
	operationPlan := domain.OperationPlan{
		ID: operationPlanID, TaskID: taskID, State: domain.PlanApproved,
		Actions: []domain.PlannedAction{action},
	}
	if err := st.SavePlans(ctx, taskID, []domain.OperationPlan{operationPlan}); err != nil {
		t.Fatal(err)
	}
	if err := st.BeginJournal(ctx, taskID, operationPlanID, operationPlan.Actions); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkJournalDone(ctx, operationPlanID, 0, path); err != nil {
		t.Fatal(err)
	}
	items, err := st.RegisterQuarantinesFromJournal(
		ctx, operationPlanID, now.Add(-48*time.Hour), now.Add(-24*time.Hour),
	)
	if err != nil || len(items) != 1 {
		t.Fatalf("register quarantine: items=%d err=%v", len(items), err)
	}
	item := items[0]
	plan := domain.PurgePlan{
		ID: "purge-test", ItemID: item.ID, State: domain.PurgeDraft,
		ExpectedPath: item.QuarantinePath, ExpectedSHA256: item.ContentSHA256,
		ExpectedSize: item.FileSize, RetainUntil: item.RetainUntil,
		ApprovalDigest: "digest", CreatedAt: now,
	}
	if err := st.SavePurgePlans(ctx, []domain.PurgePlan{plan}); err != nil {
		t.Fatal(err)
	}
	if err := st.ApprovePurgePlan(ctx, plan.ID, plan.ApprovalDigest, now); err != nil {
		t.Fatal(err)
	}
	plan, _ = st.GetPurgePlan(ctx, plan.ID)
	item, _ = st.GetQuarantineItem(ctx, item.ID)
	return st, root, item, plan, now
}

func TestPurgeDryRunAndPermanentCommit(t *testing.T) {
	st, root, item, plan, now := seedPurgeExecution(t)
	exec, err := NewPurgeExecutor(root, st)
	if err != nil {
		t.Fatal(err)
	}
	exec.now = func() time.Time { return now }
	dry := exec.ValidatePurge(context.Background(), plan, item)
	if dry.Err != nil {
		t.Fatalf("dry-run: %v", dry.Err)
	}
	if _, err := os.Stat(item.QuarantinePath); err != nil {
		t.Fatalf("dry-run changed item: %v", err)
	}
	result := exec.ExecutePurge(context.Background(), &plan, &item)
	if result.Err != nil || result.FinalState != domain.PurgeCommitted {
		t.Fatalf("purge result: %#v", result)
	}
	if _, err := os.Stat(item.QuarantinePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("purged file still exists: %v", err)
	}
	stored, err := st.GetQuarantineItem(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.QuarantinePurged || stored.PurgedAt == nil {
		t.Fatalf("item not committed in store: %#v", stored)
	}
}

func TestPurgeStaleContentIsBlockedWithoutWrites(t *testing.T) {
	st, root, item, plan, now := seedPurgeExecution(t)
	if err := os.WriteFile(item.QuarantinePath, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	exec, _ := NewPurgeExecutor(root, st)
	exec.now = func() time.Time { return now }
	result := exec.ExecutePurge(context.Background(), &plan, &item)
	if result.ErrorType != "preflight_failed" {
		t.Fatalf("expected preflight failure, got %#v", result)
	}
	if _, err := os.Stat(item.QuarantinePath); err != nil {
		t.Fatalf("stale item was moved or removed: %v", err)
	}
	entries, err := st.ListRecoverablePurges(context.Background())
	if err != nil || len(entries) != 0 {
		t.Fatalf("stale purge must not create journal: entries=%d err=%v", len(entries), err)
	}
}

type stagedFailStore struct {
	begin      bool
	rolledBack bool
}

func (s *stagedFailStore) BeginPurge(context.Context, domain.PurgePlan, domain.QuarantineItem, string, time.Time) error {
	s.begin = true
	return nil
}
func (*stagedFailStore) MarkPurgeStaged(context.Context, string) error {
	return errors.New("journal unavailable")
}
func (*stagedFailStore) MarkPurgeCommitPending(context.Context, string) error { return nil }
func (*stagedFailStore) MarkPurgeCommitted(context.Context, string, string, time.Time) error {
	return nil
}
func (s *stagedFailStore) MarkPurgeRolledBack(context.Context, string, string, time.Time) error {
	s.rolledBack = true
	return nil
}
func (*stagedFailStore) ListRecoverablePurges(context.Context) ([]domain.PurgeJournalEntry, error) {
	return nil, nil
}

func TestPurgeRollsBackWhenStagedJournalFails(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rollback.dat")
	if err := os.WriteFile(path, []byte("rollback"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := Snapshot(path, true)
	now := time.Now().UTC()
	item := domain.QuarantineItem{
		ID: "q", QuarantinePath: path, ContentSHA256: snapshot.Hash, FileSize: snapshot.Size,
		Status: domain.QuarantinePurgeEligible, RetainUntil: now.Add(-time.Hour),
	}
	plan := domain.PurgePlan{
		ID: "purge-rollback", ItemID: item.ID, State: domain.PurgeApproved,
		ExpectedPath: path, ExpectedSHA256: item.ContentSHA256,
		ExpectedSize: item.FileSize, RetainUntil: item.RetainUntil,
	}
	fake := &stagedFailStore{}
	exec, _ := NewPurgeExecutor(root, fake)
	exec.now = func() time.Time { return now }
	result := exec.ExecutePurge(context.Background(), &plan, &item)
	if result.ErrorType != "journal_staged_failed" || !fake.begin || !fake.rolledBack {
		t.Fatalf("unexpected result/store: %#v %#v", result, fake)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "rollback" {
		t.Fatalf("pre-commit rollback did not restore item: %q %v", got, err)
	}
}

type restoreCompleteFailStore struct {
	began      bool
	rolledBack bool
}

func (s *restoreCompleteFailStore) BeginRestore(context.Context, domain.RestorePlan, domain.QuarantineItem, time.Time) error {
	s.began = true
	return nil
}
func (*restoreCompleteFailStore) MarkRestoreCompleted(context.Context, string, string, time.Time) error {
	return errors.New("database full")
}
func (s *restoreCompleteFailStore) MarkRestoreRolledBack(context.Context, string, time.Time) error {
	s.rolledBack = true
	return nil
}
func (*restoreCompleteFailStore) ListPendingRestores(context.Context) ([]domain.RestoreJournalEntry, error) {
	return nil, nil
}

func TestRestoreCompletionFailureRollsBackToQuarantine(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "source")
	quarantineRoot := filepath.Join(tmp, "quarantine")
	if err := os.Mkdir(sourceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(quarantineRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	qPath := filepath.Join(quarantineRoot, "restore.dat")
	sourcePath := filepath.Join(sourceRoot, "restore.dat")
	if err := os.WriteFile(qPath, []byte("restore rollback"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := Snapshot(qPath, true)
	item := domain.QuarantineItem{
		ID: "q-restore", SourcePath: sourcePath, QuarantinePath: qPath,
		ContentSHA256: snapshot.Hash, FileSize: snapshot.Size,
		Status: domain.QuarantineActive,
	}
	plan := domain.RestorePlan{
		ID: "restore-fail", ItemID: item.ID, State: domain.RestoreApproved,
		QuarantinePath: qPath, RestorePath: sourcePath,
		ExpectedSHA256: item.ContentSHA256, ExpectedSize: item.FileSize,
	}
	fake := &restoreCompleteFailStore{}
	exec, err := NewRestoreExecutor(quarantineRoot, []string{sourceRoot}, fake)
	if err != nil {
		t.Fatal(err)
	}
	result := exec.ExecuteRestore(context.Background(), &plan, &item)
	if result.ErrorType != "journal_complete_failed" || !fake.began || !fake.rolledBack {
		t.Fatalf("unexpected restore result: %#v fake=%#v", result, fake)
	}
	if _, err := os.Stat(qPath); err != nil {
		t.Fatalf("quarantine item was not restored after failure: %v", err)
	}
	if _, err := os.Stat(sourcePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restore destination survived rollback: %v", err)
	}
}

func TestRecoverPurgeRollsStagedItemBack(t *testing.T) {
	st, root, item, plan, now := seedPurgeExecution(t)
	stageDir := filepath.Join(root, ".purge-staging")
	if err := os.Mkdir(stageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stagePath := filepath.Join(stageDir, plan.ID+".stage")
	if err := st.BeginPurge(context.Background(), plan, item, stagePath, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(item.QuarantinePath, stagePath); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkPurgeStaged(context.Background(), plan.ID); err != nil {
		t.Fatal(err)
	}
	exec, _ := NewPurgeExecutor(root, st)
	exec.now = func() time.Time { return now }
	results := exec.RecoverPurges(context.Background())
	if len(results) != 1 || results[0].FinalState != domain.PurgeRolledBack {
		t.Fatalf("unexpected recovery results: %#v", results)
	}
	if _, err := os.Stat(item.QuarantinePath); err != nil {
		t.Fatalf("staged item not returned to quarantine: %v", err)
	}
}

func TestRecoverPurgeFinalizesCompletedUnlink(t *testing.T) {
	st, root, item, plan, now := seedPurgeExecution(t)
	stageDir := filepath.Join(root, ".purge-staging")
	if err := os.Mkdir(stageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	stagePath := filepath.Join(stageDir, plan.ID+".stage")
	if err := st.BeginPurge(context.Background(), plan, item, stagePath, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(item.QuarantinePath, stagePath); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkPurgeStaged(context.Background(), plan.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkPurgeCommitPending(context.Background(), plan.ID); err != nil {
		t.Fatal(err)
	}
	if err := SafeRemove(stagePath); err != nil {
		t.Fatal(err)
	}
	exec, _ := NewPurgeExecutor(root, st)
	exec.now = func() time.Time { return now }
	results := exec.RecoverPurges(context.Background())
	if len(results) != 1 || results[0].FinalState != domain.PurgeCommitted {
		t.Fatalf("unexpected recovery results: %#v", results)
	}
	stored, err := st.GetQuarantineItem(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.QuarantinePurged {
		t.Fatalf("commit was not reconciled: %#v", stored)
	}
}
