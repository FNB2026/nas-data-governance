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
	"time"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
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

// ErrProjectNotReadWrite is returned when a V4 write operation (scan,
// cancel) is attempted on a project opened in read-only mode.
var ErrProjectNotReadWrite = errors.New("wails: project is open read-only; reopen with OpenProjectReadWrite for scan operations")

// API is the Wails-bound struct exposed to the frontend. Each public
// method becomes a callable JavaScript function via Wails bindings.
//
// V3 (Alpha, read-only): GetVersion, OpenProject, CloseProject,
// GetProjectInfo, ValidateProjectPath, ListStorages,
// ListDuplicateGroups, GetGroupDetail.
//
// V4 (scan & jobs): OpenProjectReadWrite, StartScan, GetScanProgress,
// CancelScan, ListRecentJobs, GetJobDetail.
type API struct {
	mu         sync.RWMutex
	store      *store.SQLiteStore
	dupSvc     *app.DuplicateService
	scanRunner *app.ScanJobRunner
	jobMgr     *jobs.JobManager
	path       string
	projectID  string // used as JobManager project scope; set to abs DB path
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
		a.dupSvc = nil
		a.scanRunner = nil
		a.jobMgr = nil
		a.path = ""
		a.projectID = ""
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
	a.scanRunner = nil
	a.jobMgr = nil
	a.path = ""
	a.projectID = ""
	return err
}

// GetProjectInfo returns metadata about the currently open project.
// Returns ErrNoProjectOpen if no project is open.
func (a *API) GetProjectInfo() (ProjectInfo, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

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
	a.mu.RLock()
	defer a.mu.RUnlock()

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
	a.mu.RLock()
	defer a.mu.RUnlock()

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
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.dupSvc == nil {
		return GroupDetailResponse{}, ErrNoProjectOpen
	}
	if strings.TrimSpace(storageID) == "" || strings.TrimSpace(sha256) == "" {
		return GroupDetailResponse{}, errors.New("wails: storage_id and sha256 are required")
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

// ---- V4 scan & job bindings ----

// validateProjectPathRW validates a project path for read-write mode.
// Unlike validateProjectPath, it allows the file to not exist (creation
// is permitted). If the file exists, it must be a regular non-symlink
// file with a recognised extension.
func validateProjectPathRW(path string) error {
	if path == "" {
		return errors.New("wails: project path is empty")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("wails: resolve path: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(absPath))
	if ext != ".db" && ext != ".sqlite" && ext != ".sqlite3" {
		return fmt.Errorf("wails: expected .db, .sqlite, or .sqlite3 extension, got %s", ext)
	}
	info, err := os.Lstat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // creation is allowed in RW mode
		}
		return fmt.Errorf("wails: inspect project path: %w", err)
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
	return nil
}

// OpenProjectReadWrite opens a project database in read-write mode,
// creating it (with migrations) if it does not exist. This initialises
// the ScanService, JobManager, and ScanJobRunner required for V4 scan
// operations.
//
// If a project is already open (read-only or read-write), it returns
// ErrProjectAlreadyOpen. Call CloseProject first.
//
// On open, the JobManager runs crash recovery: any non-terminal jobs
// left from a previous session are marked FAILED with error_code
// "crash_recovery".
func (a *API) OpenProjectReadWrite(path string) (ProjectInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.store != nil {
		return ProjectInfo{}, ErrProjectAlreadyOpen
	}

	if err := validateProjectPathRW(path); err != nil {
		return ProjectInfo{}, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("wails: resolve project path: %w", err)
	}

	st, err := store.Open(context.Background(), absPath)
	if err != nil {
		return ProjectInfo{}, fmt.Errorf("wails: open project (read-write): %w", err)
	}

	// Crash recovery: mark stale non-terminal jobs as FAILED before
	// accepting new work. Errors here are non-fatal — the project is
	// still usable.
	mgr := jobs.New(st)
	_, _ = mgr.Recover(context.Background())

	a.store = st
	a.dupSvc = app.NewDuplicateServiceWithReader(st)
	a.jobMgr = mgr
	a.scanRunner = app.NewScanJobRunner(app.NewScanService(st), mgr)
	a.path = absPath
	a.projectID = absPath

	info, err := a.projectInfoLocked(context.Background())
	if err != nil {
		_ = st.Close()
		a.store = nil
		a.dupSvc = nil
		a.scanRunner = nil
		a.jobMgr = nil
		a.path = ""
		a.projectID = ""
		return ProjectInfo{}, err
	}
	return info, nil
}

// StartScan begins an asynchronous filesystem scan. It returns the job
// ID immediately so the frontend can poll progress via GetScanProgress.
//
// The project must be opened with OpenProjectReadWrite (not OpenProject).
// The root directory must exist and be readable. If storage_id is empty,
// it defaults to "default".
func (a *API) StartScan(req StartScanRequest) (StartScanResponse, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.scanRunner == nil {
		if a.store != nil {
			return StartScanResponse{}, ErrProjectNotReadWrite
		}
		return StartScanResponse{}, ErrNoProjectOpen
	}
	if strings.TrimSpace(req.Root) == "" {
		return StartScanResponse{}, errors.New("wails: root is required")
	}

	storageID := strings.TrimSpace(req.StorageID)
	if storageID == "" {
		storageID = "default"
	}

	workers := req.Workers
	if workers < 1 {
		workers = 4
	}

	in := app.ScanInput{
		Root:           req.Root,
		StorageID:      storageID,
		FullScan:       req.FullScan,
		Workers:        workers,
		HashAttempts:   3,
		HashRetryDelay: 1 * time.Second,
	}

	jobID, err := a.scanRunner.StartScanJob(context.Background(), a.projectID, in)
	if err != nil {
		return StartScanResponse{}, fmt.Errorf("wails: start scan: %w", err)
	}
	return StartScanResponse{JobID: jobID}, nil
}

// GetScanProgress returns the current state and progress counters for a
// scan job. The jobID must have been returned by StartScan.
func (a *API) GetScanProgress(jobID string) (ScanJobProgress, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.jobMgr == nil {
		if a.store != nil {
			return ScanJobProgress{}, ErrProjectNotReadWrite
		}
		return ScanJobProgress{}, ErrNoProjectOpen
	}
	if strings.TrimSpace(jobID) == "" {
		return ScanJobProgress{}, errors.New("wails: job_id is required")
	}

	j, err := a.jobMgr.Get(context.Background(), jobID)
	if err != nil {
		if errors.Is(err, jobs.ErrNotFound) {
			return ScanJobProgress{}, fmt.Errorf("wails: job not found")
		}
		return ScanJobProgress{}, fmt.Errorf("wails: get scan progress: %w", err)
	}
	return mapScanJobProgress(j), nil
}

// CancelScan requests cancellation of a running scan job. The job's
// context is cancelled, causing the scan to stop gracefully. The state
// transitions to CANCEL_REQUESTED immediately and CANCELLED when the
// scan function returns.
//
// If the job is not running, this is a no-op.
func (a *API) CancelScan(jobID string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.jobMgr == nil {
		if a.store != nil {
			return ErrProjectNotReadWrite
		}
		return ErrNoProjectOpen
	}
	if strings.TrimSpace(jobID) == "" {
		return errors.New("wails: job_id is required")
	}

	if err := a.jobMgr.RequestCancel(context.Background(), jobID); err != nil {
		return fmt.Errorf("wails: cancel scan: %w", err)
	}
	return nil
}

// ListRecentJobs returns the most recent jobs for the current project,
// ordered by created_at DESC. The limit defaults to 20 and is capped at 100.
func (a *API) ListRecentJobs(limit int) ([]JobSummary, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.jobMgr == nil {
		if a.store != nil {
			return nil, ErrProjectNotReadWrite
		}
		return nil, ErrNoProjectOpen
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	jobRuns, err := a.jobMgr.ListRecent(context.Background(), a.projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("wails: list recent jobs: %w", err)
	}
	out := make([]JobSummary, len(jobRuns))
	for i, j := range jobRuns {
		out[i] = mapJobSummary(j)
	}
	return out, nil
}

// GetJobDetail returns full details for a job, including its structured
// event history. The jobID must have been returned by StartScan or
// appear in ListRecentJobs.
func (a *API) GetJobDetail(jobID string) (JobDetailResponse, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.jobMgr == nil {
		if a.store != nil {
			return JobDetailResponse{}, ErrProjectNotReadWrite
		}
		return JobDetailResponse{}, ErrNoProjectOpen
	}
	if strings.TrimSpace(jobID) == "" {
		return JobDetailResponse{}, errors.New("wails: job_id is required")
	}

	j, err := a.jobMgr.Get(context.Background(), jobID)
	if err != nil {
		if errors.Is(err, jobs.ErrNotFound) {
			return JobDetailResponse{}, fmt.Errorf("wails: job not found")
		}
		return JobDetailResponse{}, fmt.Errorf("wails: get job: %w", err)
	}

	evs, err := a.jobMgr.ListEvents(context.Background(), jobID)
	if err != nil {
		return JobDetailResponse{}, fmt.Errorf("wails: list job events: %w", err)
	}

	events := make([]JobEvent, len(evs))
	for i, e := range evs {
		events[i] = mapJobEvent(e)
	}

	return JobDetailResponse{
		ScanJobProgress: mapScanJobProgress(j),
		Events:          events,
	}, nil
}
