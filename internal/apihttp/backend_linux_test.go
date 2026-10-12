//go:build linux

package apihttp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func fixtures(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	state := filepath.Join(base, "state")
	root := filepath.Join(base, "source")
	if e := os.Mkdir(state, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(root, 0700); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		for j := 0; j < 2; j++ {
			if e := os.WriteFile(filepath.Join(root, fmt.Sprintf("private_marker_%d_%d", i, j)), []byte(strings.Repeat(fmt.Sprintf("pair_%d", i), 50)), 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	os.WriteFile(filepath.Join(root, "private_singleton"), []byte("singleton"), 0600)
	return state, root
}
func wait(t *testing.T, b *LocalBackend) Snapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		v, e := b.Snapshot("source_a")
		if e != nil {
			t.Fatal(e)
		}
		if v.State == "COMPLETED" || v.State == "FAILED" || v.State == "CANCELLED" {
			return v
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no terminal state")
	return Snapshot{}
}
func TestLinuxSharedCoreScanAndRestart(t *testing.T) {
	state, root := fixtures(t)
	before := map[string][32]byte{}
	entries, _ := os.ReadDir(root)
	for _, f := range entries {
		data, _ := os.ReadFile(filepath.Join(root, f.Name()))
		before[f.Name()] = sha256.Sum256(data)
	}
	b, e := OpenLocal(state, map[string]string{"source_a": root})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := OpenLocal(state, map[string]string{"source_a": root}); e == nil {
		t.Fatal("second writer")
	}
	if e = b.Start("source_a", false); e != nil {
		t.Fatal(e)
	}
	v := wait(t, b)
	if v.State != "COMPLETED" || v.Active != 7 || v.Quick != 7 || v.Full != 6 || len(v.Groups) != 3 || v.Missing != 0 || v.Unavailable != 0 || v.Checkpoint != "completed" {
		t.Fatalf("coverage %+v", v)
	}
	data, _ := json.Marshal(v)
	if strings.Contains(string(data), root) || strings.Contains(string(data), "private_") {
		t.Fatal("private response")
	}
	if b.Start("source_a", true) == nil {
		t.Fatal("completed resume accepted")
	}
	if e = b.Close(); e != nil {
		t.Fatal(e)
	}
	b, e = OpenLocal(state, map[string]string{"source_a": root})
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	v, e = b.Snapshot("source_a")
	if e != nil || v.Active != 7 || v.Full != 6 {
		t.Fatal("restart lost state", e)
	}
	for name, h := range before {
		data, _ := os.ReadFile(filepath.Join(root, name))
		if sha256.Sum256(data) != h {
			t.Fatal("source changed")
		}
	}
}
func TestLinuxRootBoundaryAndIdentity(t *testing.T) {
	state, root := fixtures(t)
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "outside"), []byte("secret"), 0600)
	os.Symlink(outside, filepath.Join(root, "escape"))
	b, e := OpenLocal(state, map[string]string{"source_a": root})
	if e != nil {
		t.Fatal(e)
	}
	s := b.sources["source_a"]
	if _, e = s.open("escape/outside", 0); e == nil {
		t.Fatal("symlink followed")
	}
	if _, e = s.hash(true)(filepath.Join(root, "../outside"), 6); e == nil {
		t.Fatal("outside accepted")
	}
	if e = b.Start("unknown", false); e == nil {
		t.Fatal("unknown alias")
	}
	if e = b.Start("source_a", false); e != nil {
		t.Fatal(e)
	}
	v := wait(t, b)
	if v.Active != 7 {
		t.Fatal("symlink traversal")
	}
	b.Close()
	os.Rename(root, root+"-original")
	os.Mkdir(root, 0700)
	if _, e = OpenLocal(state, map[string]string{"source_a": root}); e == nil {
		t.Fatal("replacement inherited state")
	}
	if _, e = OpenLocal(root, map[string]string{"source_a": root}); e == nil {
		t.Fatal("state inside source")
	}
}
func TestLinuxReaderRejectsMountAndBindingChange(t *testing.T) {
	state, root := fixtures(t)
	b, e := OpenLocal(state, map[string]string{"source_a": root})
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	s := b.sources["source_a"]
	os.Rename(root, root+"-moved")
	os.Mkdir(root, 0700)
	if s.check(context.Background()) == nil {
		t.Fatal("changed binding")
	}
	if _, e = s.readDir(root); e == nil {
		t.Fatal("read after replacement")
	} // Synthetic NO_XDEV check uses a known child mount, without changing any mount.
	fd, e := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer unix.Close(fd)
	f, e := unix.Openat2(fd, "proc", &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_XDEV | unix.RESOLVE_NO_SYMLINKS})
	if e == nil {
		unix.Close(f)
		t.Fatal("child mount accepted")
	}
}

func TestLinuxPreflightHasNoOutsideStateWrites(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	os.Mkdir(real, 0700)
	link := filepath.Join(base, "link")
	os.Symlink(real, link)
	root := filepath.Join(base, "source")
	os.Mkdir(root, 0700)
	if _, e := OpenLocal(filepath.Join(link, "newstate"), map[string]string{"source_a": root}); e == nil {
		t.Fatal("unsafe state accepted")
	}
	if _, e := os.Stat(filepath.Join(real, "newstate")); !os.IsNotExist(e) {
		t.Fatal("state created before validation")
	}
}

func TestLinuxCancelResumeAndRecoveryLock(t *testing.T) {
	state, root := fixtures(t)
	b, e := OpenLocal(state, map[string]string{"source_a": root})
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	s := b.sources["source_a"]
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	quick := s.hash(false)
	gated := func(path string, size int64) (string, error) {
		once.Do(func() { close(entered); <-release })
		return quick(path, size)
	}
	s.scan = app.NewScanServiceWithHashFunc(s.st, gated, s.hash(true))
	s.runner = app.NewScanJobRunner(s.scan, s.manager)
	if e = b.Start("source_a", false); e != nil {
		t.Fatal(e)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("hash never entered")
	}
	if b.Start("source_a", false) == nil {
		t.Fatal("concurrent scan")
	}
	if e = b.Cancel("source_a"); e != nil {
		t.Fatal(e)
	}
	close(release)
	v := wait(t, b)
	if v.State != "CANCELLED" {
		t.Fatal(v.State)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		running := s.running
		b.mu.Unlock()
		if !running {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if e = b.Start("source_a", true); e != nil {
		t.Fatal(e)
	}
	v = wait(t, b)
	if v.State != "COMPLETED" || v.Active != 7 || v.Full != 6 || v.Missing != 0 {
		t.Fatalf("resume %+v", v)
	}
	if e = s.st.CreateTask(context.Background(), domain.OperationTask{ID: "synthetic_task", RootPath: root, State: "queued", CreatedAt: time.Now()}); e != nil {
		t.Fatal(e)
	}
	if e = s.st.SavePlans(context.Background(), "synthetic_task", []domain.OperationPlan{{ID: "synthetic_plan", TaskID: "synthetic_task", State: domain.PlanExecuting}}); e != nil {
		t.Fatal(e)
	}
	if b.Start("source_a", false) == nil {
		t.Fatal("recovery lock bypassed")
	}
}
