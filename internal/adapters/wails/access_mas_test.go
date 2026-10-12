//go:build mas

package wails

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/fileaccess"
)

func TestMASDefaultConstructorAndPickerInjectionCannotBypass(t *testing.T) {
	a := NewAPI()
	a.SetDirectoryPicker(func(string) (string, error) { return t.TempDir(), nil })
	if _, e := a.PickDirectory(""); !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatal("legacy picker granted authority")
	}
	if _, e := a.CreateProjectFromSource(CreateProjectInput{SourcePath: t.TempDir()}); !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatal("default constructor bypassed grant policy")
	}
	if _, e := a.ExecuteRestore(ExecuteRestoreRequest{}); !errors.Is(e, fileaccess.ErrWriteDisabled) {
		t.Fatal("default constructor enabled source writes")
	}
}

func TestMASProjectDatabaseAndMetadataStayInInjectedContainer(t *testing.T) {
	a, _, source := sandboxTestAPI(t)
	if err := os.WriteFile(filepath.Join(source, "synthetic.txt"), []byte("disposable"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.PickDirectory(""); err != nil {
		t.Fatalf("select synthetic root: %v", err)
	}
	info, err := a.CreateProjectFromSource(CreateProjectInput{Name: "container-check", SourcePath: source})
	if err != nil {
		t.Fatalf("create disposable project: %v", err)
	}
	base, err := filepath.Abs(a.support)
	if err != nil {
		t.Fatal(err)
	}
	projectDB, err := filepath.Abs(info.Path)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(base, projectDB)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("project database escaped the injected sandbox support root: %v", err)
	}
	if filepath.Base(projectDB) != "governance.db" || filepath.Base(filepath.Dir(projectDB)) != "container-check" {
		t.Fatal("project database is not under the private projects container")
	}
	for _, path := range []string{projectDB, filepath.Join(filepath.Dir(projectDB), "project.json")} {
		st, err := os.Lstat(path)
		if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			t.Fatalf("project artifact is missing, linked, or not private: %v", err)
		}
	}
	for _, sidecar := range []string{projectDB + "-wal", projectDB + "-shm"} {
		if st, err := os.Lstat(sidecar); err == nil {
			if st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
				t.Fatal("SQLite sidecar is not a private regular file")
			}
		} else if !os.IsNotExist(err) {
			t.Fatalf("inspect sidecar: %v", err)
		}
	}
	if _, err := a.OpenProjectReadWrite(filepath.Join(source, "external.db")); !errors.Is(err, fileaccess.ErrAuthorizationRequired) {
		t.Fatalf("external project database escaped the container boundary: %v", err)
	}
	if _, err := os.Stat(filepath.Join(source, "synthetic.txt")); err != nil {
		t.Fatal("container setup modified the synthetic source")
	}
}
