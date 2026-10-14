package wails

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/fileaccess"
	"github.com/FNB2026/nas-data-governance/internal/fingerprint"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
)

type nativeTestGrant struct {
	mu           sync.Mutex
	root         string
	fail         bool
	afterResolve func()
	releases     int
}

func (n *nativeTestGrant) Pick(context.Context) (fileaccess.Grant, func(), error) {
	return n.Resolve(context.Background(), fileaccess.Grant{Bookmark: "sealed"})
}
func (n *nativeTestGrant) Resolve(context.Context, fileaccess.Grant) (fileaccess.Grant, func(), error) {
	n.mu.Lock()
	if n.fail {
		n.mu.Unlock()
		return fileaccess.Grant{}, nil, fileaccess.ErrAuthorizationRequired
	}
	root, afterResolve := n.root, n.afterResolve
	n.mu.Unlock()
	if afterResolve != nil {
		afterResolve()
	}
	return fileaccess.Grant{Root: root, Bookmark: "sealed", Identity: "directory-identity"}, func() {
		n.mu.Lock()
		n.releases++
		n.mu.Unlock()
	}, nil
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

func TestRegisterScanSourceIsSandboxOnlyAndHasNoScanOrRecoverySideEffects(t *testing.T) {
	api, grant, firstRoot := sandboxTestAPI(t)
	secondRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.PickDirectory(""); err != nil {
		t.Fatal(err)
	}
	if _, err := api.CreateProjectFromSource(CreateProjectInput{Name: "register-only", SourcePath: firstRoot}); err != nil {
		t.Fatal(err)
	}
	if err := api.store.CreateTask(context.Background(), domain.OperationTask{ID: "pending-task", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := api.store.SavePlans(context.Background(), "pending-task", []domain.OperationPlan{{
		ID: "pending-plan", TaskID: "pending-task", State: domain.PlanExecuting,
	}}); err != nil {
		t.Fatal(err)
	}
	beforeJobs, err := api.store.ListJobsByProject(context.Background(), api.jobScope, 20)
	if err != nil || len(beforeJobs) != 0 {
		t.Fatalf("initial jobs = %d, err=%v; want none", len(beforeJobs), err)
	}

	grant.mu.Lock()
	grant.root = secondRoot
	grant.mu.Unlock()
	if _, err := api.PickDirectory(""); err != nil {
		t.Fatal(err)
	}
	if _, err := api.RegisterScanSource(secondRoot); err != nil {
		t.Fatal(err)
	}
	storages, err := api.store.ListStorages(context.Background())
	if err != nil || len(storages) != 2 {
		t.Fatalf("registered storages = %d, err=%v; want two distinct roots", len(storages), err)
	}
	afterJobs, err := api.store.ListJobsByProject(context.Background(), api.jobScope, 20)
	if err != nil || len(afterJobs) != 0 {
		t.Fatalf("registration created scan jobs: count=%d err=%v", len(afterJobs), err)
	}
	lock, err := api.CheckRecoveryLock()
	if err != nil || !lock.LockActive {
		t.Fatalf("recovery lock after registration = %+v, err=%v; want active", lock, err)
	}
	if _, err := api.StartScan(StartScanRequest{Root: secondRoot, Resume: true}); err == nil {
		t.Fatal("source registration cleared recovery lock and allowed Resume")
	}
	finalJobs, err := api.store.ListJobsByProject(context.Background(), api.jobScope, 20)
	if err != nil || len(finalJobs) != 0 {
		t.Fatalf("blocked Resume created scan jobs: count=%d err=%v", len(finalJobs), err)
	}
}

func TestReauthorizationCapabilityFollowsSandboxBuild(t *testing.T) {
	regular := NewAPI()
	regularCapabilities, err := regular.GetAppCapabilities()
	if err != nil || regularCapabilities.CanReauthorizeSources {
		t.Fatalf("default API without an authorization manager exposed reauthorization = %v, err=%v; want false", regularCapabilities.CanReauthorizeSources, err)
	}
	unconfiguredSandbox := NewSandboxAPI(nil, t.TempDir())
	unconfiguredCapabilities, err := unconfiguredSandbox.GetAppCapabilities()
	if err != nil || unconfiguredCapabilities.CanReauthorizeSources {
		t.Fatalf("sandbox API without an authorization manager exposed reauthorization = %v, err=%v; want false", unconfiguredCapabilities.CanReauthorizeSources, err)
	}

	mas, _, _ := sandboxTestAPI(t)
	masCapabilities, err := mas.GetAppCapabilities()
	if err != nil || !masCapabilities.CanReauthorizeSources {
		t.Fatalf("sandbox API with an authorization manager capability = %v, err=%v; want true", masCapabilities.CanReauthorizeSources, err)
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

func TestSandboxConcurrentScanAndCloseCannotOvertakeLiveLease(t *testing.T) {
	a, _, root := sandboxTestAPI(t)
	for _, name := range []string{"one", "two"} {
		if e := os.WriteFile(filepath.Join(root, name), []byte("synthetic duplicate"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	_, _ = a.PickDirectory("")
	if _, e := a.CreateProjectFromSource(CreateProjectInput{Name: "concurrent", SourcePath: root}); e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	svc := app.NewScanServiceWithHashFunc(a.store, fingerprint.Quick, func(path string, _ int64) (string, error) {
		once.Do(func() { close(entered) })
		<-release
		return fingerprint.Full(path)
	})
	a.scanRunner = app.NewScanJobRunner(svc, a.jobMgr)
	j, e := a.StartScan(StartScanRequest{Root: root})
	if e != nil {
		t.Fatal(e)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking fixture never entered full hashing")
	}
	if _, e = a.StartScan(StartScanRequest{Root: root}); e == nil || e.Error() != "SCAN_BUSY" {
		t.Fatal("second scan shared progress state")
	}
	if e = a.CloseProject(); e == nil || e.Error() != "SCAN_BUSY" {
		t.Fatal("database closed beneath active IO")
	}
	if e = a.CancelScan(j.JobID); e != nil {
		t.Fatal(e)
	}
	if a.activeScans.Load() != 1 {
		t.Fatal("cancel prematurely released active IO")
	}
	close(release)
	released = true
	waitSandboxJob(t, a, j.JobID)
	p, e := a.GetScanProgress(j.JobID)
	if e != nil || p.State != "CANCELLED" {
		t.Fatal("cancelled worker did not persist terminal state")
	}
	all, e := a.store.ListJobsByProject(context.Background(), a.jobScope, 20)
	if e != nil || len(all) != 1 {
		t.Fatal("rejected concurrency created an extra job")
	}
}
