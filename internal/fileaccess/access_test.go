package fileaccess

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

type fakeNative struct {
	mu               sync.Mutex
	root             string
	fail             error
	started, stopped int
}

func (f *fakeNative) Pick(context.Context) (Grant, func(), error) {
	return f.Resolve(context.Background(), Grant{Bookmark: "token"})
}
func (f *fakeNative) Resolve(_ context.Context, saved Grant) (Grant, func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return Grant{}, nil, f.fail
	}
	if saved.Bookmark != "token" {
		return Grant{}, nil, ErrAuthorizationRequired
	}
	f.started++
	return Grant{Root: f.root, Bookmark: "token", Identity: "stable-local-id"}, func() { f.mu.Lock(); defer f.mu.Unlock(); f.stopped++ }, nil
}
func setup(t *testing.T) (*Manager, *fakeNative, string) {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	backend := &fakeNative{root: root}
	m, err := New(realTemp(t), backend)
	if err != nil {
		t.Fatal(err)
	}
	return m, backend, root
}
func TestPathAloneCannotGrantAndErrorsAreRedacted(t *testing.T) {
	m, _, root := setup(t)
	if _, err := m.Acquire(context.Background(), root); !errors.Is(err, ErrAuthorizationRequired) {
		t.Fatal("unselected root acquired")
	}
}
func TestPersistenceLeaseAndRepeatedRelease(t *testing.T) {
	m, b, root := setup(t)
	if _, err := m.Select(context.Background()); err != nil {
		t.Fatal(err)
	}
	again, err := New(m.dir, b)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := again.Acquire(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if b.stopped != 1 {
		t.Fatal("selection scope leaked or scan scope prematurely stopped")
	}
	lease.Release()
	lease.Release()
	if b.started != b.stopped {
		t.Fatal("unbalanced leases")
	}
}
func TestCorruptBookmarkAndMovedRootFailClosed(t *testing.T) {
	m, b, root := setup(t)
	_, _ = m.Select(context.Background())
	b.root = t.TempDir()
	if _, err := m.Acquire(context.Background(), root); !errors.Is(err, ErrAuthorizationRequired) {
		t.Fatal("moved root accepted")
	}
	b.root = root
	if err := os.WriteFile(m.recordPath(root), []byte("bad-private-record"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Acquire(context.Background(), root); !errors.Is(err, ErrAuthorizationRequired) {
		t.Fatal("corrupt grant accepted")
	}
	if b.started != b.stopped {
		t.Fatal("failed acquire leaked scope")
	}
}
func TestConcurrencyAndClose(t *testing.T) {
	m, b, root := setup(t)
	_, _ = m.Select(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l, e := m.Acquire(context.Background(), root)
			if e != nil {
				t.Error(e)
				return
			}
			l.Release()
		}()
	}
	wg.Wait()
	m.Close()
	if _, e := m.Acquire(context.Background(), root); !errors.Is(e, ErrAuthorizationRequired) {
		t.Fatal("closed manager accepted lease")
	}
	if b.started != b.stopped {
		t.Fatal("unbalanced concurrent leases")
	}
}
func TestRepositoryAndSymlinkBoundaries(t *testing.T) {
	m, _, root := setup(t)
	_, _ = m.Select(context.Background())
	other := t.TempDir()
	if e := os.Remove(m.recordPath(root)); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(filepath.Join(other, "outside"), m.recordPath(root)); e != nil {
		t.Fatal(e)
	}
	if _, e := m.Acquire(context.Background(), root); !errors.Is(e, ErrAuthorizationRequired) {
		t.Fatal("grant symlink accepted")
	}
}

func realTemp(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestUnavailableAndPermissionClassifications(t *testing.T) {
	m, b, root := setup(t)
	_, _ = m.Select(context.Background())
	b.fail = ErrSourceUnavailable
	if _, e := m.Acquire(context.Background(), root); !errors.Is(e, ErrSourceUnavailable) {
		t.Fatal("local absence misclassified")
	}
	b.fail = ErrAuthorizationRequired
	if _, e := m.Acquire(context.Background(), root); !errors.Is(e, ErrAuthorizationRequired) {
		t.Fatal("permission denial misclassified")
	}
	b.fail = nil
	g, stop, _ := b.Resolve(context.Background(), Grant{Bookmark: "token"})
	stop()
	g.Network = true
	if e := m.save(g); e != nil {
		t.Fatal(e)
	}
	b.fail = ErrSourceUnavailable
	if _, e := m.Acquire(context.Background(), root); !errors.Is(e, ErrNetworkPaused) {
		t.Fatal("network absence misclassified")
	}
}

func TestCloseWaitsForOutstandingLeaseAndBlocksNewBorrowers(t *testing.T) {
	m, _, root := setup(t)
	_, _ = m.Select(context.Background())
	lease, e := m.Acquire(context.Background(), root)
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	go func() { m.Close(); close(done) }()
	for {
		m.mu.Lock()
		closed := m.closed
		m.mu.Unlock()
		if closed {
			break
		}
		runtime.Gosched()
	}
	select {
	case <-done:
		t.Fatal("close prematurely released live authority")
	default:
	}
	if _, e = m.Acquire(context.Background(), root); !errors.Is(e, ErrAuthorizationRequired) {
		t.Fatal("closing manager accepted work")
	}
	lease.Release()
	<-done
}
func TestRepositoryReplacementFailsBeforeSavingAuthority(t *testing.T) {
	m, b, _ := setup(t)
	original := m.dir
	backup := original + "-retained"
	if e := os.Rename(original, backup); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(t.TempDir(), original); e != nil {
		t.Fatal(e)
	}
	if _, e := m.Select(context.Background()); !errors.Is(e, ErrAuthorizationRequired) {
		t.Fatal("repository substitution accepted")
	}
	if b.started != b.stopped {
		t.Fatal("persistence rejection leaked native lease")
	}
}
