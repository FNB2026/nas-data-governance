//go:build ndg_devprofile && !mas

package wails

import (
	"os"
	"path/filepath"
	"testing"
)

func newDevProfile(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("", "ndg-b2-")
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.WriteFile(filepath.Join(root, ".ndg-b2-profile"), []byte(devProfileMarker), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestDevProfileRequiresDisposablePrivateRoot(t *testing.T) {
	for _, root := range []string{"", ".", t.TempDir(), filepath.Join(newDevProfile(t), ".."), filepath.Join(newDevProfile(t), "support")} {
		if _, err := NewIsolatedDevAPI(root); err != errDevProfile {
			t.Fatal("unsafe profile accepted")
		}
	}
	for _, change := range []func(string){
		func(r string) { _ = os.Remove(filepath.Join(r, ".ndg-b2-profile")) },
		func(r string) { _ = os.Chmod(r, 0o755) },
		func(r string) { _ = os.WriteFile(filepath.Join(r, ".ndg-b2-profile"), []byte("wrong"), 0o600) },
		func(r string) { _ = os.Symlink(t.TempDir(), filepath.Join(r, "support")) },
	} {
		r := newDevProfile(t)
		change(r)
		if _, err := NewIsolatedDevAPI(r); err != errDevProfile {
			t.Fatal("invalid profile accepted")
		}
	}
}

func TestDevProfileProjectAndRecentStayIsolatedAcrossRestart(t *testing.T) {
	root := newDevProfile(t)
	a, err := NewIsolatedDevAPI(root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := a.CreateProjectFromSource(CreateProjectInput{Name: "synthetic-b2", SourcePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "support", "projects") + string(filepath.Separator)
	if len(info.Path) <= len(want) || info.Path[:len(want)] != want {
		t.Fatal("database escaped dev profile")
	}
	if err := a.CloseProject(); err != nil {
		t.Fatal(err)
	}
	b, err := NewIsolatedDevAPI(root)
	if err != nil {
		t.Fatal(err)
	}
	recent, err := b.ListRecentProjects()
	if err != nil || len(recent) != 1 || recent[0].Path != info.Path {
		t.Fatal("isolated recent registry did not persist")
	}
	if _, err := os.Stat(filepath.Join(root, "support", "recent.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "support"), filepath.Join(root, "previous")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "previous"), filepath.Join(root, "support")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ListRecentProjects(); err == nil {
		t.Fatal("profile substitution was not rejected")
	}
}

func TestDevProfileLauncherTempRootAndRootReplacement(t *testing.T) {
	parent, err := filepath.EvalSymlinks("/tmp")
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(parent, "ndg-b2-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	if err := os.WriteFile(filepath.Join(root, ".ndg-b2-profile"), []byte(devProfileMarker), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := NewIsolatedDevAPI(root)
	if err != nil {
		t.Fatal("launcher temporary directory rejected")
	}
	previous := root + "-preserved"
	if err := os.Rename(root, previous); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(previous) })
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".ndg-b2-profile"), []byte(devProfileMarker), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ListRecentProjects(); err == nil {
		t.Fatal("replacement root was adopted")
	}
}
