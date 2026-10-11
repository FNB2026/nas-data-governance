package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// Test-only interruption at real executor boundaries. All SQLite databases and
// files come from t.TempDir; this does not inject into a signed application.
type bcCrashJournal struct {
	*store.SQLiteStore
	mode string
}

type bcCrash struct{}

func (j bcCrashJournal) BeginJournal(ctx context.Context, task, plan string, actions []domain.PlannedAction) error {
	if j.mode == "B" {
		// The real state CAS has committed STALE_CHECKED, but no journal or
		// filesystem operation has begun. Panic prevents failure unwinding.
		panic(bcCrash{})
	}
	return j.SQLiteStore.BeginJournal(ctx, task, plan, actions)
}

func (j bcCrashJournal) MarkJournalDone(ctx context.Context, plan string, index int, target string) error {
	if err := j.SQLiteStore.MarkJournalDone(ctx, plan, index, target); err != nil {
		return err
	}
	// Real MoveFile and real durable done journal precede interruption;
	// executor/app terminal finalization has not run.
	panic(bcCrash{})
}

func TestBeta10CrashBCCombinationContract(t *testing.T) {
	for _, tc := range []struct{ mode, fault string }{
		{"B", "none"}, {"B", "audit"}, {"B", "state"},
		{"C", "none"}, {"C", "audit"}, {"C", "journal"}, {"C", "changed"},
	} {
		t.Run(tc.mode+"_"+tc.fault, func(t *testing.T) {
			ctx := context.Background()
			st, dbPath, in := consistencyFixture(t)
			p := in.Plans[0]
			var source string
			for _, a := range p.Actions {
				if a.Action == domain.OperationQuarantine {
					source = a.Path
				}
			}
			original, err := executor.Snapshot(source, true)
			if err != nil {
				t.Fatal("fixture snapshot unavailable")
			}
			backup := filepath.Join(t.TempDir(), "independent-backup")
			bytes, err := os.ReadFile(source)
			if err != nil || os.WriteFile(backup, bytes, 0600) != nil {
				t.Fatal("backup unavailable")
			}
			outside := filepath.Join(t.TempDir(), "outside-control")
			if os.WriteFile(outside, []byte("outside control"), 0600) != nil {
				t.Fatal("outside fixture unavailable")
			}
			controls := map[string]string{}
			for _, a := range p.Actions {
				if a.Path != source {
					s, err := executor.Snapshot(a.Path, true)
					if err != nil {
						t.Fatal("control unavailable")
					}
					controls[a.Path] = s.Hash
				}
			}
			controls[backup] = original.Hash
			outsideSnap, _ := executor.Snapshot(outside, true)
			controls[outside] = outsideSnap.Hash
			defer func() {
				for path, hash := range controls {
					s, err := executor.Snapshot(path, true)
					if err != nil || s.Hash != hash {
						t.Error("non-target, backup or outside control changed")
					}
				}
			}()
			ex, err := executor.NewWithJournalAndState(executor.QuarantineConfig{
				Root: in.QuarantineRoot, SourceRoots: in.SourceRoots, Structure: executor.QuarantineFlat,
			}, bcCrashJournal{st, tc.mode}, st.CompareAndSwapPlanState)
			if err != nil {
				t.Fatal("executor creation failed")
			}
			interrupted := false
			func() {
				defer func() {
					if v := recover(); v != nil {
						if _, ok := v.(bcCrash); !ok {
							panic(v)
						}
						interrupted = true
					}
				}()
				ex.Execute(ctx, &p)
			}()
			if !interrupted {
				t.Fatal("real executor interruption boundary not reached")
			}
			// Reopen before inspecting/recovering: no in-memory state proves durability.
			if err := st.Close(); err != nil {
				t.Fatal("close failed")
			}
			st, err = store.Open(ctx, dbPath)
			if err != nil {
				t.Fatal("reopen failed")
			}
			defer func() { _ = st.Close() }()
			before, err := st.GetPlan(ctx, p.ID)
			if err != nil {
				t.Fatal("durable plan missing")
			}
			journal, err := st.ListJournalAll(ctx, p.ID)
			if err != nil || requireNoRecovery(ctx, st) == nil {
				t.Fatal("interruption did not retain lock")
			}
			var target string
			if tc.mode == "B" {
				if before.State != domain.PlanStaleChecked || len(journal) != 0 {
					t.Fatal("wrong B injection boundary")
				}
				entries, err := os.ReadDir(in.QuarantineRoot)
				if err != nil || len(entries) != 0 {
					t.Fatal("B wrote quarantine output")
				}
			} else {
				if before.State != domain.PlanExecuting || len(journal) != 1 || journal[0].Status != "done" {
					t.Fatal("wrong C injection boundary")
				}
				target = journal[0].TargetPath
				q, err := executor.Snapshot(target, true)
				if err != nil || q.Hash != original.Hash || q.Size != original.Size {
					t.Fatal("C write not complete")
				}
				if _, err := os.Stat(source); !os.IsNotExist(err) {
					t.Fatal("C source unexpectedly exists")
				}
			}
			fault, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Fatal("fault connection failed")
			}
			defer fault.Close()
			trigger := ""
			switch tc.fault {
			case "audit":
				trigger = `BEFORE INSERT ON operation_logs`
			case "state":
				trigger = `BEFORE UPDATE OF state ON operation_plans WHEN NEW.state='DRAFT'`
			case "journal":
				trigger = `BEFORE UPDATE OF rollback_status ON execution_journal`
			case "changed":
				if err := os.WriteFile(target, []byte("external replacement after crash"), 0600); err != nil {
					t.Fatal("change injection failed")
				}
			}
			if trigger != "" {
				if _, err := fault.Exec(`CREATE TRIGGER reject_bc ` + trigger + ` BEGIN SELECT RAISE(FAIL,'PRIVATE_BC_CANARY'); END`); err != nil {
					t.Fatal("fault trigger failed")
				}
			}
			results, err := NewRecoveryService(st).Recover(ctx)
			if err != nil || len(results) != 1 {
				t.Fatal("recovery result unavailable")
			}
			if strings.Contains(fmt.Sprint(results), "PRIVATE_BC_CANARY") || strings.Contains(fmt.Sprint(results), source) || strings.Contains(fmt.Sprint(results), target) && target != "" {
				t.Fatal("recovery response leaked private data")
			}
			durable, err := st.GetPlan(ctx, p.ID)
			if err != nil {
				t.Fatal("final plan missing")
			}
			logs, err := st.ListLogs(ctx, p.ID)
			if err != nil {
				t.Fatal("audit unavailable")
			}
			// Capture the authoritative first-recovery outcome, then require every
			// restart/retry/old request below to preserve bytes and durable records.
			jAfter, err := st.ListJournalAll(ctx, p.ID)
			if err != nil {
				t.Fatal("post-recovery journal missing")
			}
			type bytesState struct {
				Present bool
				Hash    string
				Size    int64
			}
			readBytes := func(path string) bytesState {
				t.Helper()
				if _, e := os.Lstat(path); os.IsNotExist(e) {
					return bytesState{}
				} else if e != nil {
					t.Fatal("byte identity unavailable")
				}
				s, e := executor.Snapshot(path, true)
				if os.IsNotExist(e) {
					return bytesState{}
				}
				if e != nil {
					t.Fatal("post-recovery bytes unavailable")
				}
				return bytesState{true, s.Hash, s.Size}
			}
			sourceAfter := readBytes(source)
			targetAfter := bytesState{}
			if target != "" {
				targetAfter = readBytes(target)
			}
			assertStable := func() {
				t.Helper()
				pp, e := st.GetPlan(ctx, p.ID)
				jj, je := st.ListJournalAll(ctx, p.ID)
				ll, le := st.ListLogs(ctx, p.ID)
				if e != nil || je != nil || le != nil || !reflect.DeepEqual(pp, durable) || !reflect.DeepEqual(jj, jAfter) || !reflect.DeepEqual(ll, logs) || readBytes(source) != sourceAfter || target != "" && readBytes(target) != targetAfter {
					t.Fatal("repeat/restart/old request changed authoritative outcome")
				}
			}
			if tc.fault != "none" {
				if results[0].Action != executor.RecoverySkipped || len(results[0].Errors) == 0 || durable.State != before.State || requireNoRecovery(ctx, st) == nil || len(logs) != 0 {
					t.Fatal("unconfirmed recovery released lock or committed audit/state")
				}
				if tc.fault == "changed" {
					got, err := os.ReadFile(target)
					if err != nil || string(got) != "external replacement after crash" {
						t.Fatal("changed output modified")
					}
				} else {
					got, err := executor.Snapshot(source, true)
					if err != nil || got.Hash != original.Hash {
						t.Fatal("failure lost original content")
					}
				}
				// Repeat after another restart must retain the lock, not report success.
				_ = st.Close()
				st, err = store.Open(ctx, dbPath)
				if err != nil {
					t.Fatal("second reopen failed")
				}
				again, err := NewRecoveryService(st).Recover(ctx)
				if err != nil || len(again) != 1 || again[0].Action != executor.RecoverySkipped || requireNoRecovery(ctx, st) == nil {
					t.Fatal("restart bypassed fault lock")
				}
				blocked, err := NewExecutionService(st).Execute(ctx, in)
				if err == nil || blocked != nil && blocked.Executed != 0 {
					t.Fatal("uncertain state allowed file execution")
				}
				assertStable()
				t.Logf("fault=%s classification=skipped lock=retained audit=uncommitted restart=verified", tc.fault)
				return
			}
			want, action, event := domain.PlanDraft, executor.RecoveryResetToDraft, "approval_invalidated"
			if tc.mode == "C" {
				want, action, event = domain.PlanRolledBack, executor.RecoveryRolledBack, "recovery"
			}
			if durable.State != want || results[0].Action != action || len(results[0].Errors) != 0 || requireNoRecovery(ctx, st) != nil {
				t.Fatal("success state/result/lock mismatch")
			}
			if len(logs) != 1 || logs[0].EventType != event || logs[0].Detail["final_state"] != string(want) {
				t.Fatal("terminal audit inconsistent")
			}
			wantError, wantStatus := "approval_invalidated", "failed"
			if tc.mode == "C" {
				wantError, wantStatus = "recovery_rolled_back", "ok"
			}
			if logs[0].Detail["error_type"] != wantError || logs[0].Detail["status"] != wantStatus {
				t.Fatal("audit classification inconsistent")
			}
			got, err := executor.Snapshot(source, true)
			if err != nil || got.Hash != original.Hash || got.Size != original.Size {
				t.Fatal("recovered source mismatch")
			}
			if tc.mode == "C" {
				journal, err = st.ListJournalAll(ctx, p.ID)
				if err != nil || len(journal) != 1 || journal[0].RollbackStatus != "done" || results[0].RolledBack != 1 {
					t.Fatal("rollback journal inconsistent")
				}
				if _, err = os.Stat(target); !os.IsNotExist(err) {
					t.Fatal("rollback duplicate remains")
				}
			}
			_ = st.Close()
			st, err = store.Open(ctx, dbPath)
			if err != nil {
				t.Fatal("terminal reopen failed")
			}
			again, err := NewRecoveryService(st).Recover(ctx)
			if err != nil || len(again) != 0 || requireNoRecovery(ctx, st) != nil {
				t.Fatal("repeat recovery not idempotent")
			}
			old, err := NewExecutionService(st).Execute(ctx, in)
			if err != nil || old.Executed != 0 || old.Skipped != 1 {
				t.Fatal("old approval reused")
			}
			assertStable()
			logs, err = st.ListLogs(ctx, p.ID)
			if err != nil || len(logs) != 1 {
				t.Fatal("repeat duplicated audit")
			}
			t.Logf("boundary=%s state=%s audit=%s lock=clear SHA256=matched old_approval=rejected restart=verified", tc.mode, want, event)
		})
	}
}
