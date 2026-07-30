package wails

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain redirects the app-support root to a temp dir so the whole
// suite (including pre-existing OpenProject/OpenProjectReadWrite tests,
// which now record a recent-projects manifest) never writes to the real
// ~/Library/Application Support.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ndg-wails-test-")
	if err != nil {
		panic(err)
	}
	appSupportBaseFn = func() (string, error) { return dir, nil }
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func TestNormalizeNameID(t *testing.T) {
	cases := map[string]string{
		"Home NAS":         "home-nas",
		"  产业资料库  ":        "产业资料库",
		"Project__Two  ID": "project-two-id",
		"...!!!...":        "",
		"café-ørsted":      "café-ørsted",
	}
	for in, want := range cases {
		if got := normalizeNameID(in); got != want {
			t.Errorf("normalizeNameID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGenerateStorageID(t *testing.T) {
	// Same path must yield the same ID (idempotent upsert).
	id1 := generateStorageID("/Volumes/data")
	id2 := generateStorageID("/Volumes/data")
	if id1 != id2 {
		t.Errorf("generateStorageID not deterministic: %q vs %q", id1, id2)
	}
	// Different paths must yield different IDs.
	id3 := generateStorageID("/Volumes/other")
	if id1 == id3 {
		t.Errorf("generateStorageID collision for different paths")
	}
	// Must have the src_ prefix.
	if !strings.HasPrefix(id1, "src_") {
		t.Errorf("generateStorageID = %q, want src_ prefix", id1)
	}
	// Path normalization: trailing slash must not change the ID.
	id4 := generateStorageID("/Volumes/data/")
	if id1 != id4 {
		t.Errorf("generateStorageID changed after trailing slash: %q vs %q", id1, id4)
	}
}

func TestIsWithin(t *testing.T) {
	cases := []struct {
		path, parent string
		want         bool
	}{
		{"/a/b", "/a", true},
		{"/a/b/c", "/a", true},
		{"/a", "/a", true},
		{"/ab", "/a", false},
		{"/a/bb", "/a/b", false},
		{"/x", "/a", false},
	}
	for _, c := range cases {
		if got := isWithin(c.path, c.parent); got != c.want {
			t.Errorf("isWithin(%q, %q) = %v, want %v", c.path, c.parent, got, c.want)
		}
	}
}

// makeScanSource creates a temp dir to act as a scan source for tests.
func makeScanSource(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ndg-scan-src-")
	if err != nil {
		t.Fatalf("create scan source: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestCreateProjectFromSource(t *testing.T) {
	api := NewAPI()
	src := makeScanSource(t)

	info, err := api.CreateProjectFromSource(CreateProjectInput{
		Name:       "Home NAS",
		SourcePath: src,
	})
	if err != nil {
		t.Fatalf("CreateProjectFromSource: %v", err)
	}
	if !info.IsOpen {
		t.Error("info.IsOpen = false, want true")
	}
	if info.Path == "" || !strings.HasSuffix(info.Path, "/governance.db") {
		t.Errorf("info.Path = %q, want .../governance.db", info.Path)
	}
	if info.ProjectID == "" || info.ProjectID == info.Path {
		t.Errorf("info.ProjectID = %q, must be a non-path logical ID distinct from %q", info.ProjectID, info.Path)
	}
	if info.Name != "Home NAS" {
		t.Errorf("info.Name = %q, want %q", info.Name, "Home NAS")
	}
	// One storage (the scan source) must be registered.
	if info.StorageCount != 1 {
		t.Errorf("info.StorageCount = %d, want 1", info.StorageCount)
	}

	// Database file must be owner-only (0600) and its project dir 0700.
	fi, err := os.Stat(info.Path)
	if err != nil {
		t.Fatalf("stat db: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("db perms = %o, want 600", got)
	}
	dirInfo, err := os.Stat(filepath.Dir(info.Path))
	if err != nil {
		t.Fatalf("stat project dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("project dir perms = %o, want 700", got)
	}

	// project.json must exist and carry the display name.
	meta, ok := readProjectMeta(filepath.Dir(info.Path))
	if !ok {
		t.Fatal("project.json not found")
	}
	if meta.Name != "Home NAS" {
		t.Errorf("project.json name = %q, want %q", meta.Name, "Home NAS")
	}
	if meta.Database != "governance.db" {
		t.Errorf("project.json database = %q, want %q", meta.Database, "governance.db")
	}
	if info.ProjectID != meta.ProjectID {
		t.Errorf("info.ProjectID = %q, project.json ProjectID = %q", info.ProjectID, meta.ProjectID)
	}

	// The registered storage must use a src_ ID, not "default".
	list, err := api.ListStorages()
	if err != nil {
		t.Fatalf("ListStorages: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("storages = %d, want 1", len(list))
	}
	if !strings.HasPrefix(list[0].ID, "src_") {
		t.Errorf("storage ID = %q, want src_ prefix", list[0].ID)
	}

	// Recent manifest must record this project as the most recent entry.
	recent, err := api.ListRecentProjects()
	if err != nil {
		t.Fatalf("ListRecentProjects: %v", err)
	}
	if len(recent) == 0 || recent[0].Path != info.Path {
		t.Fatalf("recent[0] = %+v, want path %q", recent, info.Path)
	}
	if recent[0].Name != "Home NAS" {
		t.Errorf("recent[0].Name = %q, want %q", recent[0].Name, "Home NAS")
	}

	if err := api.CloseProject(); err != nil {
		t.Fatalf("CloseProject: %v", err)
	}
}

func TestCreateProjectFromSourceEmptyNameUsesSourceDirectory(t *testing.T) {
	api := NewAPI()
	parent := t.TempDir()
	src := filepath.Join(parent, "产业资料库")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}

	info, err := api.CreateProjectFromSource(CreateProjectInput{SourcePath: src})
	if err != nil {
		t.Fatalf("CreateProjectFromSource: %v", err)
	}
	defer func() { _ = api.CloseProject() }()
	if info.Name != "产业资料库" {
		t.Errorf("info.Name = %q, want source directory name", info.Name)
	}
	if info.ProjectID != "产业资料库" {
		t.Errorf("info.ProjectID = %q, want source directory name", info.ProjectID)
	}
	meta, ok := readProjectMeta(filepath.Dir(info.Path))
	if !ok || meta.Name != "产业资料库" || meta.ProjectID != info.ProjectID {
		t.Errorf("project meta = %+v, ok=%v; want source-derived name/id", meta, ok)
	}
}

func TestCreateProjectFromSourceCollisionSuffix(t *testing.T) {
	api := NewAPI()
	src1 := makeScanSource(t)
	src2 := makeScanSource(t)

	first, err := api.CreateProjectFromSource(CreateProjectInput{
		Name:       "产业资料库",
		SourcePath: src1,
	})
	if err != nil {
		t.Fatalf("first CreateProjectFromSource: %v", err)
	}
	if err := api.CloseProject(); err != nil {
		t.Fatalf("first CloseProject: %v", err)
	}
	second, err := api.CreateProjectFromSource(CreateProjectInput{
		Name:       "产业资料库",
		SourcePath: src2,
	})
	if err != nil {
		t.Fatalf("second CreateProjectFromSource: %v", err)
	}
	firstDir := filepath.Base(filepath.Dir(first.Path))
	secondDir := filepath.Base(filepath.Dir(second.Path))
	if firstDir == secondDir {
		t.Fatalf("collision: both projects share dir %q", firstDir)
	}
	if !strings.HasPrefix(firstDir, "产业资料库") || !strings.HasPrefix(secondDir, "产业资料库-") {
		t.Errorf("collision dirs = %q and %q, want source-name base with suffix", firstDir, secondDir)
	}
	if err := api.CloseProject(); err != nil {
		t.Fatalf("second CloseProject: %v", err)
	}
}

func TestRegisterScanSourceRequiresOpenProject(t *testing.T) {
	api := NewAPI()
	if _, err := api.RegisterScanSource("/x"); err != ErrNoProjectOpen {
		t.Errorf("RegisterScanSource without project: err = %v, want ErrNoProjectOpen", err)
	}
}

func TestRegisterScanSourceRejectsDuplicates(t *testing.T) {
	api := NewAPI()
	src := makeScanSource(t)
	if _, err := api.CreateProjectFromSource(CreateProjectInput{
		Name:       "Dup Test",
		SourcePath: src,
	}); err != nil {
		t.Fatalf("CreateProjectFromSource: %v", err)
	}
	defer func() { _ = api.CloseProject() }()

	// The scan source was already registered during project creation.
	if _, err := api.RegisterScanSource(src); err == nil {
		t.Error("RegisterScanSource with duplicate root: err = nil, want error")
	}
}

func TestRegisterScanSourceRejectsOverlap(t *testing.T) {
	api := NewAPI()
	src := makeScanSource(t)
	if _, err := api.CreateProjectFromSource(CreateProjectInput{
		Name:       "Overlap Test",
		SourcePath: src,
	}); err != nil {
		t.Fatalf("CreateProjectFromSource: %v", err)
	}
	defer func() { _ = api.CloseProject() }()

	// A subdirectory of the registered source must be rejected.
	sub := filepath.Join(src, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if _, err := api.RegisterScanSource(sub); err == nil {
		t.Error("RegisterScanSource with nested root: err = nil, want error")
	}
}

func TestRegisterScanSourceRejectsRootContainingCurrentDatabase(t *testing.T) {
	api := NewAPI()
	projectDir := t.TempDir()
	dbPath := filepath.Join(projectDir, "governance.db")
	if _, err := api.OpenProjectReadWrite(dbPath); err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	defer func() { _ = api.CloseProject() }()

	if _, err := api.RegisterScanSource(projectDir); err == nil || !strings.Contains(err.Error(), "WAL/SHM") {
		t.Fatalf("RegisterScanSource root containing db: err = %v, want project database/WAL/SHM rejection", err)
	}
	if _, err := api.StartScan(StartScanRequest{Root: projectDir, Workers: 1}); err == nil || !strings.Contains(err.Error(), "WAL/SHM") {
		t.Fatalf("StartScan root containing db: err = %v, want project database/WAL/SHM rejection", err)
	}
}

func TestLegacyProjectInfoUsesLogicalIDButKeepsHistoricalJobScope(t *testing.T) {
	api := NewAPI()
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	info, err := api.OpenProjectReadWrite(dbPath)
	if err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	defer func() { _ = api.CloseProject() }()
	absPath, _ := filepath.Abs(dbPath)
	if info.ProjectID == "" || info.ProjectID == info.Path || !strings.HasPrefix(info.ProjectID, "legacy_") {
		t.Errorf("legacy ProjectInfo = %+v, want non-path legacy logical ID", info)
	}
	if api.jobScope != absPath {
		t.Errorf("legacy jobScope = %q, want historical abs path %q", api.jobScope, absPath)
	}
}

func TestValidateScanSourceRejectsAppSupport(t *testing.T) {
	api := NewAPI()
	ndgRoot, _ := appSupportBase()
	if _, err := api.CreateProjectFromSource(CreateProjectInput{
		Name:       "Bad Source",
		SourcePath: ndgRoot,
	}); err == nil {
		t.Error("CreateProjectFromSource with NDG app-support dir: err = nil, want error")
		_ = api.CloseProject()
	}
}

func TestValidateScanSourceRejectsSymlink(t *testing.T) {
	api := NewAPI()
	real := makeScanSource(t)
	link := real + "-link"
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	defer func() { _ = os.Remove(link) }()

	if _, err := api.CreateProjectFromSource(CreateProjectInput{
		Name:       "Symlink Source",
		SourcePath: link,
	}); err == nil {
		t.Error("CreateProjectFromSource with symlink root: err = nil, want error")
		_ = api.CloseProject()
	}
}

func TestValidateScanSourceRejectsNonExistent(t *testing.T) {
	api := NewAPI()
	if _, err := api.CreateProjectFromSource(CreateProjectInput{
		Name:       "Missing Source",
		SourcePath: "/this/does/not/exist",
	}); err == nil {
		t.Error("CreateProjectFromSource with non-existent root: err = nil, want error")
		_ = api.CloseProject()
	}
}

func TestValidateScanSourceRejectsDirectoryWithoutPermissions(t *testing.T) {
	api := NewAPI()
	src := makeScanSource(t)
	if err := os.Chmod(src, 0); err != nil {
		t.Fatalf("chmod source: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(src, 0o700) })

	if _, err := api.CreateProjectFromSource(CreateProjectInput{
		Name:       "No Permission",
		SourcePath: src,
	}); err == nil || !strings.Contains(err.Error(), "not readable") {
		t.Fatalf("CreateProjectFromSource without permissions: err = %v, want readable/searchable rejection", err)
	}
}

func TestCreateProjectFromSourceRollsBackAfterDatabaseCreationFailure(t *testing.T) {
	api := NewAPI()
	src := makeScanSource(t)
	original := writeProjectMetaFn
	writeProjectMetaFn = func(string, ProjectMeta) error {
		return errors.New("injected project metadata failure")
	}
	t.Cleanup(func() { writeProjectMetaFn = original })

	_, err := api.CreateProjectFromSource(CreateProjectInput{
		Name:       "Rollback Probe",
		SourcePath: src,
	})
	if err == nil || !strings.Contains(err.Error(), "injected project metadata failure") {
		t.Fatalf("CreateProjectFromSource: err = %v, want injected failure", err)
	}
	base, baseErr := appSupportBase()
	if baseErr != nil {
		t.Fatalf("appSupportBase: %v", baseErr)
	}
	projectDir := filepath.Join(base, projectsSubDir, "rollback-probe")
	if _, statErr := os.Lstat(projectDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rollback project dir still exists: stat err = %v", statErr)
	}
	if _, infoErr := api.GetProjectInfo(); infoErr != ErrNoProjectOpen {
		t.Fatalf("GetProjectInfo after rollback: err = %v, want ErrNoProjectOpen", infoErr)
	}
}

func TestRecentDedupOnReopen(t *testing.T) {
	api := NewAPI()
	src := makeScanSource(t)
	info, err := api.CreateProjectFromSource(CreateProjectInput{
		Name:       "Dedup Project",
		SourcePath: src,
	})
	if err != nil {
		t.Fatalf("CreateProjectFromSource: %v", err)
	}
	dbPath := info.Path
	if err := api.CloseProject(); err != nil {
		t.Fatalf("first close: %v", err)
	}

	// Reopen the same database read-write; the recent list must not grow
	// a duplicate entry for this path.
	reopened, err := api.OpenProjectReadWrite(dbPath)
	if err != nil {
		t.Fatalf("OpenProjectReadWrite: %v", err)
	}
	if reopened.ProjectID != info.ProjectID || reopened.ProjectID == reopened.Path {
		t.Errorf("reopened identity = %q, initial = %q, path = %q", reopened.ProjectID, info.ProjectID, reopened.Path)
	}
	recent, err := api.ListRecentProjects()
	if err != nil {
		t.Fatalf("ListRecentProjects: %v", err)
	}
	count := 0
	for _, e := range recent {
		if e.Path == dbPath {
			count++
		}
	}
	if count != 1 {
		t.Errorf("recent entries for %q = %d, want 1 (dedup)", dbPath, count)
	}
	if len(recent) == 0 || recent[0].Path != dbPath {
		t.Errorf("recent[0].Path = %q, want %q (moved to front)", recent[0].Path, dbPath)
	}
	// Name must come from project.json, not the directory name.
	if recent[0].Name != "Dedup Project" {
		t.Errorf("recent[0].Name = %q, want %q (from project.json)", recent[0].Name, "Dedup Project")
	}
	_ = api.CloseProject()
}

func TestRecentManifestCorruptTolerance(t *testing.T) {
	p, err := recentPath()
	if err != nil {
		t.Fatalf("recentPath: %v", err)
	}
	// Write garbage into recent.json.
	if err := os.WriteFile(p, []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write corrupt manifest: %v", err)
	}
	// readRecentManifest must not error and must return an empty slice.
	entries, err := readRecentManifest()
	if err != nil {
		t.Fatalf("readRecentManifest on corrupt: %v", err)
	}
	if entries != nil && len(entries) != 0 {
		t.Errorf("entries = %v, want empty", entries)
	}
	// The corrupt file must have been moved aside.
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("corrupt recent.json still present after read")
	}
}

func TestRecentManifestAtomicWrite(t *testing.T) {
	entries := []RecentProjectEntry{
		{Name: "A", Path: "/a.db", OpenedAt: time.Now().UTC()},
	}
	if err := writeRecentManifest(entries); err != nil {
		t.Fatalf("writeRecentManifest: %v", err)
	}
	p, _ := recentPath()
	// No .tmp file must linger after a successful write.
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("temp file lingered after atomic write")
	}
	// Content must be valid JSON.
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var got []RecentProjectEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 1 || got[0].Path != "/a.db" {
		t.Errorf("got = %+v, want one entry /a.db", got)
	}
}

func TestProjectMetaSurvivesLostRecent(t *testing.T) {
	api := NewAPI()
	src := makeScanSource(t)
	info, err := api.CreateProjectFromSource(CreateProjectInput{
		Name:       "Survivor",
		SourcePath: src,
	})
	if err != nil {
		t.Fatalf("CreateProjectFromSource: %v", err)
	}
	dbPath := info.Path
	if err := api.CloseProject(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Simulate a lost recent.json.
	p, _ := recentPath()
	_ = os.Remove(p)

	// projectDisplayName must still recover the name from project.json.
	name := projectDisplayName(dbPath)
	if name != "Survivor" {
		t.Errorf("projectDisplayName = %q, want %q (from project.json)", name, "Survivor")
	}
}

func TestPickDirectoryWithoutPicker(t *testing.T) {
	api := NewAPI() // no SetDirectoryPicker call in tests
	if _, err := api.PickDirectory(""); err == nil {
		t.Error("PickDirectory without picker: err = nil, want error")
	}
}
