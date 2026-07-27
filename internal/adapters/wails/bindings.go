// Package wails provides the Wails desktop binding layer that sits between
// the React frontend and the internal/app application services.
//
// Per ADR-0006:
//   - This is the ONLY package outside cmd/ that may import Wails types
//     (for context.Context passed by the Wails runtime).
//   - It does NOT import Wails framework packages directly — it receives
//     context.Context from the Wails runtime and passes it to app services.
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
	"sync"

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
//   - OpenProject: opens a project database (read-write for scan/learn)
//   - CloseProject: closes the current project
//   - GetProjectInfo: returns metadata about the open project
//   - ValidateProjectPath: checks if a file looks like a valid project DB
//
// Future PRs will add: ListStorages, ListDuplicateGroups, GetGroupDetail,
// StartScan, GetScanProgress, etc.
type API struct {
	mu    sync.Mutex
	store *store.SQLiteStore
	path  string
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
// The path must be a valid SQLite database file. The method creates the
// database if it does not exist (store.Open applies migrations).
func (a *API) OpenProject(ctx context.Context, path string) (ProjectInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.store != nil {
		return ProjectInfo{}, ErrProjectAlreadyOpen
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("wails: resolve project path: %w", err)
	}

	// Verify the file exists and is not a directory.
	if info, err := os.Stat(absPath); err == nil {
		if info.IsDir() {
			return ProjectInfo{}, fmt.Errorf("wails: project path is a directory, not a file")
		}
	} else if !os.IsNotExist(err) {
		return ProjectInfo{}, fmt.Errorf("wails: inspect project path: %w", err)
	}

	st, err := store.Open(ctx, absPath)
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("wails: open project: %w", err)
	}

	a.store = st
	a.path = absPath

	return a.projectInfoLocked(), nil
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
	a.path = ""
	return err
}

// GetProjectInfo returns metadata about the currently open project.
// Returns ErrNoProjectOpen if no project is open.
func (a *API) GetProjectInfo(ctx context.Context) (ProjectInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.store == nil {
		return ProjectInfo{}, ErrNoProjectOpen
	}

	return a.projectInfoLocked(), nil
}

// ValidateProjectPath checks whether the given path looks like a valid
// project database file. It verifies the file exists and has a .db
// extension. It does NOT open the database.
//
// Returns nil if valid, an error describing the problem otherwise.
func (a *API) ValidateProjectPath(path string) error {
	if path == "" {
		return errors.New("wails: project path is empty")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("wails: resolve path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("wails: file not found: %w", err)
	}
	if info.IsDir() {
		return errors.New("wails: path is a directory, not a file")
	}
	ext := filepath.Ext(absPath)
	if ext != ".db" && ext != ".sqlite" && ext != ".sqlite3" {
		return fmt.Errorf("wails: expected .db, .sqlite, or .sqlite3 extension, got %s", ext)
	}
	return nil
}

// projectInfoLocked builds ProjectInfo from the current state.
// Caller must hold a.mu.
func (a *API) projectInfoLocked() ProjectInfo {
	return ProjectInfo{
		Path:   a.path,
		IsOpen: true,
	}
}

// Store returns the currently open store, or nil. This is used by
// the Wails main.go to pass the store to other bound services
// (e.g., ScanService) when they are added in future PRs.
func (a *API) Store() *store.SQLiteStore {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.store
}
