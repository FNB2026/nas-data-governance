package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// Issue #72 regression: fresh synthetic data only, never historical failure DBs.
func TestRestorePartialDestinationRetainsRecoveryLock(t *testing.T) {
	ctx := context.Background()
	source, quarantine := t.TempDir(), t.TempDir()
	st := openTestStore(t)
	item := seedQuarantineItem(t, st, source, quarantine, "disposable.dat", time.Now(), time.Now().Add(time.Hour))
	svc := NewQuarantineService(st)
	plan, err := svc.CreateRestorePlan(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ApproveRestorePlan(ctx, plan.ID, plan.ApprovalDigest); err != nil {
		t.Fatal(err)
	}
	approved, err := st.GetRestorePlan(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.BeginRestore(ctx, approved, item, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(item.SourcePath, []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	results, err := svc.RecoverRestores(ctx, RecoverRestoresInput{QuarantineRoot: quarantine, SourceRoots: []string{source}})
	if err != nil || len(results) != 1 {
		t.Fatalf("recovery results: %v %v", results, err)
	}
	actual, err := st.GetRestorePlan(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := st.ListPendingRestores(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Status == executor.StepOK || results[0].Err == nil || actual.State == domain.RestoreRolledBack || len(pending) != 1 {
		t.Fatalf("partial destination incorrectly terminalized: status=%s final=%s durable=%s pending=%d", results[0].Status, results[0].FinalState, actual.State, len(pending))
	}
	if err := requireNoRecovery(ctx, st); err == nil {
		t.Fatal("partial destination released recovery lock")
	}
	b, err := os.ReadFile(item.SourcePath)
	if err != nil || string(b) != "partial" {
		t.Fatal("partial evidence modified")
	}
	q, err := executor.Snapshot(item.QuarantinePath, true)
	if err != nil || q.Size != item.FileSize || q.Hash != item.ContentSHA256 {
		t.Fatal("quarantine copy modified")
	}
}

type restoreSafetyFixture struct {
	st                     *store.SQLiteStore
	svc                    *QuarantineService
	db, source, quarantine string
	item                   domain.QuarantineItem
	plan                   domain.RestorePlan
	original               []byte
}

func newRestoreSafetyFixture(t *testing.T) *restoreSafetyFixture {
	t.Helper()
	f := &restoreSafetyFixture{db: filepath.Join(t.TempDir(), "fresh.db"), source: t.TempDir(), quarantine: t.TempDir()}
	var err error
	f.st, err = store.Open(context.Background(), f.db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.st.Close() })
	f.svc = NewQuarantineService(f.st)
	f.item = seedQuarantineItem(t, f.st, f.source, f.quarantine, "PRIVATE_MARKER.dat", time.Now(), time.Now().Add(time.Hour))
	f.original, err = os.ReadFile(f.item.QuarantinePath)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.svc.CreateRestorePlan(context.Background(), f.item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.ApproveRestorePlan(context.Background(), p.ID, p.ApprovalDigest); err != nil {
		t.Fatal(err)
	}
	f.plan, err = f.st.GetRestorePlan(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *restoreSafetyFixture) begin(t *testing.T) {
	t.Helper()
	if err := f.st.BeginRestore(context.Background(), f.plan, f.item, time.Now()); err != nil {
		t.Fatal(err)
	}
}
func (f *restoreSafetyFixture) reopen(t *testing.T) {
	t.Helper()
	if err := f.st.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.st, err = store.Open(context.Background(), f.db)
	if err != nil {
		t.Fatal(err)
	}
	f.svc = NewQuarantineService(f.st)
}
func (f *restoreSafetyFixture) recover(t *testing.T, blocked bool) {
	t.Helper()
	ctx := context.Background()
	rs, err := f.svc.RecoverRestores(ctx, RecoverRestoresInput{QuarantineRoot: f.quarantine, SourceRoots: []string{f.source}})
	if err != nil {
		pending, pErr := f.st.ListPendingRestores(ctx)
		if !blocked || pErr != nil || len(pending) != 1 || requireNoRecovery(ctx, f.st) == nil {
			t.Fatal("configuration refusal released lock")
		}
		if strings.Contains(err.Error(), "PRIVATE_MARKER") || strings.Contains(err.Error(), f.source) || strings.Contains(err.Error(), f.quarantine) {
			t.Fatal("configuration error leaked")
		}
		return
	}
	if len(rs) != 1 {
		t.Fatalf("unexpected recovery count: %d", len(rs))
	}
	pending, err := f.st.ListPendingRestores(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.st.GetRestorePlan(ctx, f.plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if blocked {
		if rs[0].Status != executor.StepFailed || rs[0].Err == nil || p.State != domain.RestoreApproved || len(pending) != 1 || requireNoRecovery(ctx, f.st) == nil {
			t.Fatal("unsafe terminal or released lock")
		}
		if strings.Contains(rs[0].Err.Error(), "PRIVATE_MARKER") || strings.Contains(rs[0].Err.Error(), f.source) || strings.Contains(rs[0].Err.Error(), f.quarantine) {
			t.Fatal("private error leaked")
		}
	} else {
		if rs[0].Status != executor.StepOK || p.State != domain.RestoreRolledBack || len(pending) != 0 || requireNoRecovery(ctx, f.st) != nil || p.ApprovalDigest != "" {
			t.Fatal("safe closure state/approval/lock inconsistent")
		}
	}
}

func TestRestoreRecoveryEvidenceMatrix(t *testing.T) {
	for _, mode := range []string{"absent_target", "partial", "changed_same_size", "both_full", "only_target", "q_changed", "both_absent", "target_directory", "target_unreadable", "q_unreadable", "target_symlink", "q_symlink", "source_root_symlink", "q_root_symlink", "source_root_missing", "q_root_missing", "source_root_unreadable", "q_root_unreadable", "out_of_scope"} {
		t.Run(mode, func(t *testing.T) {
			f := newRestoreSafetyFixture(t)
			f.begin(t)
			write := func(p string, b []byte) {
				t.Helper()
				if err := os.WriteFile(p, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			remove := func(p string) {
				t.Helper()
				if err := os.Remove(p); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "partial":
				write(f.item.SourcePath, []byte("part"))
			case "changed_same_size":
				write(f.item.SourcePath, bytes.Repeat([]byte{'x'}, len(f.original)))
			case "both_full":
				write(f.item.SourcePath, f.original)
			case "only_target":
				write(f.item.SourcePath, f.original)
				remove(f.item.QuarantinePath)
			case "q_changed":
				write(f.item.QuarantinePath, []byte("changed"))
			case "both_absent":
				remove(f.item.QuarantinePath)
			case "target_directory":
				if err := os.Mkdir(f.item.SourcePath, 0700); err != nil {
					t.Fatal(err)
				}
			case "target_unreadable":
				write(f.item.SourcePath, []byte("PRIVATE_MARKER"))
				if err := os.Chmod(f.item.SourcePath, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(f.item.SourcePath, 0600) })
			case "q_unreadable":
				if err := os.Chmod(f.item.QuarantinePath, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(f.item.QuarantinePath, 0600) })
			case "target_symlink":
				outside := filepath.Join(t.TempDir(), "outside")
				write(outside, f.original)
				if err := os.Symlink(outside, f.item.SourcePath); err != nil {
					t.Fatal(err)
				}
			case "q_symlink":
				outside := filepath.Join(t.TempDir(), "outside")
				write(outside, f.original)
				remove(f.item.QuarantinePath)
				if err := os.Symlink(outside, f.item.QuarantinePath); err != nil {
					t.Fatal(err)
				}
			case "source_root_unreadable", "q_root_unreadable":
				root := f.source
				if mode == "q_root_unreadable" {
					root = f.quarantine
				}
				if err := os.Chmod(root, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(root, 0700) })
			case "source_root_symlink", "q_root_symlink", "source_root_missing", "q_root_missing":
				root := f.source
				if mode == "q_root_symlink" || mode == "q_root_missing" {
					root = f.quarantine
				}
				saved := root + "-preserved"
				if err := os.Rename(root, saved); err != nil {
					t.Fatal(err)
				}
				if strings.HasSuffix(mode, "symlink") {
					if err := os.Symlink(saved, root); err != nil {
						t.Fatal(err)
					}
				}
				t.Cleanup(func() { _ = os.Remove(root); _ = os.Rename(saved, root) })
			case "out_of_scope":
				f.source = t.TempDir()
			}
			beforeS, _ := os.ReadFile(f.item.SourcePath)
			beforeQ, _ := os.ReadFile(f.item.QuarantinePath)
			f.recover(t, mode != "absent_target")
			if mode != "absent_target" {
				f.recover(t, true)
				f.reopen(t)
				f.recover(t, true)
				afterS, _ := os.ReadFile(f.item.SourcePath)
				afterQ, _ := os.ReadFile(f.item.QuarantinePath)
				if !bytes.Equal(beforeS, afterS) || !bytes.Equal(beforeQ, afterQ) {
					t.Fatal("ambiguous evidence changed")
				}
			} else {
				f.reopen(t)
				rs, err := f.svc.RecoverRestores(context.Background(), RecoverRestoresInput{QuarantineRoot: f.quarantine, SourceRoots: []string{f.source}})
				if err != nil || len(rs) != 0 {
					t.Fatal("terminal recovery repeated")
				}
			}
			logs, err := f.st.ListLogs(context.Background(), f.item.PlanID)
			if err != nil || (len(logs) == 0 && mode != "source_root_symlink" && mode != "q_root_symlink" && mode != "source_root_missing" && mode != "q_root_missing") {
				t.Fatal("missing recovery audit")
			}
			for _, l := range logs {
				b, _ := json.Marshal(l.Detail)
				if bytes.Contains(b, []byte("PRIVATE_MARKER")) || bytes.Contains(b, []byte(f.item.QuarantinePath)) {
					t.Fatal("audit leaked private file identity")
				}
			}
		})
	}
}

func TestRestoreManualPreservationThenSafeClosure(t *testing.T) {
	f := newRestoreSafetyFixture(t)
	f.begin(t)
	partial := []byte("preserve this partial evidence")
	if err := os.WriteFile(f.item.SourcePath, partial, 0600); err != nil {
		t.Fatal(err)
	}
	f.recover(t, true)
	f.reopen(t)
	f.recover(t, true)
	// Explicit, test-only manual reconciliation. No backend force-clear or deletion.
	retained := filepath.Join(t.TempDir(), "retained-partial")
	if err := os.Rename(f.item.SourcePath, retained); err != nil {
		t.Fatal(err)
	}
	f.recover(t, false)
	f.reopen(t)
	got, _ := os.ReadFile(retained)
	q, _ := os.ReadFile(f.item.QuarantinePath)
	if !bytes.Equal(got, partial) || !bytes.Equal(q, f.original) {
		t.Fatal("manual evidence or complete copy changed")
	}
	_, err := f.svc.ExecuteRestore(context.Background(), RestoreExecuteInput{PlanID: f.plan.ID, Digest: f.plan.ApprovalDigest, QuarantineRoot: f.quarantine, SourceRoots: []string{f.source}})
	if err == nil {
		t.Fatal("old approval reused after recovery")
	}
	p, err := f.svc.CreateRestorePlan(context.Background(), f.item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.ApproveRestorePlan(context.Background(), p.ID, p.ApprovalDigest); err != nil {
		t.Fatal(err)
	}
	out, err := f.svc.ExecuteRestore(context.Background(), RestoreExecuteInput{PlanID: p.ID, Digest: p.ApprovalDigest, QuarantineRoot: f.quarantine, SourceRoots: []string{f.source}})
	if err != nil || out.Result.FinalState != domain.RestoreCompleted {
		t.Fatal("new independently approved restore failed")
	}
	restored, _ := os.ReadFile(f.item.SourcePath)
	if !bytes.Equal(restored, f.original) {
		t.Fatal("fresh restore bytes mismatch")
	}
}

func TestRestoreTerminalPersistenceFailsClosed(t *testing.T) {
	for _, table := range []string{"restore_journal", "restore_plans", "operation_logs"} {
		t.Run(table, func(t *testing.T) {
			f := newRestoreSafetyFixture(t)
			f.begin(t)
			db, err := sql.Open("sqlite", f.db)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			verb := "UPDATE"
			if table == "operation_logs" {
				verb = "INSERT"
			}
			if _, err = db.Exec("CREATE TRIGGER fail_terminal BEFORE " + verb + " ON " + table + " BEGIN SELECT RAISE(ABORT,'PRIVATE_MARKER confidential path'); END"); err != nil {
				t.Fatal(err)
			}
			f.recover(t, true)
			f.reopen(t)
			f.recover(t, true)
			if _, err = db.Exec("DROP TRIGGER fail_terminal"); err != nil {
				t.Fatal(err)
			}
			f.recover(t, false)
		})
	}
}

func TestRestoreReservationCASAndOwnerLock(t *testing.T) {
	f := newRestoreSafetyFixture(t)
	ctx := context.Background()
	altered := f.plan
	altered.ExpectedSize++
	if err := f.st.BeginRestore(ctx, altered, f.item, time.Now()); err == nil {
		t.Fatal("changed caller identity reserved")
	}
	f.begin(t)
	if err := f.st.BeginRestore(ctx, f.plan, f.item, time.Now()); err == nil {
		t.Fatal("duplicate reservation accepted")
	}
	second, err := store.Open(ctx, f.db)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	unlock, err := f.st.AcquireExecutionLock()
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewQuarantineService(second).RecoverRestores(ctx, RecoverRestoresInput{QuarantineRoot: f.quarantine, SourceRoots: []string{f.source}})
	unlock()
	if err == nil {
		t.Fatal("concurrent recovery owner accepted")
	}
	if err = f.st.MarkRestoreCompleted(ctx, f.plan.ID, "wrong-item", time.Now()); err == nil {
		t.Fatal("wrong lifecycle identity accepted")
	}
	f.recover(t, false)
	if err = f.st.MarkRestoreRolledBack(ctx, f.plan.ID, time.Now()); err == nil {
		t.Fatal("duplicate terminal accepted")
	}
	if err = f.st.MarkRestoreCompleted(ctx, f.plan.ID, f.item.ID, time.Now()); err == nil {
		t.Fatal("terminal overwritten")
	}
}

type restoreFaultStore struct {
	*store.SQLiteStore
	afterBegin                         func()
	completionFailure, rollbackFailure bool
	rolledBackCalls                    int
}

func (s *restoreFaultStore) BeginRestore(ctx context.Context, p domain.RestorePlan, q domain.QuarantineItem, at time.Time) error {
	if err := s.SQLiteStore.BeginRestore(ctx, p, q, at); err != nil {
		return err
	}
	if s.afterBegin != nil {
		s.afterBegin()
	}
	return nil
}
func (s *restoreFaultStore) MarkRestoreCompleted(ctx context.Context, p, q string, at time.Time) error {
	if s.completionFailure {
		return errors.New("PRIVATE_MARKER injected DB failure")
	}
	return s.SQLiteStore.MarkRestoreCompleted(ctx, p, q, at)
}
func (s *restoreFaultStore) MarkRestoreRolledBack(ctx context.Context, p string, at time.Time) error {
	s.rolledBackCalls++
	if s.rollbackFailure {
		return errors.New("PRIVATE_MARKER injected rollback failure")
	}
	return s.SQLiteStore.MarkRestoreRolledBack(ctx, p, at)
}
func TestRestoreExecuteFailureRetainsPending(t *testing.T) {
	for _, mode := range []string{"destination_replaced", "completion_persistence", "rollback_persistence"} {
		t.Run(mode, func(t *testing.T) {
			f := newRestoreSafetyFixture(t)
			fault := &restoreFaultStore{SQLiteStore: f.st}
			switch mode {
			case "destination_replaced":
				fault.afterBegin = func() {
					if err := os.WriteFile(f.item.SourcePath, []byte("partial externally replaced evidence"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "completion_persistence":
				fault.completionFailure = true
			case "rollback_persistence":
				fault.rollbackFailure = true
				fault.afterBegin = func() {
					if err := os.Chmod(f.source, 0500); err != nil {
						t.Fatal(err)
					}
				}
				t.Cleanup(func() { _ = os.Chmod(f.source, 0700) })
			}
			ex, err := executor.NewRestoreExecutor(f.quarantine, []string{f.source}, fault)
			if err != nil {
				t.Fatal(err)
			}
			result := ex.ExecuteRestore(context.Background(), &f.plan, &f.item)
			if result.Status != executor.StepFailed || result.Err == nil || result.FinalState == domain.RestoreRolledBack {
				t.Fatal("failed write/rollback incorrectly terminalized")
			}
			if strings.Contains(result.Err.Error(), "PRIVATE_MARKER") {
				t.Fatal("raw persistence error leaked")
			}
			pending, err := f.st.ListPendingRestores(context.Background())
			if err != nil || len(pending) != 1 || requireNoRecovery(context.Background(), f.st) == nil {
				t.Fatal("execution failure released lock")
			}
			if mode == "completion_persistence" {
				if fault.rolledBackCalls != 0 {
					t.Fatal("unproven target auto rolled back")
				}
				r, err := executor.Snapshot(f.item.SourcePath, true)
				if err != nil || r.Hash != f.item.ContentSHA256 {
					t.Fatal("full target lost after persistence failure")
				}
			}
			if mode == "rollback_persistence" {
				if fault.rolledBackCalls != 1 {
					t.Fatal("rollback persistence failure not actually injected")
				}
				_ = os.Chmod(f.source, 0700)
			}
			f.reopen(t)
			pending, err = f.st.ListPendingRestores(context.Background())
			if err != nil || len(pending) != 1 {
				t.Fatal("restart lost unresolved execution")
			}
		})
	}
}

func TestRestoreCompletionTransactionFailurePreservesFullTarget(t *testing.T) {
	f := newRestoreSafetyFixture(t)
	db, err := sql.Open("sqlite", f.db)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TRIGGER fail_complete BEFORE INSERT ON operation_logs BEGIN SELECT RAISE(ABORT,'PRIVATE_MARKER'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.ExecuteRestore(context.Background(), RestoreExecuteInput{PlanID: f.plan.ID, Digest: f.plan.ApprovalDigest, QuarantineRoot: f.quarantine, SourceRoots: []string{f.source}})
	if err == nil || strings.Contains(err.Error(), "PRIVATE_MARKER") {
		t.Fatal("completion persistence failure not safely reported")
	}
	p, err := f.st.GetRestorePlan(context.Background(), f.plan.ID)
	if err != nil || p.State != domain.RestoreApproved {
		t.Fatal("failed transaction partially changed plan")
	}
	q, err := f.st.GetQuarantineItem(context.Background(), f.item.ID)
	if err != nil || q.Status != domain.QuarantineActive {
		t.Fatal("failed transaction partially changed lifecycle")
	}
	r, err := executor.Snapshot(f.item.SourcePath, true)
	if err != nil || r.Hash != f.item.ContentSHA256 {
		t.Fatal("unconfirmed target changed")
	}
	f.reopen(t)
	f.recover(t, true)
}
