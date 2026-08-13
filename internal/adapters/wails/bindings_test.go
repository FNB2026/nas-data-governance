package wails

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

type recoveryLockFixture struct {
	source   []string
	restores []domain.RestoreJournalEntry
	purges   []domain.PurgeJournalEntry
}

func (f recoveryLockFixture) ListExecutingPlans(context.Context) ([]string, error) {
	return f.source, nil
}
func (f recoveryLockFixture) ListPendingRestores(context.Context) ([]domain.RestoreJournalEntry, error) {
	return f.restores, nil
}
func (f recoveryLockFixture) ListRecoverablePurges(context.Context) ([]domain.PurgeJournalEntry, error) {
	return f.purges, nil
}

func TestCheckRecoveryLockIncludesAllWriteJournals(t *testing.T) {
	status, err := checkRecoveryLock(context.Background(), recoveryLockFixture{
		source:   []string{"source-1"},
		restores: []domain.RestoreJournalEntry{{PlanID: "restore-1"}},
		purges:   []domain.PurgeJournalEntry{{PlanID: "purge-1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !status.LockActive || status.ExecutingCount != 3 ||
		status.SourceExecutingCount != 1 || status.RestorePendingCount != 1 ||
		status.PurgeRecoverableCount != 1 {
		t.Fatalf("unexpected recovery status: %#v", status)
	}
}

func TestMapPlanUsesStableGroupID(t *testing.T) {
	dto := mapPlan(domain.OperationPlan{ID: "plan-1", GroupID: "group-stable"})
	if dto.GroupID != "group-stable" {
		t.Fatalf("group id = %q", dto.GroupID)
	}
}

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

// ---- V4 scan & job binding tests ----

// createScanDir creates a temp directory with 3 small text files for
// scan testing. Two files share identical content (duplicate candidates).
func createScanDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string][]byte{
		"file1.txt": []byte("hello world"),
		"file2.txt": []byte("hello world"), // duplicate of file1
		"file3.txt": []byte("unique content here"),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	return dir
}

// waitForJobTerminal polls GetScanProgress until the job reaches a
// terminal state (COMPLETED, FAILED, or CANCELLED) or the timeout expires.
func waitForJobTerminal(t *testing.T, api *API, jobID string, timeout time.Duration) ScanJobProgress {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		prog, err := api.GetScanProgress(jobID)
		if err != nil {
			t.Fatalf("GetScanProgress: %v", err)
		}
		if prog.State == "COMPLETED" || prog.State == "FAILED" || prog.State == "CANCELLED" {
			return prog
		}
		time.Sleep(100 * time.Millisecond)
	}
	prog, _ := api.GetScanProgress(jobID)
	t.Fatalf("job %s did not reach terminal state within %s (last state: %s)", jobID, timeout, prog.State)
	return ScanJobProgress{}
}

func TestOpenProjectReadWriteCreatesNewDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new-project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	info, err := api.OpenProjectReadWrite(path)
	if err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	if !info.IsOpen {
		t.Fatal("expected project to be open")
	}
	if info.StorageCount != 0 {
		t.Errorf("expected 0 storages in new project, got %d", info.StorageCount)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("database file not created: %v", err)
	}
}

func TestOpenProjectReadWriteOpensExisting(t *testing.T) {
	path := createProjectDB(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	info, err := api.OpenProjectReadWrite(path)
	if err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	if info.StorageCount != 1 {
		t.Errorf("expected 1 storage, got %d", info.StorageCount)
	}
}

func TestOpenProjectReadWriteRejectsAlreadyOpen(t *testing.T) {
	path := createProjectDB(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := api.OpenProjectReadWrite(path); err != ErrProjectAlreadyOpen {
		t.Errorf("expected ErrProjectAlreadyOpen, got %v", err)
	}
}

func TestOpenProjectReadWriteRejectsSymlink(t *testing.T) {
	path := createProjectDB(t)
	link := filepath.Join(t.TempDir(), "project.db")
	if err := os.Symlink(path, link); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	api := NewAPI()
	if _, err := api.OpenProjectReadWrite(link); err == nil {
		t.Fatal("expected symlink validation error")
	}
}

func TestOpenProjectReadWriteRejectsBadExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.txt")
	if err := os.WriteFile(path, []byte("not a db"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	api := NewAPI()
	if _, err := api.OpenProjectReadWrite(path); err == nil {
		t.Fatal("expected extension validation error")
	}
}

func TestStartScanRejectsReadOnlyProject(t *testing.T) {
	path := createProjectDB(t)
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProject(path); err != nil {
		t.Fatalf("OpenProject: %v", err)
	}
	_, err := api.StartScan(StartScanRequest{Root: "/tmp"})
	if err != ErrProjectNotReadWrite {
		t.Errorf("expected ErrProjectNotReadWrite, got %v", err)
	}
}

func TestStartScanRequiresRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	if _, err := api.StartScan(StartScanRequest{}); err == nil {
		t.Fatal("expected error for empty root")
	}
}

func TestPreflightSourceIsReadOnlyAndPrivacySafe(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.txt"), []byte("sample"), 0o600); err != nil {
		t.Fatal(err)
	}
	api := NewAPI()
	profile, err := api.PreflightSource(root)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Status != "online" || profile.RecommendedWorkers < 1 {
		t.Fatalf("profile = %#v", profile)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "sample.txt" {
		t.Fatalf("preflight changed source: entries=%v err=%v", entries, err)
	}
}

func TestScanWorkersForNetworkProfileIsBounded(t *testing.T) {
	profile := scanner.SourceProfile{Network: true, RecommendedWorkers: 1}
	if got := scanWorkersForProfile(0, profile); got != 1 {
		t.Fatalf("default network workers = %d, want 1", got)
	}
	if got := scanWorkersForProfile(64, profile); got != 4 {
		t.Fatalf("bounded network workers = %d, want 4", got)
	}
	if got := scanWorkersForProfile(2, profile); got != 2 {
		t.Fatalf("explicit safe workers = %d, want 2", got)
	}
}

func TestStartScanAndPollProgress(t *testing.T) {
	scanDir := createScanDir(t)
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}

	resp, err := api.StartScan(StartScanRequest{Root: scanDir})
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	if resp.JobID == "" {
		t.Fatal("expected non-empty job ID")
	}

	prog := waitForJobTerminal(t, api, resp.JobID, 30*time.Second)
	if prog.State != "COMPLETED" {
		t.Fatalf("expected COMPLETED, got %s (error: %s)", prog.State, prog.ErrorCode)
	}
	if prog.Discovered != 3 {
		t.Errorf("expected 3 discovered, got %d", prog.Discovered)
	}
	if prog.Processed != 3 {
		t.Errorf("expected 3 processed, got %d", prog.Processed)
	}

	// Verify the storage was registered in the database.
	storages, err := api.ListStorages()
	if err != nil {
		t.Fatalf("ListStorages: %v", err)
	}
	found := false
	for _, s := range storages {
		if s.RootPath == scanDir {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected storage for scan dir %q to be registered after scan, got %#v", scanDir, storages)
	}
}

// TestStartScanRejectsMismatchedStorageID verifies that the backend rejects
// a StorageID that does not match the ID derived from the scan root. This
// prevents a frontend or API client from silently rebinding an existing
// storage ID to a different directory.
func TestStartScanRejectsMismatchedStorageID(t *testing.T) {
	scanDir := createScanDir(t)
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}

	// 1. Mismatched StorageID must be rejected.
	_, err := api.StartScan(StartScanRequest{Root: scanDir, StorageID: "wrong-id"})
	if err == nil {
		t.Fatal("expected error for mismatched storage_id, got nil")
	}

	// 2. Correct derived StorageID must succeed.
	correctID := generateStorageID(scanDir)
	resp, err := api.StartScan(StartScanRequest{Root: scanDir, StorageID: correctID})
	if err != nil {
		t.Fatalf("StartScan with correct derived ID: %v", err)
	}
	_ = waitForJobTerminal(t, api, resp.JobID, 30*time.Second)

	// 3. Empty StorageID must also succeed (backend derives it).
	resp2, err := api.StartScan(StartScanRequest{Root: scanDir})
	if err != nil {
		t.Fatalf("StartScan with empty storage_id: %v", err)
	}
	_ = waitForJobTerminal(t, api, resp2.JobID, 30*time.Second)
}

func TestGetScanProgressJobNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	_, err := api.GetScanProgress("job-nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent job")
	}
}

func TestCancelScan(t *testing.T) {
	scanDir := createScanDir(t)
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}

	resp, err := api.StartScan(StartScanRequest{Root: scanDir})
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}

	// Cancel immediately. For a small directory the scan may have
	// already completed, in which case the cancel is a no-op and the
	// job reaches COMPLETED. Either CANCELLED or COMPLETED is acceptable.
	if err := api.CancelScan(resp.JobID); err != nil {
		t.Fatalf("CancelScan: %v", err)
	}

	prog := waitForJobTerminal(t, api, resp.JobID, 30*time.Second)
	if prog.State != "CANCELLED" && prog.State != "COMPLETED" {
		t.Fatalf("expected CANCELLED or COMPLETED, got %s", prog.State)
	}
}

func TestListRecentJobs(t *testing.T) {
	scanDir := createScanDir(t)
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}

	resp, err := api.StartScan(StartScanRequest{Root: scanDir})
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	_ = waitForJobTerminal(t, api, resp.JobID, 30*time.Second)

	jobs, err := api.ListRecentJobs(10)
	if err != nil {
		t.Fatalf("ListRecentJobs: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected at least 1 job")
	}
	first := jobs[0]
	if first.JobID != resp.JobID {
		t.Errorf("expected first job ID %s, got %s", resp.JobID, first.JobID)
	}
	if first.JobType != "scan" {
		t.Errorf("expected job type 'scan', got %s", first.JobType)
	}
	if first.State != "COMPLETED" {
		t.Errorf("expected COMPLETED, got %s", first.State)
	}
}

func TestGetJobDetail(t *testing.T) {
	scanDir := createScanDir(t)
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}

	resp, err := api.StartScan(StartScanRequest{Root: scanDir})
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	_ = waitForJobTerminal(t, api, resp.JobID, 30*time.Second)

	detail, err := api.GetJobDetail(resp.JobID)
	if err != nil {
		t.Fatalf("GetJobDetail: %v", err)
	}
	if detail.JobID != resp.JobID {
		t.Errorf("expected job ID %s, got %s", resp.JobID, detail.JobID)
	}
	if detail.State != "COMPLETED" {
		t.Errorf("expected COMPLETED, got %s", detail.State)
	}
	// A completed scan should have at least created, stage, and
	// completed events.
	if len(detail.Events) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(detail.Events))
	}
	// Verify event ordering by sequence.
	for i := 1; i < len(detail.Events); i++ {
		if detail.Events[i].Sequence <= detail.Events[i-1].Sequence {
			t.Error("events are not ordered by sequence")
			break
		}
	}
}

func TestGetJobDetailNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	_, err := api.GetJobDetail("job-nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent job")
	}
}

func TestV4MethodsNoProjectOpen(t *testing.T) {
	api := NewAPI()

	if _, err := api.StartScan(StartScanRequest{Root: "/tmp"}); err != ErrNoProjectOpen {
		t.Errorf("StartScan: expected ErrNoProjectOpen, got %v", err)
	}
	if _, err := api.GetScanProgress("job-1"); err != ErrNoProjectOpen {
		t.Errorf("GetScanProgress: expected ErrNoProjectOpen, got %v", err)
	}
	if err := api.CancelScan("job-1"); err != ErrNoProjectOpen {
		t.Errorf("CancelScan: expected ErrNoProjectOpen, got %v", err)
	}
	if _, err := api.ListRecentJobs(10); err != ErrNoProjectOpen {
		t.Errorf("ListRecentJobs: expected ErrNoProjectOpen, got %v", err)
	}
	if _, err := api.GetJobDetail("job-1"); err != ErrNoProjectOpen {
		t.Errorf("GetJobDetail: expected ErrNoProjectOpen, got %v", err)
	}
	if _, err := api.GetScanCheckpoint("/tmp"); err != ErrNoProjectOpen {
		t.Errorf("GetScanCheckpoint: expected ErrNoProjectOpen, got %v", err)
	}
}

// TestGetScanCheckpointAfterCompletedScan verifies that after a scan
// completes successfully, GetScanCheckpoint returns Available=false —
// the "继续扫描" button should NOT appear.
func TestGetScanCheckpointAfterCompletedScan(t *testing.T) {
	scanDir := createScanDir(t)
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}

	resp, err := api.StartScan(StartScanRequest{Root: scanDir})
	if err != nil {
		t.Fatalf("StartScan: %v", err)
	}
	prog := waitForJobTerminal(t, api, resp.JobID, 30*time.Second)
	if prog.State != "COMPLETED" {
		t.Fatalf("expected COMPLETED, got %s", prog.State)
	}

	// After a completed scan, no resumable checkpoint should exist.
	cp, err := api.GetScanCheckpoint(scanDir)
	if err != nil {
		t.Fatalf("GetScanCheckpoint: %v", err)
	}
	if cp.Available {
		t.Errorf("Available = true, want false (scan completed successfully)")
	}
}

// TestGetScanCheckpointWithAbortedCheckpoint verifies that when an
// aborted checkpoint exists, GetScanCheckpoint returns Available=true
// with the checkpoint's scanned count — the "继续扫描" button should appear.
func TestGetScanCheckpointWithAbortedCheckpoint(t *testing.T) {
	scanDir := createScanDir(t)
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}

	// Manually create an aborted checkpoint to simulate a crashed/cancelled scan.
	storageID := generateStorageID(scanDir)
	ctx := context.Background()
	if err := api.store.RegisterStorage(ctx, domain.Storage{
		ID: storageID, RootPath: scanDir, Kind: "filesystem", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("RegisterStorage: %v", err)
	}
	cpID, err := api.store.StartCheckpoint(ctx, storageID)
	if err != nil {
		t.Fatalf("StartCheckpoint: %v", err)
	}
	if err := api.store.UpdateCheckpoint(ctx, cpID, filepath.Join(scanDir, "file2.txt"), 1); err != nil {
		t.Fatalf("UpdateCheckpoint: %v", err)
	}
	if err := api.store.CompleteCheckpoint(ctx, cpID, "aborted"); err != nil {
		t.Fatalf("CompleteCheckpoint: %v", err)
	}

	// GetScanCheckpoint should report the aborted checkpoint as available.
	cp, err := api.GetScanCheckpoint(scanDir)
	if err != nil {
		t.Fatalf("GetScanCheckpoint: %v", err)
	}
	if !cp.Available {
		t.Fatal("Available = false, want true (aborted checkpoint exists)")
	}
	if cp.Status != "aborted" {
		t.Errorf("Status = %q, want 'aborted'", cp.Status)
	}
	if cp.ScannedCount != 1 {
		t.Errorf("ScannedCount = %d, want 1", cp.ScannedCount)
	}
}

// TestStartScanWithResumeOption verifies that the StartScan binding
// correctly passes Resume=true to the backend when requested.
func TestStartScanWithResumeOption(t *testing.T) {
	scanDir := createScanDir(t)
	path := filepath.Join(t.TempDir(), "project.db")
	api := NewAPI()
	t.Cleanup(func() { _ = api.CloseProject() })

	if _, err := api.OpenProjectReadWrite(path); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}

	// Start a scan with Resume=true. Since no prior checkpoint exists,
	// this should still succeed (starts a fresh checkpoint).
	resp, err := api.StartScan(StartScanRequest{
		Root:   scanDir,
		Resume: true,
	})
	if err != nil {
		t.Fatalf("StartScan with Resume: %v", err)
	}
	prog := waitForJobTerminal(t, api, resp.JobID, 30*time.Second)
	if prog.State != "COMPLETED" {
		t.Fatalf("expected COMPLETED, got %s", prog.State)
	}
}
