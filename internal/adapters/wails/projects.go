package wails

// First-launch project lifecycle bindings (V9).
//
// New users should not have to understand database files, extensions, or
// absolute paths. CreateProjectFromSource takes a scan source directory
// (chosen via the native directory picker) and an optional project name,
// then atomically:
//   1. validates the scan source,
//   2. creates a project dir + governance.db under the OS app-support
//      dir (~/Library/Application Support/NDG/projects/<id>/ on macOS),
//   3. registers the scan source as a storage with a backend-generated ID,
//   4. writes project.json metadata alongside the database,
//   5. records the project in recent.json.
//
// On any failure after the database is created, the project directory is
// removed so no half-created project lingers. The scan source directory
// is never written to by NDG; the project database lives under the
// app-support dir, keeping WAL on a local filesystem and preventing the
// scan from descending into NDG's own artifacts.
//
// Per ADR-0006 this package must not import Wails framework packages, so
// the native directory dialog is injected via DirectoryPicker (main.go
// supplies the implementation that calls runtime.OpenDirectoryDialog).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
	"github.com/FNB2026/nas-data-governance/internal/privatefs"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

const (
	appSupportDirName    = "NDG" // created under os.UserConfigDir()
	projectsSubDir       = "projects"
	dbFileName           = "governance.db"
	projectMetaFileName  = "project.json"
	recentFileName       = "recent.json"
	recentMaxEntries     = 20
	projectMetaSchemaVer = 1
)

var writeProjectMetaFn = writeProjectMeta

// DirectoryPicker opens a native directory-selection dialog and returns
// the chosen path. It returns "" with a nil error when the user cancels.
// The implementation lives in cmd/ndg-desktop (which may import the Wails
// runtime); the adapter layer stays free of framework imports.
type DirectoryPicker func(title string) (string, error)

// RecentProjectEntry is one item in the recent-projects manifest stored
// at <app-support>/NDG/recent.json.
type RecentProjectEntry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	OpenedAt time.Time `json:"opened_at"`
}

// CreateProjectInput is the request DTO for CreateProjectFromSource.
// Name is an optional human-readable project name. SourcePath is the
// absolute path to the scan source directory (must exist and be readable).
type CreateProjectInput struct {
	Name       string `json:"name"`
	SourcePath string `json:"source_path"`
}

// ProjectMeta is the on-disk project metadata stored as project.json
// alongside governance.db. It lets the app recover the display name and
// project ID even when recent.json is lost.
type ProjectMeta struct {
	SchemaVersion int       `json:"schemaVersion"`
	ProjectID     string    `json:"projectId"`
	Name          string    `json:"name"`
	Database      string    `json:"database"`
	CreatedAt     time.Time `json:"createdAt"`
}

// SetDirectoryPicker injects the native directory picker. main.go calls
// this in OnStartup with a closure over the Wails runtime context. It is
// safe to call before the app window is ready; the picker is only invoked
// when the frontend calls PickDirectory.
func (a *API) SetDirectoryPicker(picker DirectoryPicker) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dirPicker = picker
}

// PickDirectory opens a native directory picker. Returns "" when the user
// cancels. The picker is optional; outside the Wails runtime (e.g. in
// tests) it returns an error instead of blocking.
func (a *API) PickDirectory(title string) (string, error) {
	a.mu.RLock()
	picker := a.dirPicker
	a.mu.RUnlock()
	if picker == nil {
		return "", errors.New("wails: directory picker is not configured")
	}
	if strings.TrimSpace(title) == "" {
		title = "选择目录"
	}
	return picker(title)
}

// CreateProjectFromSource atomically creates a new project from a scan
// source directory. It validates the source, creates the project database
// under the OS app-support dir, registers the source as a storage with a
// backend-generated ID, writes project.json, and records the project in
// recent.json.
//
// On any failure after the database is created, the project directory is
// removed (rollback) so no half-created project lingers. The scan source
// itself is never modified.
//
// If a project is already open, it returns ErrProjectAlreadyOpen.
func (a *API) CreateProjectFromSource(input CreateProjectInput) (ProjectInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.store != nil {
		return ProjectInfo{}, ErrProjectAlreadyOpen
	}

	// 1. Validate the scan source BEFORE creating anything. This refuses
	//    NDG's own app-support dir, the current project dir, symlinks,
	//    non-existent paths, and duplicate/overlapping storages.
	sourceRoot, err := validateScanSource(strings.TrimSpace(input.SourcePath), nil, "")
	if err != nil {
		return ProjectInfo{}, err
	}

	// 2. Prepare the project directory under the app-support root.
	base, err := appSupportBase()
	if err != nil {
		return ProjectInfo{}, err
	}
	if err := privatefs.SecureDirectory(base); err != nil {
		return ProjectInfo{}, fmt.Errorf("wails: create app support dir: %w", err)
	}
	projectsRoot := filepath.Join(base, projectsSubDir)
	if err := privatefs.SecureDirectory(projectsRoot); err != nil {
		return ProjectInfo{}, fmt.Errorf("wails: create projects dir: %w", err)
	}

	displayName := strings.TrimSpace(input.Name)
	if displayName == "" {
		displayName = filepath.Base(filepath.Clean(sourceRoot))
		if displayName == "." || displayName == string(filepath.Separator) {
			displayName = ""
		}
	}
	id := uniqueProjectID(projectsRoot, displayName)
	projectDir := filepath.Join(projectsRoot, id)
	dbPath := filepath.Join(projectDir, dbFileName)

	// 3. Open the database (store.Open applies migrations and secures the
	//    db file + WAL/SHM sidecars to 0600, and the project dir to 0700).
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("wails: create project database: %w", err)
	}

	// From here on, any failure triggers rollback: close the db, reset
	// state, and remove the freshly-created project directory.
	rollback := func(cause error) (ProjectInfo, error) {
		_ = st.Close()
		a.resetProjectStateLocked()
		_ = removeFreshProjectDir(projectDir, projectsRoot)
		return ProjectInfo{}, cause
	}

	a.initReadWriteServicesLocked(st)
	a.path = dbPath
	a.projectID = id
	a.jobScope = id

	// 4. Register the scan source with a backend-generated stable ID.
	storageID := generateStorageID(sourceRoot)
	storage := domain.Storage{
		ID:        storageID,
		RootPath:  sourceRoot,
		Kind:      "filesystem",
		CreatedAt: time.Now().UTC(),
	}
	if err := a.store.RegisterStorage(context.Background(), storage); err != nil {
		return rollback(fmt.Errorf("wails: register scan source: %w", err))
	}

	// 5. Write project.json metadata so the name survives a lost recent.json.
	if displayName == "" {
		displayName = id
	}
	meta := ProjectMeta{
		SchemaVersion: projectMetaSchemaVer,
		ProjectID:     id,
		Name:          displayName,
		Database:      dbFileName,
		CreatedAt:     time.Now().UTC(),
	}
	if err := writeProjectMetaFn(projectDir, meta); err != nil {
		return rollback(err)
	}

	// 6. Build ProjectInfo (reads storage count from the open store).
	info, err := a.projectInfoLocked(context.Background())
	if err != nil {
		return rollback(err)
	}

	// 7. Record in recent.json (best-effort; does not roll back).
	_ = a.recordRecentLocked(displayName, dbPath)
	return info, nil
}

// RegisterScanSource registers an additional data source (scan root) in
// the currently open project. The storage ID is generated by the backend
// from the normalized path; callers cannot override it. The root must
// pass validateScanSource (exists, is a directory, not a symlink, not
// inside NDG's app-support dir, not the project database, and not a
// duplicate of or overlapping an existing storage).
//
// The project must be open in read-write mode.
func (a *API) RegisterScanSource(root string) (StorageInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.store == nil {
		return StorageInfo{}, ErrNoProjectOpen
	}

	existing, err := a.store.ListStorages(context.Background())
	if err != nil {
		return StorageInfo{}, fmt.Errorf("wails: list existing storages: %w", err)
	}

	resolved, err := validateScanSource(strings.TrimSpace(root), existing, a.path)
	if err != nil {
		return StorageInfo{}, err
	}

	id := generateStorageID(resolved)
	s := domain.Storage{
		ID:        id,
		RootPath:  resolved,
		Kind:      "filesystem",
		CreatedAt: time.Now().UTC(),
	}
	if err := a.store.RegisterStorage(context.Background(), s); err != nil {
		return StorageInfo{}, fmt.Errorf("wails: register scan source: %w", err)
	}
	return mapStorages([]domain.Storage{s})[0], nil
}

// ListRecentProjects returns the most recently opened projects, most
// recent first, read from <app-support>/NDG/recent.json. Returns an empty
// slice when no manifest exists yet or the manifest is corrupt (in which
// case the corrupt file is preserved aside for diagnosis). It does not
// require an open project.
func (a *API) ListRecentProjects() ([]RecentProjectEntry, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return readRecentManifest()
}

// ---- scan source validation ----

// validateScanSource checks that root is a legitimate read-only scan
// target. It returns the resolved absolute path (with intermediate
// symlinks evaluated) or a descriptive error.
//
// Checks (per review feedback):
//   - non-empty
//   - exists and is a directory
//   - not a symbolic link at the root (per AGENTS guardrail: do not
//     follow symlinks)
//   - readable
//   - not inside the NDG app-support dir (prevents scanning NDG's own
//     databases, WAL, manifests)
//   - does not contain the current project database, WAL, or SHM sidecars
//   - not a duplicate of or nested inside / containing an existing
//     registered storage
func validateScanSource(root string, existing []domain.Storage, projectDBPath string) (string, error) {
	if root == "" {
		return "", errors.New("wails: scan source root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("wails: resolve scan source: %w", err)
	}

	// Must exist and be a directory; refuse a symlink at the root.
	info, err := os.Lstat(absRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("wails: scan source does not exist: %s", absRoot)
		}
		return "", fmt.Errorf("wails: inspect scan source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("wails: scan source must not be a symbolic link: %s", absRoot)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("wails: scan source must be a directory: %s", absRoot)
	}
	if info.Mode().Perm()&0o444 == 0 || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("wails: scan source is not readable and searchable: %s", absRoot)
	}

	// Evaluate intermediate symlinks in the path to get the real target.
	real, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("wails: resolve scan source symlinks: %w", err)
	}

	// Must be readable.
	f, err := os.Open(real)
	if err != nil {
		return "", fmt.Errorf("wails: scan source is not readable: %w", err)
	}
	_ = f.Close()

	// Must not be inside the NDG app-support dir — otherwise NDG could
	// scan its own databases, WAL files, and manifests. Resolve symlinks
	// on both sides (e.g. macOS /tmp → /private/tmp) before comparing.
	if ndgRoot, err := appSupportBase(); err == nil {
		if ndgReal, err := filepath.EvalSymlinks(ndgRoot); err == nil {
			ndgRoot = ndgReal
		}
		if isWithin(real, ndgRoot) {
			return "", fmt.Errorf("wails: scan source must not be inside the NDG application support directory")
		}
	}

	if err := rejectScanRootContainingProjectFiles(real, projectDBPath); err != nil {
		return "", err
	}

	// Must not duplicate or overlap an existing storage.
	for _, s := range existing {
		existingReal := s.RootPath
		if r, err := filepath.EvalSymlinks(s.RootPath); err == nil {
			existingReal = r
		}
		if existingReal == real {
			return "", fmt.Errorf("wails: scan source already registered: %s", s.RootPath)
		}
		if isWithin(real, existingReal) {
			return "", fmt.Errorf("wails: scan source is inside an already registered source: %s", s.RootPath)
		}
		if isWithin(existingReal, real) {
			return "", fmt.Errorf("wails: scan source contains an already registered source: %s", s.RootPath)
		}
	}
	return real, nil
}

// rejectScanRootContainingProjectFiles prevents a scan from indexing the
// currently open database and its SQLite sidecars. Checking the database
// itself is sufficient because WAL and SHM live beside it, but the error
// names all three artifacts to make the safety boundary explicit.
func rejectScanRootContainingProjectFiles(root, projectDBPath string) error {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(projectDBPath) == "" {
		return nil
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		realRoot, err = filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("wails: resolve scan root: %w", err)
		}
	}
	realDB, err := filepath.EvalSymlinks(projectDBPath)
	if err != nil {
		realDB, err = filepath.Abs(projectDBPath)
		if err != nil {
			return fmt.Errorf("wails: resolve project database: %w", err)
		}
	}
	if isWithin(realDB, realRoot) {
		return errors.New("wails: scan source must not contain the current project database or its WAL/SHM files")
	}
	return nil
}

// isWithin reports whether path is parent itself or a descendant of parent.
// Both paths must be cleaned (filepath.Clean) for a reliable result.
func isWithin(path, parent string) bool {
	p := filepath.Clean(path)
	par := filepath.Clean(parent)
	if p == par {
		return true
	}
	return strings.HasPrefix(p, par+string(filepath.Separator))
}

// generateStorageID derives a stable, opaque storage ID from the
// normalized absolute path. The same path always yields the same ID, so
// re-registering is an idempotent upsert rather than a duplicate. The
// frontend never chooses the ID.
func generateStorageID(absRoot string) string {
	h := sha256.Sum256([]byte(filepath.Clean(absRoot)))
	return "src_" + hex.EncodeToString(h[:6]) // 12 hex chars
}

// removeFreshProjectDir removes a freshly-created project directory on
// rollback. It refuses to follow symlinks and verifies the directory is
// actually under projectsRoot before removing, per AGENTS guardrail #4
// (do not follow symlinks, do not cross mount points, do not exceed the
// task root).
func removeFreshProjectDir(projectDir, projectsRoot string) error {
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return err
	}
	absRoot, err := filepath.Abs(projectsRoot)
	if err != nil {
		return err
	}
	if !isWithin(absDir, absRoot) {
		return fmt.Errorf("wails: refuse to remove dir outside projects root: %s", absDir)
	}
	info, err := os.Lstat(absDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("wails: refuse to remove symlink project dir: %s", absDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("wails: project path is not a directory: %s", absDir)
	}
	return os.RemoveAll(absDir)
}

// ---- shared service helpers ----

// initReadWriteServicesLocked initialises the full service layer for a
// read-write project: it runs crash recovery (marking stale non-terminal
// jobs FAILED) and wires the scan runner / job manager. The caller must
// hold a.mu. OpenProjectReadWrite and CreateProjectFromSource both use this.
func (a *API) initReadWriteServicesLocked(st *store.SQLiteStore) {
	mgr := jobs.New(st)
	_, _ = mgr.Recover(context.Background())
	a.store = st
	a.dupSvc = app.NewDuplicateServiceWithReader(st)
	a.diagSvc = app.NewDiagnosticService(st)
	a.planSvc = app.NewPlanService()
	a.reviewSvc = app.NewReviewService(st)
	a.quarantineSvc = app.NewQuarantineService(st)
	a.purgeSvc = app.NewPurgeService(st)
	a.recoverySvc = app.NewRecoveryService(st)
	a.executionSvc = app.NewExecutionService(st)
	a.jobMgr = mgr
	a.scanRunner = app.NewScanJobRunner(app.NewScanService(st), mgr)
}

// resetProjectStateLocked clears all project-scoped fields. The caller
// must hold a.mu and have already closed the store if one was open.
func (a *API) resetProjectStateLocked() {
	a.store = nil
	a.dupSvc = nil
	a.diagSvc = nil
	a.planSvc = nil
	a.reviewSvc = nil
	a.quarantineSvc = nil
	a.purgeSvc = nil
	a.recoverySvc = nil
	a.executionSvc = nil
	a.scanRunner = nil
	a.jobMgr = nil
	a.path = ""
	a.projectID = ""
	a.jobScope = ""
}

// ---- path + name-id helpers ----

// appSupportBaseFn returns the root for all project artifacts. It is a
// function variable so tests can redirect it to a temp dir without
// touching the real ~/Library/Application Support.
var appSupportBaseFn = defaultAppSupportBase

// appSupportBase returns <appSupportBaseFn()>, the root for all project
// artifacts. On macOS this is ~/Library/Application Support/NDG.
func appSupportBase() (string, error) {
	return appSupportBaseFn()
}

// defaultAppSupportBase returns <os.UserConfigDir()>/NDG. On macOS this
// is ~/Library/Application Support/NDG.
func defaultAppSupportBase() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("wails: resolve app support dir: %w", err)
	}
	return filepath.Join(base, appSupportDirName), nil
}

// projectDisplayName derives a human-readable name for an existing
// project from its db path, preferring project.json (if present) and
// falling back to the project directory name.
func projectDisplayName(dbPath string) string {
	dir := filepath.Dir(dbPath)
	if meta, ok := readProjectMeta(dir); ok && strings.TrimSpace(meta.Name) != "" {
		return meta.Name
	}
	parent := filepath.Base(dir)
	if parent != "" && parent != "." && parent != string(filepath.Separator) {
		return parent
	}
	return strings.TrimSuffix(filepath.Base(dbPath), filepath.Ext(dbPath))
}

// projectIdentity returns the logical project ID and display name. Managed
// projects persist both in project.json. Older databases have no metadata, so
// they receive a deterministic compatibility ID derived from their absolute
// path; the path itself is never exposed as ProjectInfo.ProjectID.
func projectIdentity(dbPath string) (string, string) {
	name := projectDisplayName(dbPath)
	if meta, ok := readProjectMeta(filepath.Dir(dbPath)); ok && strings.TrimSpace(meta.ProjectID) != "" {
		return meta.ProjectID, name
	}
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		absPath = filepath.Clean(dbPath)
	}
	h := sha256.Sum256([]byte(absPath))
	return "legacy_" + hex.EncodeToString(h[:8]), name
}

// projectJobScope preserves access to jobs written by older NDG versions,
// which used the absolute database path as their project scope. Managed
// projects use their persisted logical ID for all new jobs.
func projectJobScope(dbPath, projectID string) string {
	if meta, ok := readProjectMeta(filepath.Dir(dbPath)); ok && strings.TrimSpace(meta.ProjectID) != "" {
		return projectID
	}
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return filepath.Clean(dbPath)
	}
	return absPath
}

var (
	nameSep      = regexp.MustCompile(`[\s_]+`)
	nameStrip    = regexp.MustCompile(`[^\pL\pN-]+`)
	nameCollapse = regexp.MustCompile(`-{2,}`)
)

// normalizeNameID produces a filesystem-safe, unicode-aware directory
// name from a project name. Letters (including CJK) and digits are kept;
// whitespace/underscores become hyphens; everything else is removed.
// This is not an ASCII slug — CJK characters are preserved because macOS
// supports them natively in paths.
func normalizeNameID(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = nameSep.ReplaceAllString(s, "-")
	s = nameStrip.ReplaceAllString(s, "")
	s = nameCollapse.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// uniqueProjectID returns a non-colliding project directory name under
// projectsRoot for the given display name. On collision a "-2", "-3", ...
// suffix is appended. If the name yields an empty id, a timestamp-based
// ID is used so CJK-only or punctuation-only names still work.
func uniqueProjectID(projectsRoot, name string) string {
	id := normalizeNameID(name)
	if id == "" {
		id = "project-" + time.Now().Format("20060102-150405")
	}
	candidate := id
	for i := 2; ; i++ {
		if _, err := os.Lstat(filepath.Join(projectsRoot, candidate)); errors.Is(err, os.ErrNotExist) {
			return candidate
		} else if err != nil {
			return id + "-" + time.Now().Format("150405")
		}
		candidate = fmt.Sprintf("%s-%d", id, i)
		if i > 1000 {
			return id + "-" + time.Now().Format("150405")
		}
	}
}

// ---- project metadata (project.json) ----

// writeProjectMeta atomically writes project.json into projectDir.
func writeProjectMeta(projectDir string, meta ProjectMeta) error {
	p := filepath.Join(projectDir, projectMetaFileName)
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("wails: marshal project meta: %w", err)
	}
	return atomicWriteFile(p, data)
}

// readProjectMeta reads project.json from projectDir. Returns
// (ProjectMeta, false) when the file is missing or unparseable.
func readProjectMeta(projectDir string) (ProjectMeta, bool) {
	p := filepath.Join(projectDir, projectMetaFileName)
	data, err := os.ReadFile(p)
	if err != nil {
		return ProjectMeta{}, false
	}
	var meta ProjectMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return ProjectMeta{}, false
	}
	return meta, true
}

// ---- recent-projects manifest ----

// recentPath ensures the app-support dir exists and returns the path to
// recent.json.
func recentPath() (string, error) {
	base, err := appSupportBase()
	if err != nil {
		return "", err
	}
	if err := privatefs.SecureDirectory(base); err != nil {
		return "", fmt.Errorf("wails: secure app support dir: %w", err)
	}
	return filepath.Join(base, recentFileName), nil
}

// readRecentManifest loads the recent-projects list. A missing manifest is
// not an error; it yields an empty slice. A corrupt manifest is preserved
// aside (renamed to recent.json.corrupt-<ts>) for diagnosis, then treated
// as empty so a corrupt manifest never blocks opening projects.
func readRecentManifest() ([]RecentProjectEntry, error) {
	p, err := recentPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("wails: read recent manifest: %w", err)
	}
	var entries []RecentProjectEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		// Preserve the corrupt file for diagnosis, then treat as empty.
		_ = os.Rename(p, p+".corrupt-"+time.Now().Format("20060102-150405"))
		return nil, nil
	}
	return entries, nil
}

// writeRecentManifest atomically replaces recent.json with the given
// entries. It writes to a temp file, fsyncs, then renames over the
// target so a crash mid-write never leaves a truncated manifest.
func writeRecentManifest(entries []RecentProjectEntry) error {
	p, err := recentPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("wails: marshal recent manifest: %w", err)
	}
	return atomicWriteFile(p, data)
}

// atomicWriteFile writes data to path atomically: it creates a temp file
// in the same directory, writes, fsyncs, closes, then renames over the
// target. The temp file is removed on any error. Files are created with
// owner-only permissions via privatefs.
func atomicWriteFile(path string, data []byte) error {
	tmp := path + ".tmp"
	f, err := privatefs.Create(tmp)
	if err != nil {
		return fmt.Errorf("wails: create temp file: %w", err)
	}
	wrote := false
	defer func() {
		if !wrote {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("wails: write temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("wails: sync temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("wails: close temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("wails: rename temp file: %w", err)
	}
	wrote = true
	return nil
}

// recordRecentLocked prepends (or moves) an entry to the front of the
// recent-projects manifest, de-duplicating by path and capping the list.
// Errors are returned but callers treat them as best-effort: a manifest
// failure must not block opening a project. The caller must hold a.mu.
func (a *API) recordRecentLocked(name, dbPath string) error {
	entries, err := readRecentManifest()
	if err != nil {
		return err
	}
	out := make([]RecentProjectEntry, 0, len(entries)+1)
	out = append(out, RecentProjectEntry{Name: name, Path: dbPath, OpenedAt: time.Now().UTC()})
	for _, e := range entries {
		if e.Path == dbPath {
			continue
		}
		out = append(out, e)
	}
	if len(out) > recentMaxEntries {
		out = out[:recentMaxEntries]
	}
	return writeRecentManifest(out)
}
