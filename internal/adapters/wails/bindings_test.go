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

// createProjectDBWithDuplicates creates a project database with 2 storages
// and seeded duplicate files for query testing:
//   - s1: 3 files sharing hash "aaa" (1000 bytes each) + 1 unique file
//   - s2: 2 files sharing hash "ccc" (2000 bytes each)
func createProjectDBWithDuplicates(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "project.db")
	st, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("create project database: %v", err)
	}
	ctx := context.Background()
	for _, s := range []domain.Storage{
		{ID: "s1", RootPath: "/source1", Kind: "local", CreatedAt: time.Now().UTC()},
		{ID: "s2", RootPath: "/source2", Kind: "local", CreatedAt: time.Now().UTC()},
	} {
		if err := st.RegisterStorage(ctx, s); err != nil {
			_ = st.Close()
			t.Fatalf("seed storage %s: %v", s.ID, err)
		}
	}
	now := time.Now().UTC()
	files := []domain.FileInstance{
		{StorageID: "s1", Path: "/source1/a.txt", Name: "a.txt", Size: 1000, Mode: 0644, ModifiedAt: now, ContentSHA256: "aaa", DiscoveredAt: now},
		{StorageID: "s1", Path: "/source1/b.txt", Name: "b.txt", Size: 1000, Mode: 0644, ModifiedAt: now, ContentSHA256: "aaa", DiscoveredAt: now},
		{StorageID: "s1", Path: "/source1/c.txt", Name: "c.txt", Size: 1000, Mode: 0644, ModifiedAt: now, ContentSHA256: "aaa", DiscoveredAt: now},
		{StorageID: "s1", Path: "/source1/unique.txt", Name: "unique.txt", Size: 500, Mode: 0644, ModifiedAt: now, ContentSHA256: "bbb", DiscoveredAt: now},
		{StorageID: "s2", Path: "/source2/x.txt", Name: "x.txt", Size: 2000, Mode: 0644, ModifiedAt: now, ContentSHA256: "ccc", DiscoveredAt: now},
		{StorageID: "s2", Path: "/source2/y.txt", Name: "y.txt", Size: 2000, Mode: 0644, ModifiedAt: now, ContentSHA256: "ccc", DiscoveredAt: now},
	}
	if _, err := st.UpsertFiles(ctx, files); err != nil {
		_ = st.Close()
		t.Fatalf("seed files: %v", err)
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

// ---- V3 Alpha query binding tests ----

func TestListStorages(t *testing.T) {
	path := createProjectDBWithDuplicates(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}

	storages, err := api.ListStorages()
	if err != nil {
		t.Fatalf("ListStorages: %v", err)
	}
	if len(storages) != 2 {
		t.Fatalf("expected 2 storages, got %d", len(storages))
	}
	if storages[0].ID != "s1" || storages[1].ID != "s2" {
		t.Errorf("storage IDs: expected s1,s2 got %s,%s", storages[0].ID, storages[1].ID)
	}
	if storages[0].RootPath != "/source1" {
		t.Errorf("s1 root: expected /source1, got %s", storages[0].RootPath)
	}
}

func TestListStoragesNoProjectOpen(t *testing.T) {
	api := NewAPI()
	_, err := api.ListStorages()
	if err != ErrNoProjectOpen {
		t.Errorf("expected ErrNoProjectOpen, got %v", err)
	}
}

func TestListDuplicateGroupsAllStorages(t *testing.T) {
	path := createProjectDBWithDuplicates(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}

	resp, err := api.ListDuplicateGroups(ListGroupsRequest{PageSize: 10})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if resp.TotalCount != 2 {
		t.Fatalf("TotalCount: expected 2, got %d", resp.TotalCount)
	}
	if len(resp.Groups) != 2 {
		t.Fatalf("groups: expected 2, got %d", len(resp.Groups))
	}

	// Sort order: reclaimable DESC, sha256 ASC.
	// Both groups have reclaimable 2000; "aaa" < "ccc".
	first := resp.Groups[0]
	if first.SHA256 != "aaa" || first.StorageID != "s1" {
		t.Errorf("first group: expected aaa/s1, got %s/%s", first.SHA256, first.StorageID)
	}
	if first.PathCount != 3 || first.PhysicalCopyCount != 3 {
		t.Errorf("aaa group: path_count=%d, physical_copy_count=%d", first.PathCount, first.PhysicalCopyCount)
	}
	if first.PhysicalReclaimableBytes != 2000 {
		t.Errorf("aaa reclaimable: expected 2000, got %d", first.PhysicalReclaimableBytes)
	}
}

func TestListDuplicateGroupsByStorage(t *testing.T) {
	path := createProjectDBWithDuplicates(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}

	resp, err := api.ListDuplicateGroups(ListGroupsRequest{StorageID: "s2"})
	if err != nil {
		t.Fatalf("ListDuplicateGroups: %v", err)
	}
	if resp.TotalCount != 1 {
		t.Fatalf("TotalCount: expected 1 for s2, got %d", resp.TotalCount)
	}
	if len(resp.Groups) != 1 {
		t.Fatalf("groups: expected 1, got %d", len(resp.Groups))
	}
	g := resp.Groups[0]
	if g.SHA256 != "ccc" || g.StorageID != "s2" {
		t.Errorf("group: expected ccc/s2, got %s/%s", g.SHA256, g.StorageID)
	}
}

func TestListDuplicateGroupsPagination(t *testing.T) {
	path := createProjectDBWithDuplicates(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}

	// Page 1: 1 item
	page1, err := api.ListDuplicateGroups(ListGroupsRequest{PageSize: 1})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(page1.Groups) != 1 {
		t.Fatalf("page 1: expected 1 group, got %d", len(page1.Groups))
	}
	if page1.NextCursor == "" {
		t.Fatal("page 1: expected non-empty NextCursor")
	}
	if page1.TotalCount != 2 {
		t.Errorf("TotalCount: expected 2, got %d", page1.TotalCount)
	}

	// Page 2: use cursor from page 1
	page2, err := api.ListDuplicateGroups(ListGroupsRequest{PageSize: 1, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2.Groups) != 1 {
		t.Fatalf("page 2: expected 1 group, got %d", len(page2.Groups))
	}
	if page2.NextCursor != "" {
		t.Errorf("page 2: expected empty NextCursor, got %s", page2.NextCursor)
	}

	// Verify the two pages cover different groups
	if page1.Groups[0].SHA256 == page2.Groups[0].SHA256 {
		t.Error("page 1 and 2 returned the same group")
	}
}

func TestListDuplicateGroupsInvalidCursor(t *testing.T) {
	path := createProjectDBWithDuplicates(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}

	_, err := api.ListDuplicateGroups(ListGroupsRequest{Cursor: "not-valid-base64!!!"})
	if err == nil {
		t.Fatal("expected error for invalid cursor")
	}
	_, err = api.ListDuplicateGroups(ListGroupsRequest{Cursor: "e30="}) // {}
	if err == nil {
		t.Fatal("expected error for incomplete cursor")
	}
}

func TestListDuplicateGroupsRejectsInvalidLimits(t *testing.T) {
	path := createProjectDBWithDuplicates(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })
	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}

	for _, req := range []ListGroupsRequest{
		{PageSize: -1},
		{PageSize: 201},
		{MinReclaimableBytes: -1},
	} {
		if _, err := api.ListDuplicateGroups(req); err == nil {
			t.Fatalf("expected validation error for %#v", req)
		}
	}
}

func TestListDuplicateGroupsNoProjectOpen(t *testing.T) {
	api := NewAPI()
	_, err := api.ListDuplicateGroups(ListGroupsRequest{})
	if err != ErrNoProjectOpen {
		t.Errorf("expected ErrNoProjectOpen, got %v", err)
	}
}

func TestGetGroupDetail(t *testing.T) {
	path := createProjectDBWithDuplicates(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}

	detail, err := api.GetGroupDetail("s1", "aaa")
	if err != nil {
		t.Fatalf("GetGroupDetail: %v", err)
	}
	if len(detail.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(detail.Files))
	}
	// Verify summary fields are populated
	if detail.SHA256 != "aaa" || detail.StorageID != "s1" {
		t.Errorf("detail: expected aaa/s1, got %s/%s", detail.SHA256, detail.StorageID)
	}
	// Verify file paths
	paths := make(map[string]bool)
	for _, f := range detail.Files {
		paths[f.Path] = true
	}
	for _, expected := range []string{"/source1/a.txt", "/source1/b.txt", "/source1/c.txt"} {
		if !paths[expected] {
			t.Errorf("missing file: %s", expected)
		}
	}
}

func TestGetGroupDetailNotFound(t *testing.T) {
	path := createProjectDBWithDuplicates(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}

	_, err := api.GetGroupDetail("s1", "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent group")
	}
}

func TestGetGroupDetailNoProjectOpen(t *testing.T) {
	api := NewAPI()
	_, err := api.GetGroupDetail("s1", "aaa")
	if err != ErrNoProjectOpen {
		t.Errorf("expected ErrNoProjectOpen, got %v", err)
	}
}

func TestGetGroupDetailRequiresIdentifiers(t *testing.T) {
	path := createProjectDBWithDuplicates(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })
	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}

	if _, err := api.GetGroupDetail("", "aaa"); err == nil {
		t.Fatal("expected error for empty storage ID")
	}
	if _, err := api.GetGroupDetail("s1", ""); err == nil {
		t.Fatal("expected error for empty SHA-256")
	}
}
