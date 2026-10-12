package wails

import (
	"context"
	"errors"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/fileaccess"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type nativeTestGrant struct {
	mu   sync.Mutex
	root string
	fail bool
}

func (n *nativeTestGrant) Pick(context.Context) (fileaccess.Grant, func(), error) {
	return n.Resolve(context.Background(), fileaccess.Grant{Bookmark: "sealed"})
}
func (n *nativeTestGrant) Resolve(context.Context, fileaccess.Grant) (fileaccess.Grant, func(), error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.fail {
		return fileaccess.Grant{}, nil, fileaccess.ErrAuthorizationRequired
	}
	return fileaccess.Grant{Root: n.root, Bookmark: "sealed", Identity: "directory-identity"}, func() {}, nil
}
func sandboxTestAPI(t *testing.T) (*API, *nativeTestGrant, string) {
	t.Helper()
	base, _ := filepath.EvalSymlinks(t.TempDir())
	root, _ := filepath.EvalSymlinks(t.TempDir())
	n := &nativeTestGrant{root: root}
	m, e := fileaccess.New(filepath.Join(base, "grants"), n)
	if e != nil {
		t.Fatal(e)
	}
	a := NewSandboxAPI(m, base)
	t.Cleanup(func() { a.shutdownAccess() })
	return a, n, root
}
func TestSandboxDirectEntryPointsFailBeforeSideEffects(t *testing.T) {
	a, _, root := sandboxTestAPI(t)
	if _, e := a.CreateProjectFromSource(CreateProjectInput{SourcePath: root}); !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatal("path-only create bypass")
	}
	if _, e := a.OpenProjectReadWrite(filepath.Join(root, "private.db")); !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatal("external DB bypass")
	}
	if _, e := a.PreflightSource(root); !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatal("path-only probe bypass")
	}
	if _, e := a.ExecutePlans(ExecutePlansRequest{}); !errors.Is(e, fileaccess.ErrWriteDisabled) {
		t.Fatal("quarantine bypass")
	}
	if _, e := a.ExecuteRestore(ExecuteRestoreRequest{}); !errors.Is(e, fileaccess.ErrWriteDisabled) {
		t.Fatal("restore bypass")
	}
	if _, e := a.ExecutePurge(ExecutePurgeRequest{}); !errors.Is(e, fileaccess.ErrWriteDisabled) {
		t.Fatal("purge bypass")
	}
}
func TestSandboxGrantLossKeepsIndexedFilesActive(t *testing.T) {
	a, n, root := sandboxTestAPI(t)
	if e := os.WriteFile(filepath.Join(root, "artificial"), []byte("disposable"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := a.PickDirectory(""); e != nil {
		t.Fatal(e)
	}
	if _, e := a.CreateProjectFromSource(CreateProjectInput{SourcePath: root}); e != nil {
		t.Fatal(e)
	}
	job, e := a.StartScan(StartScanRequest{Root: root})
	if e != nil {
		t.Fatal(e)
	}
	waitSandboxJob(t, a, job.JobID)

	n.mu.Lock()
	n.fail = true
	n.mu.Unlock()
	if _, e := a.StartScan(StartScanRequest{Root: root, Resume: true}); !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatal("lost permission accepted")
	}
	st, e := a.store.ListStorages(context.Background())
	if e != nil || len(st) != 1 {
		t.Fatal("registration mutated")
	}
	recent, e := a.store.ListJobsByProject(context.Background(), a.jobScope, 20)
	if e != nil || len(recent) != 1 {
		t.Fatal("failed authorization created jobs")
	}
	files, e := a.store.ListFiles(context.Background(), st[0].ID)
	if e != nil || len(files) != 1 {
		t.Fatal("authorization loss changed coverage")
	}
}

func waitSandboxJob(t *testing.T, a *API, id string) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		p, e := a.GetScanProgress(id)
		if e != nil {
			t.Fatal(e)
		}
		if jobs.JobState(p.State).IsTerminal() {
			for a.activeScans.Load() != 0 {
				time.Sleep(time.Millisecond)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not reach terminal")
}
func TestSandboxRepeatedResumeAndReopen(t *testing.T) {
	a, _, root := sandboxTestAPI(t)
	for _, name := range []string{"one", "two"} {
		if e := os.WriteFile(filepath.Join(root, name), []byte("synthetic pair"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	_, _ = a.PickDirectory("")
	info, e := a.CreateProjectFromSource(CreateProjectInput{Name: "reopen", SourcePath: root})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		j, e := a.StartScan(StartScanRequest{Root: root, Resume: true})
		if e != nil {
			t.Fatal(e)
		}
		waitSandboxJob(t, a, j.JobID)
	}
	if e := a.CloseProject(); e != nil {
		t.Fatal(e)
	}
	if _, e := a.OpenProjectReadWrite(info.Path); e != nil {
		t.Fatal(e)
	}
	storages, e := a.store.ListStorages(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	files, e := a.store.ListFiles(context.Background(), storages[0].ID)
	if e != nil || len(files) != 2 || files[0].ContentSHA256 == "" || files[1].ContentSHA256 == "" {
		t.Fatal("repeat resume changed durable coverage")
	}
	caps, e := a.GetAppCapabilities()
	if e != nil || caps.CanExecuteQuarantine || caps.CanExecutePurge {
		t.Fatal("sandbox exposed writes")
	}
}

func TestReauthorizationNeverClearsRecoveryLock(t *testing.T) {
	a, n, root := sandboxTestAPI(t)
	_, _ = a.PickDirectory("")
	_, e := a.CreateProjectFromSource(CreateProjectInput{Name: "locked", SourcePath: root})
	if e != nil {
		t.Fatal(e)
	}
	task := domain.OperationTask{ID: "synthetic-task", CreatedAt: time.Now()}
	if e = a.store.CreateTask(context.Background(), task); e != nil {
		t.Fatal(e)
	}
	p := domain.OperationPlan{ID: "synthetic-execution", TaskID: task.ID, State: domain.PlanExecuting}
	if e = a.store.SavePlans(context.Background(), "synthetic-task", []domain.OperationPlan{p}); e != nil {
		t.Fatal(e)
	}
	n.mu.Lock()
	n.fail = true
	n.mu.Unlock()
	if _, e = a.PreflightSource(root); !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatal("permission loss was ignored")
	}
	n.mu.Lock()
	n.fail = false
	n.mu.Unlock()
	if _, e = a.PickDirectory(""); e != nil {
		t.Fatal(e)
	}
	lock, e := a.CheckRecoveryLock()
	if e != nil || !lock.LockActive {
		t.Fatal("authorization cleared durable uncertainty")
	}
	if _, e = a.StartScan(StartScanRequest{Root: root}); e == nil {
		t.Fatal("scan bypassed recovery lock")
	}
	if _, e = a.RecoverSourcePlans(); !errors.Is(e, fileaccess.ErrWriteDisabled) {
		t.Fatal("write recovery bypass")
	}
}

func TestAllSandboxFilesystemEntryPointsStayDeniedWithoutAuthority(t *testing.T) {
	a, _, root := sandboxTestAPI(t)
	if e := a.ValidateProjectPath(filepath.Join(root, "governance.db")); !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatal("project validation probed ungranted path")
	}
	checks := []func() error{
		func() error { _, e := a.CreateRestorePlan("synthetic"); return e },
		func() error { return a.ApproveRestorePlan("synthetic", "digest") },
		func() error { _, e := a.CreatePurgePlans(); return e },
		func() error { return a.ApprovePurgePlan("synthetic", "digest") },
		func() error { _, e := a.RecoverRestores(RecoverRestoresRequest{}); return e },
		func() error { _, e := a.RecoverPurges(root); return e },
		func() error { _, e := a.RecoverSourcePlans(); return e },
		func() error { _, e := a.DiagnoseFormats(DiagnoseFormatsRequest{}); return e },
		func() error { _, e := a.DiagnoseGovernance(DiagnoseGovernanceRequest{}); return e },
		func() error { _, e := a.DiagnoseMerges(DiagnoseMergesRequest{}); return e },
	}
	for i, fn := range checks {
		if e := fn(); !errors.Is(e, fileaccess.ErrWriteDisabled) {
			t.Fatalf("entry %d escaped policy", i)
		}
	}
}
