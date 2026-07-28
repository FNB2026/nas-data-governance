// Package wails provides the Wails desktop binding layer that sits between
// the React frontend and the internal/app application services.
//
// Per ADR-0006:
//   - This is the ONLY package outside cmd/ used as the Wails binding layer.
//   - It does NOT import Wails framework packages directly.
//   - It exposes only high-level use cases, never raw file/SQL/command APIs.
//   - All returned types are plain structs with json tags, safe for Wails
//     to serialize to the frontend.
//   - V3 (Alpha) is read-only: no execution or write methods are bound.
package wails

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/query"
	"github.com/FNB2026/nas-data-governance/internal/store"
	"github.com/FNB2026/nas-data-governance/internal/version"
)

// VersionInfo is the DTO returned by GetVersion. It mirrors version.Info
// but is defined here to keep the adapter layer self-contained.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

// ProjectInfo describes the currently open project.
type ProjectInfo struct {
	// Path is the database file path (display-safe: this is the user's
	// chosen project file, not a NAS data path).
	Path         string `json:"path"`
	IsOpen       bool   `json:"is_open"`
	StorageCount int    `json:"storage_count"`
}

// ErrNoProjectOpen is returned when an operation requires an open project
// but none is currently open.
var ErrNoProjectOpen = errors.New("wails: no project open")

// ErrProjectAlreadyOpen is returned when a project is already open and
// OpenProject is called without first closing the current one.
var ErrProjectAlreadyOpen = errors.New("wails: a project is already open; close it first")

// API is the Wails-bound struct exposed to the frontend. Each public
// method becomes a callable JavaScript function via Wails bindings.
//
// The API is designed for V3 (read-only Alpha):
//   - GetVersion: returns build version info
//   - OpenProject: opens an existing project database read-only
//   - CloseProject: closes the current project
//   - GetProjectInfo: returns metadata about the open project
//   - ValidateProjectPath: checks if a file looks like a valid project DB
//   - ListStorages: lists all registered storage entries
//   - ListDuplicateGroups: paginated duplicate group summaries
//   - GetGroupDetail: full file list for a single duplicate group
//
// Future PRs will add: StartScan, GetScanProgress, etc.
type API struct {
	mu     sync.Mutex
	store  *store.SQLiteStore
	dupSvc *app.DuplicateService
	path   string
}

// NewAPI creates a new Wails API instance. The store starts nil;
// OpenProject initializes it.
func NewAPI() *API {
	return &API{}
}

// GetVersion returns the build version information.
func (a *API) GetVersion() VersionInfo {
	v := version.Get()
	return VersionInfo{
		Version:   v.Version,
		Commit:    v.Commit,
		BuildTime: v.BuildTime,
	}
}

// OpenProject opens a project database at the given path. If a project
// is already open, it returns ErrProjectAlreadyOpen.
//
// The path must be an existing regular SQLite database file. Opening a
// project never creates a file or applies migrations in the read-only Alpha.
func (a *API) OpenProject(path string) (ProjectInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.store != nil {
		return ProjectInfo{}, ErrProjectAlreadyOpen
	}

	if err := validateProjectPath(path); err != nil {
		return ProjectInfo{}, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("wails: resolve project path: %w", err)
	}

	st, err := store.OpenReadOnly(context.Background(), absPath)
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("wails: open project: %w", err)
	}

	a.store = st
	a.dupSvc = app.NewDuplicateServiceWithReader(st)
	a.path = absPath

	info, err := a.projectInfoLocked(context.Background())
	if err != nil {
		_ = st.Close()
		a.store = nil
		a.path = ""
		return ProjectInfo{}, err
	}
	return info, nil
}

// CloseProject closes the currently open project database. If no project
// is open, it is a no-op.
func (a *API) CloseProject() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.store == nil {
		return nil
	}

	err := a.store.Close()
	a.store = nil
	a.dupSvc = nil
	a.path = ""
	return err
}

// GetProjectInfo returns metadata about the currently open project.
// Returns ErrNoProjectOpen if no project is open.
func (a *API) GetProjectInfo() (ProjectInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.store == nil {
		return ProjectInfo{}, ErrNoProjectOpen
	}

	return a.projectInfoLocked(context.Background())
}

// ValidateProjectPath checks whether the given path looks like a valid
// project database file. It verifies the file exists and has a .db
// extension. It does NOT open the database.
//
// Returns nil if valid, an error describing the problem otherwise.
func (a *API) ValidateProjectPath(path string) error {
	return validateProjectPath(path)
}

func validateProjectPath(path string) error {
	if path == "" {
		return errors.New("wails: project path is empty")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("wails: resolve path: %w", err)
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("wails: file not found: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("wails: project path must not be a symbolic link")
	}
	if info.IsDir() {
		return errors.New("wails: path is a directory, not a file")
	}
	if !info.Mode().IsRegular() {
		return errors.New("wails: project path is not a regular file")
	}
	ext := strings.ToLower(filepath.Ext(absPath))
	if ext != ".db" && ext != ".sqlite" && ext != ".sqlite3" {
		return fmt.Errorf("wails: expected .db, .sqlite, or .sqlite3 extension, got %s", ext)
	}
	return nil
}

// projectInfoLocked builds ProjectInfo from the current state.
// Caller must hold a.mu.
func (a *API) projectInfoLocked(ctx context.Context) (ProjectInfo, error) {
	storages, err := a.store.ListStorages(ctx)
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("wails: read project metadata: %w", err)
	}
	return ProjectInfo{
		Path:         a.path,
		IsOpen:       true,
		StorageCount: len(storages),
	}, nil
}

// ---- V3 Alpha query bindings ----

// ListStorages returns all registered storage entries in the open project.
func (a *API) ListStorages() ([]StorageInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.store == nil {
		return nil, ErrNoProjectOpen
	}
	storages, err := a.store.ListStorages(context.Background())
	if err != nil {
		return nil, fmt.Errorf("wails: list storages: %w", err)
	}
	return mapStorages(storages), nil
}

// ListDuplicateGroups returns a page of duplicate group summaries.
// Pagination is keyset-based: pass the NextCursor from the previous
// response to fetch the next page.
func (a *API) ListDuplicateGroups(req ListGroupsRequest) (ListGroupsResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.dupSvc == nil {
		return ListGroupsResponse{}, ErrNoProjectOpen
	}
	q, err := req.toQuery()
	if err != nil {
		return ListGroupsResponse{}, err
	}
	page, err := a.dupSvc.ListGroups(context.Background(), q)
	if err != nil {
		return ListGroupsResponse{}, fmt.Errorf("wails: list duplicate groups: %w", err)
	}
	return mapGroupPage(page), nil
}

// GetGroupDetail loads the full file member list for a single duplicate
// group identified by storageID and content SHA-256.
func (a *API) GetGroupDetail(storageID, sha256 string) (GroupDetailResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.dupSvc == nil {
		return GroupDetailResponse{}, ErrNoProjectOpen
	}
	detail, err := a.dupSvc.GroupDetail(context.Background(), storageID, sha256)
	if err != nil {
		if errors.Is(err, query.ErrNotFound) {
			return GroupDetailResponse{}, fmt.Errorf("wails: group not found")
		}
		return GroupDetailResponse{}, fmt.Errorf("wails: get group detail: %w", err)
	}
	return mapGroupDetail(detail), nil
}
