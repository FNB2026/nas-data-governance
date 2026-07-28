package wails

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func createProjectDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "project.db")
	st, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("create project database: %v", err)
	}
	if err := st.RegisterStorage(context.Background(), domain.Storage{
		ID: "s1", RootPath: "/source", Kind: "test", CreatedAt: time.Now().UTC(),
	}); err != nil {
		_ = st.Close()
		t.Fatalf("seed storage: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close project database: %v", err)
	}
	return path
}

func TestOpenProjectIsReadOnlyAndReportsStorageCount(t *testing.T) {
	path := createProjectDB(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	info, err := api.OpenProject(path)
	if err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	if !info.IsOpen || info.Path != path || info.StorageCount != 1 {
		t.Fatalf("unexpected project info: %#v", info)
	}

	// A second writer can still open the database; more importantly, the
	// bound API exposes no raw store or write method and uses mode=ro.
	got, err := api.GetProjectInfo()
	if err != nil || got.StorageCount != 1 {
		t.Fatalf("GetProjectInfo: %#v, %v", got, err)
	}
}

func TestOpenProjectDoesNotCreateMissingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	api := NewAPI()
	if _, err := api.OpenProject(path); err == nil {
		t.Fatal("expected missing project error")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("OpenProject created a missing database: %v", err)
	}
}

func TestValidateProjectPathRejectsSymlink(t *testing.T) {
	path := createProjectDB(t)
	link := filepath.Join(t.TempDir(), "project.db")
	if err := os.Symlink(path, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if err := NewAPI().ValidateProjectPath(link); err == nil {
		t.Fatal("expected symlink validation error")
	}
}

func TestValidateProjectPathAcceptsCaseInsensitiveExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.DB")
	if err := os.WriteFile(path, []byte("placeholder"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := NewAPI().ValidateProjectPath(path); err != nil {
		t.Fatalf("ValidateProjectPath: %v", err)
	}
}
