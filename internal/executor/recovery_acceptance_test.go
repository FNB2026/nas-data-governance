package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// Fresh disposable fault fixtures against frozen beta.8 backend semantics.
// These tests are not evidence of a crash in the signed desktop artifact.
func TestRecoveryDurableAcceptance(t *testing.T) {
	for _, mode := range []string{"pending_partial", "no_journal", "stale_checked_no_journal", "done"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			dbPath := filepath.Join(root, "project.db")
			st, err := store.Open(ctx, dbPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.Close() })
			src, target := filepath.Join(root, "source"), filepath.Join(root, "isolated")
			original := []byte("disposable recovery byte evidence")
			if err := os.WriteFile(src, original, 0600); err != nil {
				t.Fatal(err)
			}
			backup := filepath.Join(t.TempDir(), "independent-backup")
			if err := os.WriteFile(backup, original, 0600); err != nil {
				t.Fatal(err)
			}
			snap, err := Snapshot(src, true)
			if err != nil {
				t.Fatal(err)
			}
			actions := []domain.PlannedAction{{Path: src, Action: domain.OperationQuarantine, File: domain.FileInstance{Path: src, Size: snap.Size, ContentSHA256: snap.Hash}}}
			plan, task := seedPlanWithTask(t, st, "disposable-recovery", actions)
			if mode != "no_journal" && mode != "stale_checked_no_journal" {
				if err := st.BeginJournal(ctx, task, plan.ID, actions); err != nil {
					t.Fatal(err)
				}
				if mode == "done" {
					if err := os.Rename(src, target); err != nil {
						t.Fatal(err)
					}
					if err := st.MarkJournalDone(ctx, plan.ID, 0, target); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(target, []byte("partial"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			initial := domain.PlanExecuting
			if mode == "stale_checked_no_journal" {
				initial = domain.PlanStaleChecked
			}
			if err := st.UpdatePlanState(ctx, plan.ID, initial); err != nil {
				t.Fatal(err)
			}
			results := NewForRecovery().Recover(ctx, st)
			if len(results) != 1 {
				t.Fatal("missing recovery classification")
			}
			if err := st.Close(); err != nil {
				t.Fatal(err)
			}
			st, err = store.Open(ctx, dbPath)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := st.GetPlan(ctx, plan.ID)
			if err != nil {
				t.Fatal(err)
			}
			locked, err := st.ListExecutingPlans(ctx)
			if err != nil {
				t.Fatal(err)
			}
			logs, err := st.ListLogs(ctx, plan.ID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "pending_partial" {
				if results[0].Action != RecoverySkipped || len(results[0].Errors) == 0 || actual.State != domain.PlanExecuting || len(locked) != 1 {
					t.Fatal("uncertain action lost durable lock")
				}
				again := NewForRecovery().Recover(ctx, st)
				if len(again) != 1 || again[0].Action != RecoverySkipped {
					t.Fatal("pending retry changed classification")
				}
				actual, err = st.GetPlan(ctx, plan.ID)
				if err != nil || actual.State != domain.PlanExecuting {
					t.Fatal("retry released plan")
				}
				locked, err = st.ListExecutingPlans(ctx)
				if err != nil || len(locked) != 1 {
					t.Fatal("retry released lock")
				}
				pending, err := st.ListJournalPending(ctx, plan.ID)
				if err != nil || len(pending) != 1 || pending[0].RollbackStatus != "" {
					t.Fatal("retry altered uncertain journal")
				}
				logs, err = st.ListLogs(ctx, plan.ID)
				if err != nil {
					t.Fatal(err)
				}
				part, err := os.ReadFile(target)
				if err != nil || string(part) != "partial" {
					t.Fatal("partial evidence changed")
				}
				if len(logs) != 0 {
					t.Fatal("uncertain action recorded completed recovery")
				}
			} else {
				wantState, wantAction, wantEvent := domain.PlanDraft, RecoveryResetToDraft, "approval_invalidated"
				if mode == "done" {
					wantState, wantAction, wantEvent = domain.PlanRolledBack, RecoveryRolledBack, "recovery"
				}
				if actual.State != wantState || results[0].Action != wantAction || len(results[0].Errors) != 0 || len(locked) != 0 {
					t.Fatal("terminal recovery not durable")
				}
				if len(logs) != 1 || logs[0].EventType != wantEvent {
					t.Fatal("terminal audit missing")
				}
				detail := logs[0].Detail
				wantStatus, wantError := "failed", "approval_invalidated"
				if mode == "done" {
					wantStatus, wantError = "ok", "recovery_rolled_back"
				}
				if detail["status"] != wantStatus || detail["error_type"] != wantError || detail["final_state"] != string(wantState) {
					t.Fatal("audit classification inconsistent")
				}
				if err := st.CompareAndSwapPlanState(ctx, plan.ID, domain.PlanApproved, domain.PlanStaleChecked); !errors.Is(err, store.ErrPlanStateConflict) {
					t.Fatal("old approval acquired execution")
				}
				if len(NewForRecovery().Recover(ctx, st)) != 0 {
					t.Fatal("terminal recovery not idempotent")
				}
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatal("unexpected isolation output")
				}
				if mode == "done" {
					entries, err := st.ListJournalDone(ctx, plan.ID)
					if err != nil || len(entries) != 1 || entries[0].RollbackStatus != "done" {
						t.Fatal("rollback journal not durable")
					}
				}
			}
			for _, path := range []string{src, backup} {
				got, err := Snapshot(path, true)
				if err != nil || got.Hash != snap.Hash || got.Size != snap.Size {
					t.Fatal("source or backup integrity lost")
				}
			}
		})
	}
}
