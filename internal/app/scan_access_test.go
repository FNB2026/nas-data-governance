package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/fileaccess"
	"github.com/FNB2026/nas-data-governance/internal/fingerprint"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func TestAuthorizationLostBeforeReconcilePreservesDurableCoverage(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "source")
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	source := filepath.Join(root, "artificial")
	if e := os.WriteFile(source, []byte("synthetic bytes"), 0600); e != nil {
		t.Fatal(e)
	}
	st, e := store.Open(ctx, filepath.Join(tmp, "governance.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	svc := NewScanService(st)
	in := ScanInput{Root: root, StorageID: "synthetic", Workers: 1, HashAttempts: 1}
	if _, e = svc.Scan(ctx, in); e != nil {
		t.Fatal(e)
	}
	// Simulates invisible coverage, not a real authorization grant. The late
	// capability check MUST fail before the otherwise-complete empty traversal
	// can mark the previously indexed record missing.
	if e = os.Remove(source); e != nil {
		t.Fatal(e)
	}
	var calls atomic.Int32
	in.AccessCheck = func(context.Context) error {
		if calls.Add(1) > 1 {
			return fileaccess.ErrAuthorizationRequired
		}
		return nil
	}
	_, e = svc.Scan(ctx, in)
	if !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatal("late authority loss accepted")
	}
	files, e := st.ListFiles(ctx, in.StorageID)
	if e != nil || len(files) != 1 {
		t.Fatal("authority loss reconciled missing")
	}
	cp, e := st.LastCheckpoint(ctx, in.StorageID)
	if e != nil || cp.Status != "aborted" {
		t.Fatal("authority loss declared completed")
	}
}
func TestSandboxJobCancellationDrainsLeaseAndPersistsTerminal(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "source")
	_ = os.Mkdir(root, 0700)
	_ = os.WriteFile(filepath.Join(root, "synthetic"), []byte("artificial"), 0600)
	st, e := store.Open(ctx, filepath.Join(tmp, "governance.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	mgr := jobs.New(st)
	r := NewScanJobRunner(NewScanService(st), mgr)
	lifecycle, cancel := context.WithCancel(ctx)
	entered := make(chan struct{})
	done := make(chan struct{})
	in := ScanInput{Root: root, StorageID: "synthetic", Workers: 1, HashAttempts: 1, AccessContext: lifecycle, AccessCheck: func(c context.Context) error { close(entered); <-c.Done(); return c.Err() }}
	id, e := r.StartScanJobWithCleanup(ctx, "synthetic-project", in, func() { close(done) })
	if e != nil {
		t.Fatal(e)
	}
	<-entered
	cancel()
	<-done
	job, e := mgr.Get(ctx, id)
	if e != nil || job.State != jobs.StateCancelled {
		t.Fatal("shutdown lost durable terminal or resource cleanup")
	}
}
func TestSandboxJobAuthorizationPauseIsExplicit(t *testing.T) {
	ctx := context.Background()
	st, e := store.Open(ctx, filepath.Join(t.TempDir(), "governance.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	mgr := jobs.New(st)
	r := NewScanJobRunner(NewScanService(st), mgr)
	_, _, e = r.RunScanAsJob(ctx, "synthetic", ScanInput{Root: t.TempDir(), HashAttempts: 1, Workers: 1, AccessCheck: func(context.Context) error { return fileaccess.ErrAuthorizationRequired }})
	if !errors.Is(e, jobs.ErrAuthorizationPaused) {
		t.Fatal("wrong authorization classification")
	}
	all, e := mgr.ListRecent(ctx, "synthetic", 10)
	if e != nil || len(all) != 1 || all[0].State != jobs.StatePausedAuthorization {
		t.Fatal("authorization not durably paused")
	}
}

func TestResumePrefixPermissionDenialIsAuthorizationPause(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "source")
	prefix := filepath.Join(root, "prefix")
	if e := os.MkdirAll(prefix, 0700); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"one", "two"} {
		if e := os.WriteFile(filepath.Join(prefix, name), []byte("synthetic duplicate"), 0600); e != nil {
			t.Fatal(e)
		}
	}
	st, e := store.Open(ctx, filepath.Join(tmp, "governance.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer st.Close()
	initial := NewScanServiceWithHashFunc(st, fingerprint.Quick, func(string, int64) (string, error) { return "", syscall.ENETDOWN })
	in := ScanInput{Root: root, StorageID: "prefix-permission", Workers: 1, HashAttempts: 1, NetworkSource: true}
	if _, e = initial.Scan(ctx, in); !errors.Is(e, ErrNetworkSourceUnavailable) {
		t.Fatal("fixture did not persist unfinished prefix candidates")
	}
	if e = os.Chmod(prefix, 0000); e != nil {
		t.Fatal(e)
	}
	defer os.Chmod(prefix, 0700)
	if _, e = os.ReadDir(prefix); !os.IsPermission(e) {
		t.Skip("host bypasses POSIX permission fixture; classification not certified by this test")
	}
	in.Resume = true
	in.AccessCheck = func(context.Context) error { return nil }
	_, e = NewScanService(st).Scan(ctx, in)
	if !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatalf("permission rejection classified incorrectly: %v", e)
	}
	files, e := st.ListFiles(ctx, in.StorageID)
	if e != nil || len(files) != 2 {
		t.Fatal("prefix records lost")
	}
	cp, e := st.LastCheckpoint(ctx, in.StorageID)
	if e != nil || cp.Status != "aborted" {
		t.Fatal("permission rejection certified content completion")
	}
}
