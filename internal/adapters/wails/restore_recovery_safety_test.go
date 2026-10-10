package wails

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func TestRestoreRecoveryDTOAndDurableLock(t *testing.T) {
	ctx := context.Background()
	source, q := t.TempDir(), t.TempDir()
	db := filepath.Join(t.TempDir(), "fresh.db")
	st, err := store.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	qp := filepath.Join(q, "PRIVATE_MARKER.dat")
	sp := filepath.Join(source, "PRIVATE_MARKER.dat")
	if err = os.WriteFile(qp, []byte("fresh disposable baseline"), 0600); err != nil {
		t.Fatal(err)
	}
	snap, err := executor.Snapshot(qp, true)
	if err != nil {
		t.Fatal(err)
	}
	task := domain.OperationTask{ID: "task", RootPath: source, State: "completed", CreatedAt: time.Now()}
	if err = st.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	p := domain.OperationPlan{ID: "source-plan", TaskID: task.ID, State: domain.PlanExecuting, Actions: []domain.PlannedAction{{Path: sp, Action: domain.OperationQuarantine, File: domain.FileInstance{Path: sp, Size: snap.Size, ContentSHA256: snap.Hash}}}}
	if err = st.SavePlans(ctx, task.ID, []domain.OperationPlan{p}); err != nil {
		t.Fatal(err)
	}
	if err = st.BeginJournal(ctx, task.ID, p.ID, p.Actions); err != nil {
		t.Fatal(err)
	}
	if err = st.MarkJournalDone(ctx, p.ID, 0, qp); err != nil {
		t.Fatal(err)
	}
	if err = st.CompletePlanExecution(ctx, p, domain.PlanVerified, nil, time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	items, err := st.ListQuarantineItems(ctx, "")
	if err != nil || len(items) != 1 {
		t.Fatal("managed fixture missing")
	}
	svc := app.NewQuarantineService(st)
	rp, err := svc.CreateRestorePlan(ctx, items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.ApproveRestorePlan(ctx, rp.ID, rp.ApprovalDigest); err != nil {
		t.Fatal(err)
	}
	approved, err := st.GetRestorePlan(ctx, rp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.BeginRestore(ctx, approved, items[0], time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(sp, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	a := NewAPI()
	a.mu.Lock()
	a.initReadWriteServicesLocked(st)
	a.mu.Unlock()
	req := RecoverRestoresRequest{QuarantineRoot: q, SourceRoots: []string{source}}
	for i := 0; i < 2; i++ {
		out, err := a.RecoverRestores(req)
		if err != nil || len(out) != 1 || out[0].Status != "failed" || out[0].FinalState != "APPROVED" || out[0].ErrorType != "manual_reconciliation_required" || out[0].Error == "" {
			t.Fatal("unsafe or missing structured refusal")
		}
		if strings.Contains(out[0].Error, "PRIVATE_MARKER") || strings.Contains(out[0].Error, source) {
			t.Fatal("DTO leaked private path")
		}
		lock, err := a.CheckRecoveryLock()
		if err != nil || !lock.LockActive || lock.RestorePendingCount != 1 {
			t.Fatal("Wails released unresolved lock")
		}
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.initReadWriteServicesLocked(st)
	a.mu.Unlock()
	lock, err := a.CheckRecoveryLock()
	if err != nil || !lock.LockActive {
		t.Fatal("reopen lost lock")
	}
	// Manual preservation is separately authorized; no forced DB unlock.
	if err = os.Rename(sp, filepath.Join(t.TempDir(), "retained")); err != nil {
		t.Fatal(err)
	}
	out, err := a.RecoverRestores(req)
	if err != nil || len(out) != 1 || out[0].Status != "ok" || out[0].FinalState != "ROLLED_BACK" {
		t.Fatal("safe reconciliation not propagated")
	}
	lock, err = a.CheckRecoveryLock()
	if err != nil || lock.LockActive || lock.RestorePendingCount != 0 {
		t.Fatal("safe terminal lock not cleared")
	}
	logs, err := st.ListLogs(ctx, p.ID)
	if err != nil || len(logs) != 3 {
		t.Fatal("blocked and successful recovery audit missing")
	}
}
