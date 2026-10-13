package wails

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// Adapter coverage supplements the real executor interruption carrier in app.
// Here the durable boundaries are explicitly seeded only in new test databases.
func TestBeta10CrashBCAdapterContract(t *testing.T) {
	for _, mode := range []string{"B", "C", "B_audit_failure", "C_audit_failure"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			root, q := t.TempDir(), t.TempDir()
			db := filepath.Join(t.TempDir(), "fixture.db")
			source := filepath.Join(root, "PRIVATE_BC.dat")
			if err := os.WriteFile(source, []byte("disposable source bytes"), 0600); err != nil {
				t.Fatal("fixture write failed")
			}
			snap, err := executor.Snapshot(source, true)
			if err != nil {
				t.Fatal("snapshot missing")
			}
			st, err := store.Open(ctx, db)
			if err != nil {
				t.Fatal("store open failed")
			}
			defer func() {
				if st != nil {
					_ = st.Close()
				}
			}()
			task := domain.OperationTask{ID: "task", RootPath: root, State: "executing", CreatedAt: time.Now()}
			if st.CreateTask(ctx, task) != nil {
				t.Fatal("task missing")
			}
			p := domain.OperationPlan{ID: "plan", TaskID: task.ID, State: domain.PlanApproved, Actions: []domain.PlannedAction{{Path: source, Action: domain.OperationQuarantine, File: domain.FileInstance{Path: source, Size: snap.Size, ModifiedAt: snap.ModifiedAt, Device: snap.Device, Inode: snap.Inode, ContentSHA256: snap.Hash}}}}
			if st.SavePlans(ctx, task.ID, []domain.OperationPlan{p}) != nil || st.CompareAndSwapPlanState(ctx, p.ID, domain.PlanApproved, domain.PlanStaleChecked) != nil {
				t.Fatal("reservation failed")
			}
			if strings.HasPrefix(mode, "C") {
				target := filepath.Join(q, "PRIVATE_BC.dat")
				if st.BeginJournal(ctx, task.ID, p.ID, p.Actions) != nil || st.CompareAndSwapPlanState(ctx, p.ID, domain.PlanStaleChecked, domain.PlanExecuting) != nil || executor.MoveFile(source, target, snap.Hash) != nil || st.MarkJournalDone(ctx, p.ID, 0, target) != nil {
					t.Fatal("done boundary missing")
				}
			}
			if err := st.Close(); err != nil {
				t.Fatal("close failed")
			}
			st, err = store.Open(ctx, db)
			if err != nil {
				t.Fatal("reopen failed")
			}
			a := NewAPI()
			a.initReadWriteServicesLocked(st)
			assertLock := func(want bool) {
				t.Helper()
				lock, e := a.CheckRecoveryLock()
				caps, ce := a.GetAppCapabilities()
				if e != nil || ce != nil || lock.LockActive != want || caps.RecoveryLockActive != want || caps.CanExecuteQuarantine == want || caps.CanScan == want || caps.CanEditReviews == want || caps.CanApprovePlans == want || caps.CanExecutePurge {
					t.Fatal("adapter lock/capability mismatch")
				}
			}
			assertLock(true)
			var fault *sql.DB
			const auditFault = `CREATE TRIGGER reject_audit BEFORE INSERT ON operation_logs BEGIN SELECT RAISE(FAIL,'PRIVATE_BC_CANARY'); END`
			if strings.HasSuffix(mode, "failure") {
				other := p
				other.ID = "approved-control"
				other.TaskID = "control-task"
				controlTask := task
				controlTask.ID = other.TaskID
				if st.CreateTask(ctx, controlTask) != nil || st.SavePlans(ctx, other.TaskID, []domain.OperationPlan{other}) != nil {
					t.Fatal("approved control unavailable")
				}
				var e error
				fault, e = sql.Open("sqlite", db)
				if e != nil {
					t.Fatal("fault connection failed")
				}
				defer fault.Close()
				if _, e = fault.Exec(auditFault); e != nil {
					t.Fatal("fault creation failed")
				}
			}
			out, err := a.RecoverSourcePlans()
			if err != nil || len(out) != 1 {
				t.Fatal("adapter result missing")
			}
			want := "reset_to_draft"
			if strings.HasPrefix(mode, "C") {
				want = "rolled_back"
			}
			failed := strings.HasSuffix(mode, "failure")
			if failed {
				want = "skipped"
			}
			if out[0].Action != want || failed != (len(out[0].Errors) > 0) || strings.Contains(strings.Join(out[0].Errors, " "), "PRIVATE_BC") || strings.Contains(strings.Join(out[0].Errors, " "), root) {
				t.Fatal("adapter classification/private response mismatch")
			}
			assertLock(failed)
			logs, err := st.ListLogs(ctx, p.ID)
			if err != nil || len(logs) != map[bool]int{false: 1, true: 0}[failed] {
				t.Fatal("adapter audit inconsistent")
			}
			// Extra test triggers are intentionally not supported project layouts.
			// Suspend only the synthetic injector across the real store preflight;
			// durable failed recovery state/audit/journal remain untouched.
			if failed {
				if _, e := fault.Exec(`DROP TRIGGER reject_audit`); e != nil {
					t.Fatal("suspend fault")
				}
			}
			if err = st.Close(); err != nil {
				t.Fatal("second close failed")
			}
			st, err = store.Open(ctx, db)
			if err != nil {
				t.Fatal("second reopen failed")
			}
			if failed {
				if _, e := fault.Exec(auditFault); e != nil {
					t.Fatal("restore synthetic injector")
				}
			}
			a = NewAPI()
			a.initReadWriteServicesLocked(st)
			assertLock(failed)
			again, e := a.RecoverSourcePlans()
			if e != nil || !failed && len(again) != 0 || failed && (len(again) != 1 || again[0].Action != "skipped") {
				t.Fatal("adapter repeat inconsistent")
			}
			if failed {
				if _, e := a.ApprovePlans(ApprovePlansRequest{PlanIDs: []string{p.ID}}); e == nil {
					t.Fatal("locked adapter allowed approval")
				}
				out, e := a.ExecutePlans(ExecutePlansRequest{PlanIDs: []string{p.ID}, QuarantineRoot: q, SourceRoots: []string{root}, RetentionHours: 24})
				// The adapter may reject an ineligible reserved plan by returning
				// an empty zero-execution response instead of an error.
				if out.Executed != 0 || e == nil && len(out.Results) != 0 {
					t.Fatal("locked adapter allowed execution")
				}
				out, e = a.ExecutePlans(ExecutePlansRequest{PlanIDs: []string{"approved-control"}, QuarantineRoot: q, SourceRoots: []string{root}, RetentionHours: 24})
				if e == nil || out.Executed != 0 {
					t.Fatal("recovery lock allowed eligible plan to write")
				}
				jj, e := st.ListJournalAll(ctx, "approved-control")
				if e != nil || len(jj) != 0 {
					t.Fatal("locked request wrote control journal")
				}
				assertLock(true)
			}
			if failed || strings.HasPrefix(mode, "C") {
				got, e := executor.Snapshot(source, true)
				if e != nil || got.Hash != snap.Hash || got.Size != snap.Size {
					t.Fatal("adapter recovery lost bytes")
				}
			}
			t.Logf("mode=%s DTO=%s restarted_lock=%t audit_rows=%d", mode, want, failed, len(logs))
		})
	}
}
