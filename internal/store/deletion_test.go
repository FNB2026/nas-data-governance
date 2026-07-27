package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func seedManagedQuarantine(t *testing.T, st *SQLiteStore, protected bool, now time.Time) domain.QuarantineItem {
	t.Helper()
	ctx := context.Background()
	taskID := "task-m7"
	planID := "plan-m7"
	if err := st.CreateTask(ctx, domain.OperationTask{
		ID: taskID, RootPath: "/synthetic", State: "executing", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	action := domain.PlannedAction{
		Path: "/synthetic/source.dat", Action: domain.OperationQuarantine,
		Context: domain.DirectoryContext{Role: domain.RoleTemporary, Protected: protected},
		File: domain.FileInstance{
			Path: "/synthetic/source.dat", Size: 3, ContentSHA256: "abc",
		},
	}
	plan := domain.OperationPlan{
		ID: planID, TaskID: taskID, State: domain.PlanApproved,
		Actions: []domain.PlannedAction{action},
	}
	if err := st.SavePlans(ctx, taskID, []domain.OperationPlan{plan}); err != nil {
		t.Fatal(err)
	}
	if err := st.BeginJournal(ctx, taskID, planID, plan.Actions); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkJournalDone(ctx, planID, 0, "/managed-quarantine/source.dat"); err != nil {
		t.Fatal(err)
	}
	items, err := st.RegisterQuarantinesFromJournal(
		ctx, planID, now.Add(-48*time.Hour), now.Add(-24*time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one registered item, got %d", len(items))
	}
	return items[0]
}

func TestRegisterQuarantinesFromJournalIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "m7.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now().UTC()
	item := seedManagedQuarantine(t, st, false, now)
	if item.Status != domain.QuarantineActive {
		t.Fatalf("expected active quarantine, got %s", item.Status)
	}
	again, err := st.RegisterQuarantinesFromJournal(
		ctx, item.PlanID, item.QuarantinedAt, item.RetainUntil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("expected idempotent registration, got %d new rows", len(again))
	}
}

func TestProtectedQuarantineEntersHold(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "m7-hold.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	item := seedManagedQuarantine(t, st, true, time.Now().UTC())
	if item.Status != domain.QuarantineHold || item.HoldReason != "protected_context" {
		t.Fatalf("protected item did not enter HOLD: %#v", item)
	}
}

func TestPurgePlanApprovalIsSingleDigestBound(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, filepath.Join(t.TempDir(), "m7-approve.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now().UTC()
	item := seedManagedQuarantine(t, st, false, now)
	plan := domain.PurgePlan{
		ID: "purge-one", ItemID: item.ID, State: domain.PurgeDraft,
		ExpectedPath: item.QuarantinePath, ExpectedSHA256: item.ContentSHA256,
		ExpectedSize: item.FileSize, RetainUntil: item.RetainUntil,
		ApprovalDigest: "correct", CreatedAt: now,
	}
	if err := st.SavePurgePlans(ctx, []domain.PurgePlan{plan}); err != nil {
		t.Fatal(err)
	}
	if err := st.ApprovePurgePlan(ctx, plan.ID, "wrong", now); err == nil {
		t.Fatal("wrong digest must not approve purge")
	}
	if err := st.ApprovePurgePlan(ctx, plan.ID, "correct", now); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetPurgePlan(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.PurgeApproved {
		t.Fatalf("expected APPROVED, got %s", got.State)
	}
}
