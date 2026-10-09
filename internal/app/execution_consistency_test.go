package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func consistencyFixture(t *testing.T) (*store.SQLiteStore, string, ExecutionInput) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "input")
	q := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "temp"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(root, "temp", name), []byte("duplicate fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	db := filepath.Join(t.TempDir(), "governance.db")
	st, err := store.Open(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	plans := scanForPlans(t, root, "fixture")
	if len(plans) != 1 {
		t.Fatalf("expected one plan: %d", len(plans))
	}
	if err := st.CreateTask(context.Background(), domain.OperationTask{ID: "fixture-task", RootPath: root, State: "planned", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	plans[0].TaskID = "fixture-task"
	if err := st.SavePlans(context.Background(), "fixture-task", plans); err != nil {
		t.Fatal(err)
	}
	return st, db, ExecutionInput{Plans: plans, SourceRoots: []string{root}, QuarantineRoot: q, Retention: 720 * time.Hour}
}

func TestExecutionConsistencyVerifiedReloadAndRepeat(t *testing.T) {
	st, db, in := consistencyFixture(t)
	ctx := context.Background()
	result, err := NewExecutionService(st).Execute(ctx, in)
	if err != nil || result.Executed != 1 || result.Failed != 0 {
		t.Fatalf("first execution: %+v %v", result, err)
	}
	p, err := st.GetPlan(ctx, in.Plans[0].ID)
	if err != nil || p.State != domain.PlanVerified {
		t.Fatalf("durable final state=%s err=%v", p.State, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	// Reuse the original APPROVED request, as a stale CLI/UI request would.
	again, err := NewExecutionService(reopened).Execute(ctx, in)
	if err != nil || again.Executed != 0 {
		t.Fatalf("repeat must not execute: %+v %v", again, err)
	}
	items, err := reopened.ListQuarantineItems(ctx, "")
	if err != nil || len(items) != 1 {
		t.Fatalf("duplicate quarantine items: %d %v", len(items), err)
	}
	journal, err := reopened.ListJournalAll(ctx, p.ID)
	if err != nil || len(journal) != 1 || journal[0].Status != "done" {
		t.Fatalf("journal mismatch: %+v %v", journal, err)
	}
}

func TestExecutionConsistencyStaleInvalidatesApproval(t *testing.T) {
	st, _, in := consistencyFixture(t)
	ctx := context.Background()
	dry := in
	dry.DryRun = true
	if r, e := NewExecutionService(st).Execute(ctx, dry); e != nil || r.Failed != 0 {
		t.Fatalf("dry run: %+v %v", r, e)
	}
	var target string
	for _, a := range in.Plans[0].Actions {
		if a.Action == domain.OperationQuarantine {
			target = a.Path
		}
	}
	if err := os.WriteFile(target, []byte("changed after approved dry run"), 0600); err != nil {
		t.Fatal(err)
	}
	r, e := NewExecutionService(st).Execute(ctx, in)
	if e != nil || r.Executed != 0 || r.Failed != 1 || r.Results[0].ErrorType != "stale_detected" {
		t.Fatalf("stale counted as success: %+v %v", r, e)
	}
	p, e := st.GetPlan(ctx, in.Plans[0].ID)
	if e != nil || p.State != domain.PlanDraft {
		t.Fatalf("stale approval remains %s %v", p.State, e)
	}
	again, e := NewExecutionService(st).Execute(ctx, in)
	if e != nil || again.Executed != 0 {
		t.Fatalf("old approval reused: %+v %v", again, e)
	}
	entries, e := st.ListJournalAll(ctx, p.ID)
	if e != nil || len(entries) != 0 {
		t.Fatalf("stale created journal %+v %v", entries, e)
	}
	if _, e := os.Stat(target); e != nil {
		t.Fatal("stale source lost")
	}
}

func TestExecutionConsistencyTerminalPersistenceFailure(t *testing.T) {
	st, db, in := consistencyFixture(t)
	ctx := context.Background()
	fault, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	defer fault.Close()
	if _, err := fault.Exec(`CREATE TRIGGER reject_verified BEFORE UPDATE OF state ON operation_plans WHEN NEW.state='VERIFIED' BEGIN SELECT RAISE(FAIL,'injected state failure'); END`); err != nil {
		t.Fatal(err)
	}
	r, e := NewExecutionService(st).Execute(ctx, in)
	if e == nil || r == nil || r.Executed != 0 || r.Failed != 1 {
		t.Fatalf("persistence failure declared success: %+v %v", r, e)
	}
	p, err := st.GetPlan(ctx, in.Plans[0].ID)
	if err != nil || p.State != domain.PlanExecuting {
		t.Fatalf("must retain recovery state: %s %v", p.State, err)
	}
	items, err := st.ListQuarantineItems(ctx, "")
	if err != nil || len(items) != 0 {
		t.Fatalf("partial finalization: %d %v", len(items), err)
	}
	entries, err := st.ListJournalDone(ctx, p.ID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("confirmed write evidence missing: %+v %v", entries, err)
	}
	again, e := NewExecutionService(st).Execute(ctx, in)
	if again != nil && again.Executed != 0 {
		t.Fatalf("uncertain plan repeated: %+v %v", again, e)
	}
}

func TestExecutionConsistencyFaultBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, trigger string
		state         domain.PlanState
		sourcePresent bool
	}{
		{"reservation", `BEFORE UPDATE OF state ON operation_plans WHEN NEW.state='STALE_CHECKED'`, domain.PlanApproved, true},
		{"journal_begin", `BEFORE INSERT ON execution_journal`, domain.PlanApproved, true},
		{"execution_claim", `BEFORE UPDATE OF state ON operation_plans WHEN NEW.state='EXECUTING'`, domain.PlanStaleChecked, true},
		{"quarantine_registration", `BEFORE INSERT ON quarantine_items`, domain.PlanExecuting, false},
		{"audit_commit", `BEFORE INSERT ON operation_logs`, domain.PlanExecuting, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, db, in := consistencyFixture(t)
			fault, e := sql.Open("sqlite", db)
			if e != nil {
				t.Fatal(e)
			}
			defer fault.Close()
			if _, e = fault.Exec(`CREATE TRIGGER injected_fault ` + tc.trigger + ` BEGIN SELECT RAISE(FAIL,'PRIVATE_PATH_CANARY'); END`); e != nil {
				t.Fatal(e)
			}
			r, e := NewExecutionService(st).Execute(context.Background(), in)
			if r == nil || r.Executed != 0 || r.Failed != 1 {
				t.Fatalf("fault was reported successful: %+v %v", r, e)
			}
			if strings.Contains(fmt.Sprint(e, r.Results[0].Err), "PRIVATE_PATH_CANARY") {
				t.Fatal("database fault leaked private marker")
			}
			p, err := st.GetPlan(context.Background(), in.Plans[0].ID)
			if err != nil || p.State != tc.state {
				t.Fatalf("durable state=%s %v", p.State, err)
			}
			for _, a := range in.Plans[0].Actions {
				if a.Action == domain.OperationQuarantine {
					_, err := os.Stat(a.Path)
					if (err == nil) != tc.sourcePresent {
						t.Fatalf("unexpected filesystem outcome: %v", err)
					}
				}
			}
			items, err := st.ListQuarantineItems(context.Background(), "")
			if err != nil || len(items) != 0 {
				t.Fatalf("partial finalization: %+v %v", items, err)
			}
		})
	}
}

func TestExecutionConsistencyOwnerExcludesRecoveryAndOtherHandles(t *testing.T) {
	st, db, in := consistencyFixture(t)
	other, e := store.Open(context.Background(), db)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	unlock, e := st.AcquireExecutionLock()
	if e != nil {
		t.Fatal(e)
	}
	r, e := NewExecutionService(other).Execute(context.Background(), in)
	if e == nil || r != nil {
		t.Fatalf("active execution owner not excluded: %+v %v", r, e)
	}
	recovery, e := NewRecoveryService(other).Recover(context.Background())
	if e != nil || len(recovery) != 1 || len(recovery[0].Errors) == 0 {
		t.Fatalf("recovery not excluded: %+v %v", recovery, e)
	}
	unlock()
	r, e = NewExecutionService(other).Execute(context.Background(), in)
	if e != nil || r.Executed != 1 {
		t.Fatalf("owner released but execution failed: %+v %v", r, e)
	}
}

func TestExecutionConsistencyConcurrentRequests(t *testing.T) {
	st, db, in := consistencyFixture(t)
	other, e := store.Open(context.Background(), db)
	if e != nil {
		t.Fatal(e)
	}
	defer other.Close()
	start := make(chan struct{})
	counts := make(chan int, 2)
	for _, handle := range []*store.SQLiteStore{st, other} {
		go func(handle *store.SQLiteStore) {
			<-start
			r, _ := NewExecutionService(handle).Execute(context.Background(), in)
			n := 0
			if r != nil {
				n = r.Executed
			}
			counts <- n
		}(handle)
	}
	close(start)
	if n := <-counts + <-counts; n != 1 {
		t.Fatalf("concurrent winners=%d", n)
	}
	items, e := st.ListQuarantineItems(context.Background(), "")
	if e != nil || len(items) != 1 {
		t.Fatalf("concurrent quarantine registration: %d %v", len(items), e)
	}
}

func TestExecutionConsistencyCanonicalActions(t *testing.T) {
	st, _, in := consistencyFixture(t)
	external := filepath.Join(t.TempDir(), "non-target.txt")
	if e := os.WriteFile(external, []byte("protected outside"), 0600); e != nil {
		t.Fatal(e)
	}
	in.Plans[0].Actions = append([]domain.PlannedAction(nil), in.Plans[0].Actions...)
	for i := range in.Plans[0].Actions {
		if in.Plans[0].Actions[i].Action == domain.OperationQuarantine {
			in.Plans[0].Actions[i].Path = external
			in.Plans[0].Actions[i].File.Path = external
		}
	}
	r, e := NewExecutionService(st).Execute(context.Background(), in)
	if e != nil || r.Executed != 1 {
		t.Fatalf("canonical execution: %+v %v", r, e)
	}
	got, e := os.ReadFile(external)
	if e != nil || string(got) != "protected outside" {
		t.Fatal("request overrode approved actions")
	}
}

func TestExecutionConsistencyFinalizationStopsBatch(t *testing.T) {
	st, db, in := consistencyFixture(t)
	first := in.Plans[0]
	// A second independent plan with different bytes and its own task.
	root := filepath.Join(t.TempDir(), "input")
	if e := os.MkdirAll(filepath.Join(root, "temp"), 0700); e != nil {
		t.Fatal(e)
	}
	for _, n := range []string{"c.txt", "d.txt"} {
		if e := os.WriteFile(filepath.Join(root, "temp", n), []byte("second duplicate content"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	second := scanForPlans(t, root, "second-fixture")[0]
	second.TaskID = "second-task"
	ctx := context.Background()
	if e := st.CreateTask(ctx, domain.OperationTask{ID: second.TaskID, RootPath: root, State: "planned", CreatedAt: time.Now()}); e != nil {
		t.Fatal(e)
	}
	if e := st.SavePlans(ctx, second.TaskID, []domain.OperationPlan{second}); e != nil {
		t.Fatal(e)
	}
	in.Plans = append(in.Plans, second)
	in.SourceRoots = append(in.SourceRoots, root)
	fault, e := sql.Open("sqlite", db)
	if e != nil {
		t.Fatal(e)
	}
	defer fault.Close()
	if _, e = fault.Exec(`CREATE TRIGGER terminal_failure BEFORE UPDATE OF state ON operation_plans WHEN NEW.state='VERIFIED' BEGIN SELECT RAISE(FAIL,'injected fault'); END`); e != nil {
		t.Fatal(e)
	}
	r, e := NewExecutionService(st).Execute(ctx, in)
	if e == nil || r == nil || r.Executed != 0 || r.Failed != 1 || r.Skipped != 1 || r.Results[1].ErrorType != "recovery_required" {
		t.Fatalf("batch continued after uncertain outcome: %+v %v", r, e)
	}
	p, e := st.GetPlan(ctx, first.ID)
	if e != nil || p.State != domain.PlanExecuting {
		t.Fatal("first reservation lost")
	}
	p, e = st.GetPlan(ctx, second.ID)
	if e != nil || p.State != domain.PlanApproved {
		t.Fatal("second approval changed")
	}
	journal, e := st.ListJournalAll(ctx, second.ID)
	if e != nil || len(journal) != 0 {
		t.Fatal("second plan started filesystem work")
	}
	for _, a := range second.Actions {
		if _, e := os.Stat(a.Path); e != nil {
			t.Fatal("second source changed")
		}
	}
}
