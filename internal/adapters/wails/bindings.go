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
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/formatdiag"
	"github.com/FNB2026/nas-data-governance/internal/governancediag"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
	"github.com/FNB2026/nas-data-governance/internal/merge"
	projectsvc "github.com/FNB2026/nas-data-governance/internal/project"
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
	// ProjectID is the immutable logical project identifier. For managed
	// projects it comes from project.json and is intentionally distinct
	// from Path, which is the database location.
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
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
//
// V5 (diagnostics): DiagnoseFormats, DiagnoseGovernance, DiagnoseMerges.
//
// V6 (governance): ListAllPlans, ListReviewPlans, BuildDraftPlans,
// GetGroupDecision, SaveGroupDecision, ListGroupDecisions, ApprovePlans.
//
// V7 (execution): ListQuarantineItems, CreateRestorePlan, ApproveRestorePlan,
// ExecuteRestore, CreatePurgePlans, ApprovePurgePlan, ExecutePurge,
// CheckRecoveryLock, RecoverSourcePlans, RecoverRestores, RecoverPurges.
//
// V8 (audit): ListOperationLogs, ListJournalEntries.
//
// V9 (first-launch): PickDirectory, CreateProjectFromSource, RegisterScanSource,
// ListRecentProjects. New projects are created under the OS app-support
// dir (~/Library/Application Support/NDG/projects/<id>/governance.db on
// macOS) with 0700/0600 perms, never inside the scan source.
type API struct {
	mu            sync.RWMutex
	store         *store.SQLiteStore
	dupSvc        *app.DuplicateService
	scanRunner    *app.ScanJobRunner
	jobMgr        *jobs.JobManager
	diagSvc       *app.DiagnosticService
	planSvc       *app.PlanService
	reviewSvc     *app.ReviewService
	quarantineSvc *app.QuarantineService
	purgeSvc      *app.PurgeService
	recoverySvc   *app.RecoveryService
	executionSvc  *app.ExecutionService
	path          string
	projectID     string // logical ID exposed through ProjectInfo; never a database path
	jobScope      string // legacy JobManager compatibility scope; may be the historical abs path
	projectSvc    *projectsvc.Service
	dirPicker     DirectoryPicker // injected by main.go; nil outside Wails runtime
}

// NewAPI creates a new Wails API instance. The store starts nil;
// OpenProject initializes it.
func NewAPI() *API {
	return &API{projectSvc: newProjectService()}
}

// initReadWriteServicesLocked is composition-root wiring, not project
// lifecycle logic. The caller must hold a.mu.
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
	a.diagSvc = app.NewDiagnosticService(st)
	a.planSvc = app.NewPlanService()
	a.reviewSvc = app.NewReviewService(st)
	a.quarantineSvc = app.NewQuarantineService(st)
	a.purgeSvc = app.NewPurgeService(st)
	a.recoverySvc = app.NewRecoveryService(st)
	a.executionSvc = app.NewExecutionService(st)
	a.path = absPath
	a.projectID, _, a.jobScope = a.resolveProjectIdentity(absPath)

	info, err := a.projectInfoLocked(context.Background())
	if err != nil {
		_ = st.Close()
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
		return ProjectInfo{}, err
	}
	_ = a.recordRecentLocked(a.projectDisplayName(absPath), absPath)
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
		ProjectID:    a.projectID,
		Name:         a.projectDisplayName(a.path),
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

	// Crash recovery + service init. initReadWriteServicesLocked runs
	// crash recovery (marking stale non-terminal jobs FAILED) and wires
	// the scan runner / job manager required for V4 scan operations.
	a.initReadWriteServicesLocked(st)
	a.path = absPath
	a.projectID, _, a.jobScope = a.resolveProjectIdentity(absPath)

	info, err := a.projectInfoLocked(context.Background())
	if err != nil {
		_ = st.Close()
		a.resetProjectStateLocked()
		return ProjectInfo{}, err
	}
	_ = a.recordRecentLocked(a.projectDisplayName(absPath), absPath)
	return info, nil
}

// StartScan begins an asynchronous filesystem scan. It returns the job
// ID immediately so the frontend can poll progress via GetScanProgress.
//
// The project must be opened with OpenProjectReadWrite (not OpenProject).
// The root directory must exist and be readable. If storage_id is empty,
// a stable ID is derived from the root path (same as RegisterScanSource).
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
	if err := a.rejectScanRootContainingProjectFiles(req.Root, a.path); err != nil {
		return StartScanResponse{}, err
	}

	storageID := strings.TrimSpace(req.StorageID)
	if storageID == "" {
		// Derive a stable ID from the root path, consistent with
		// RegisterScanSource / CreateProjectFromSource. This avoids a
		// "default" vs "src_<hash>" mismatch when the user types a root
		// manually instead of picking a registered storage.
		storageID = projectsvc.GenerateStorageID(req.Root)
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

	jobID, err := a.scanRunner.StartScanJob(context.Background(), a.jobScope, in)
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

	jobRuns, err := a.jobMgr.ListRecent(context.Background(), a.jobScope, limit)
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

// ---- V5 diagnostic bindings ----

// DiagnoseFormats builds a read-only format review report from the
// open project database. It identifies large unknown files, extension
// mismatches, and metadata gaps. No filesystem access is performed.
//
// The project must be open (read-only or read-write).
func (a *API) DiagnoseFormats(req DiagnoseFormatsRequest) (*formatdiag.Report, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.diagSvc == nil {
		return nil, ErrNoProjectOpen
	}
	report, err := a.diagSvc.DiagnoseFormats(context.Background(), app.DiagnoseFormatsInput{
		StorageID:           req.StorageID,
		LargeUnknownMinimum: req.LargeUnknownMinimum,
	})
	if err != nil {
		return nil, fmt.Errorf("wails: diagnose formats: %w", err)
	}
	return report, nil
}

// DiagnoseGovernance builds a read-only governance review report from
// the open project database. It analyzes duplicate groups, zero-byte
// files, and large media files with their relations.
//
// All generated plans are DRAFT. The service refuses non-draft results.
// The project must be open (read-only or read-write).
func (a *API) DiagnoseGovernance(req DiagnoseGovernanceRequest) (*governancediag.Report, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.diagSvc == nil {
		return nil, ErrNoProjectOpen
	}
	report, err := a.diagSvc.DiagnoseGovernance(context.Background(), app.DiagnoseGovernanceInput{
		StorageID:         req.StorageID,
		LargeMediaMinimum: req.LargeMediaMinimum,
	})
	if err != nil {
		return nil, fmt.Errorf("wails: diagnose governance: %w", err)
	}
	return report, nil
}

// DiagnoseMerges builds a read-only merge gate review report from the
// open project database. It identifies sibling directories with similar
// names and evaluates filename overlap using Jaccard similarity.
//
// The project must be open (read-only or read-write).
func (a *API) DiagnoseMerges(req DiagnoseMergesRequest) (*merge.DiagnosticReport, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.diagSvc == nil {
		return nil, ErrNoProjectOpen
	}
	report, err := a.diagSvc.DiagnoseMerges(context.Background(), app.DiagnoseMergesInput{
		StorageID: req.StorageID,
	})
	if err != nil {
		return nil, fmt.Errorf("wails: diagnose merges: %w", err)
	}
	return report, nil
}

// ---- V6 governance bindings ----

// ListAllPlans returns all operation plans across all tasks, ordered by
// task creation time then plan ID. Available in both read-only and
// read-write modes.
func (a *API) ListAllPlans() ([]PlanDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.store == nil {
		return nil, ErrNoProjectOpen
	}
	plans, err := a.store.ListAllPlans(context.Background())
	if err != nil {
		return nil, fmt.Errorf("wails: list all plans: %w", err)
	}
	out := make([]PlanDTO, len(plans))
	for i, p := range plans {
		out[i] = mapPlan(p)
	}
	return out, nil
}

// ListReviewPlans returns plans that contain at least one REVIEW action.
// These are the plans that need human review before they can proceed.
// Available in both read-only and read-write modes.
func (a *API) ListReviewPlans() ([]PlanDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.reviewSvc == nil {
		return nil, ErrNoProjectOpen
	}
	plans, err := a.reviewSvc.ListReviewPlans(context.Background())
	if err != nil {
		return nil, fmt.Errorf("wails: list review plans: %w", err)
	}
	out := make([]PlanDTO, len(plans))
	for i, p := range plans {
		out[i] = mapPlan(p)
	}
	return out, nil
}

// BuildDraftPlans generates draft governance plans from scanned files.
// This is a read-only operation: it loads files, groups them into duplicate
// groups, and runs the planner. The plans are NOT persisted; they are
// returned for review only.
//
// If storageID is empty, all storages are included.
func (a *API) BuildDraftPlans(storageID string) ([]PlanDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.planSvc == nil {
		return nil, ErrNoProjectOpen
	}
	files, err := a.store.ListFiles(context.Background(), strings.TrimSpace(storageID))
	if err != nil {
		return nil, fmt.Errorf("wails: load files for planning: %w", err)
	}
	if len(files) == 0 {
		return []PlanDTO{}, nil
	}
	plans := a.planSvc.BuildPlans(context.Background(), files)
	out := make([]PlanDTO, len(plans))
	for i, p := range plans {
		out[i] = mapPlan(p)
	}
	return out, nil
}

// GetGroupDecision returns the review decision for a group, or an error
// if no decision has been recorded.
func (a *API) GetGroupDecision(groupID string) (GroupDecisionDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.reviewSvc == nil {
		return GroupDecisionDTO{}, ErrNoProjectOpen
	}
	if strings.TrimSpace(groupID) == "" {
		return GroupDecisionDTO{}, errors.New("wails: group_id is required")
	}
	d, err := a.reviewSvc.GetDecision(context.Background(), groupID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return GroupDecisionDTO{}, fmt.Errorf("wails: no decision for group")
		}
		return GroupDecisionDTO{}, fmt.Errorf("wails: get group decision: %w", err)
	}
	return mapDecision(d), nil
}

// SaveGroupDecision records a review decision for a duplicate group.
// The decision is upserted by group_id, so there can be only one decision
// per group. Requires read-write mode.
func (a *API) SaveGroupDecision(req SaveDecisionRequest) (GroupDecisionDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.reviewSvc == nil {
		return GroupDecisionDTO{}, ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return GroupDecisionDTO{}, ErrProjectNotReadWrite
	}
	if strings.TrimSpace(req.GroupID) == "" {
		return GroupDecisionDTO{}, errors.New("wails: group_id is required")
	}
	if strings.TrimSpace(req.DecisionType) == "" {
		return GroupDecisionDTO{}, errors.New("wails: decision_type is required")
	}

	now := time.Now().UTC()
	d := domain.GroupDecision{
		ID:           req.GroupID,
		GroupID:      req.GroupID,
		DecisionType: domain.ReviewDecisionType(req.DecisionType),
		Reason:       req.Reason,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Preserve created_at if a decision already exists.
	if existing, err := a.reviewSvc.GetDecision(context.Background(), req.GroupID); err == nil {
		d.CreatedAt = existing.CreatedAt
	}

	if err := a.reviewSvc.SaveDecision(context.Background(), d); err != nil {
		return GroupDecisionDTO{}, fmt.Errorf("wails: save group decision: %w", err)
	}
	return mapDecision(d), nil
}

// ListGroupDecisions returns review decisions, optionally filtered by type.
// Pass an empty string for decisionType to return all decisions.
func (a *API) ListGroupDecisions(decisionType string) ([]GroupDecisionDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.reviewSvc == nil {
		return nil, ErrNoProjectOpen
	}
	decisions, err := a.reviewSvc.ListDecisions(context.Background(), domain.ReviewDecisionType(decisionType))
	if err != nil {
		return nil, fmt.Errorf("wails: list group decisions: %w", err)
	}
	out := make([]GroupDecisionDTO, len(decisions))
	for i, d := range decisions {
		out[i] = mapDecision(d)
	}
	return out, nil
}

// ApprovePlans transitions selected plans from DRAFT to APPROVED and
// persists the state change. Requires read-write mode.
//
// Critical-risk plans cannot be approved here; they require an independent
// hold-release workflow. Non-DRAFT plans are rejected.
func (a *API) ApprovePlans(req ApprovePlansRequest) (ApprovePlansResponse, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.store == nil {
		return ApprovePlansResponse{}, ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return ApprovePlansResponse{}, ErrProjectNotReadWrite
	}
	if len(req.PlanIDs) == 0 {
		return ApprovePlansResponse{}, errors.New("wails: at least one plan_id is required")
	}

	// Load all plans so PlanService can validate transitions.
	allPlans, err := a.store.ListAllPlans(context.Background())
	if err != nil {
		return ApprovePlansResponse{}, fmt.Errorf("wails: load plans for approval: %w", err)
	}

	result, err := a.planSvc.ApprovePlans(context.Background(), app.ApprovePlansInput{
		Plans: allPlans,
		IDs:   req.PlanIDs,
	})
	if err != nil {
		return ApprovePlansResponse{}, fmt.Errorf("wails: approve plans: %w", err)
	}

	// Persist each state transition.
	for _, p := range result.Approved {
		if err := a.store.UpdatePlanState(context.Background(), p.ID, p.State); err != nil {
			return ApprovePlansResponse{}, fmt.Errorf("wails: persist approval for plan %s: %w", p.ID, err)
		}
	}

	approved := make([]PlanDTO, len(result.Approved))
	for i, p := range result.Approved {
		approved[i] = mapPlan(p)
	}
	return ApprovePlansResponse{Approved: approved}, nil
}

// ---- V7 execution bindings ----

// ListQuarantineItems returns quarantined file entries, optionally filtered
// by lifecycle status. Pass an empty string for status to return all items.
// Available in both read-only and read-write modes.
func (a *API) ListQuarantineItems(status string) ([]QuarantineItemDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.quarantineSvc == nil {
		return nil, ErrNoProjectOpen
	}
	items, err := a.quarantineSvc.ListItems(context.Background(), domain.QuarantineStatus(status))
	if err != nil {
		return nil, fmt.Errorf("wails: list quarantine items: %w", err)
	}
	out := make([]QuarantineItemDTO, len(items))
	for i, item := range items {
		out[i] = mapQuarantineItem(item)
	}
	return out, nil
}

// ListRestorePlans returns durable restore plans for restart-safe desktop use.
func (a *API) ListRestorePlans() ([]RestorePlanDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.quarantineSvc == nil {
		return nil, ErrNoProjectOpen
	}
	plans, err := a.quarantineSvc.ListRestorePlans(context.Background())
	if err != nil {
		return nil, fmt.Errorf("wails: list restore plans: %w", err)
	}
	out := make([]RestorePlanDTO, len(plans))
	for i, plan := range plans {
		out[i] = mapRestorePlan(plan)
	}
	return out, nil
}

// CreateRestorePlan builds a DRAFT restore plan for a single quarantine item.
// The plan is persisted to the database. Requires read-write mode.
func (a *API) CreateRestorePlan(itemID string) (RestorePlanDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.quarantineSvc == nil {
		return RestorePlanDTO{}, ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return RestorePlanDTO{}, ErrProjectNotReadWrite
	}
	if strings.TrimSpace(itemID) == "" {
		return RestorePlanDTO{}, errors.New("wails: item_id is required")
	}
	plan, err := a.quarantineSvc.CreateRestorePlan(context.Background(), itemID)
	if err != nil {
		return RestorePlanDTO{}, fmt.Errorf("wails: create restore plan: %w", err)
	}
	return mapRestorePlan(*plan), nil
}

// ApproveRestorePlan transitions a restore plan from DRAFT to APPROVED using
// the plan's digest. Requires read-write mode.
func (a *API) ApproveRestorePlan(planID, digest string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.quarantineSvc == nil {
		return ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return ErrProjectNotReadWrite
	}
	if strings.TrimSpace(planID) == "" {
		return errors.New("wails: plan_id is required")
	}
	if strings.TrimSpace(digest) == "" {
		return errors.New("wails: digest is required")
	}
	if err := a.quarantineSvc.ApproveRestorePlan(context.Background(), planID, digest); err != nil {
		return fmt.Errorf("wails: approve restore plan: %w", err)
	}
	return nil
}

// ExecuteRestore executes an approved restore plan. When dry_run is true,
// only validation is performed. Requires read-write mode.
func (a *API) ExecuteRestore(req ExecuteRestoreRequest) (ExecuteRestoreResponse, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.quarantineSvc == nil {
		return ExecuteRestoreResponse{}, ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return ExecuteRestoreResponse{}, ErrProjectNotReadWrite
	}
	if strings.TrimSpace(req.PlanID) == "" {
		return ExecuteRestoreResponse{}, errors.New("wails: plan_id is required")
	}
	if strings.TrimSpace(req.Digest) == "" {
		return ExecuteRestoreResponse{}, errors.New("wails: digest is required")
	}
	if strings.TrimSpace(req.QuarantineRoot) == "" {
		return ExecuteRestoreResponse{}, errors.New("wails: quarantine_root is required")
	}

	result, err := a.quarantineSvc.ExecuteRestore(context.Background(), app.RestoreExecuteInput{
		PlanID:         req.PlanID,
		Digest:         req.Digest,
		QuarantineRoot: req.QuarantineRoot,
		SourceRoots:    req.SourceRoots,
		DryRun:         req.DryRun,
	})
	if err != nil {
		return ExecuteRestoreResponse{}, fmt.Errorf("wails: execute restore: %w", err)
	}
	return ExecuteRestoreResponse{
		PlanID:     result.Result.PlanID,
		FinalState: string(result.Result.FinalState),
		Status:     string(result.Result.Status),
		ErrorType:  result.Result.ErrorType,
		Error:      errText(result.Result.Err),
	}, nil
}

// CreatePurgePlans builds DRAFT purge plans for all eligible quarantine items
// whose retention period has expired. Plans are persisted to the database.
// Requires read-write mode.
func (a *API) CreatePurgePlans() ([]PurgePlanDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.purgeSvc == nil {
		return nil, ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return nil, ErrProjectNotReadWrite
	}
	plans, err := a.purgeSvc.CreatePlans(context.Background())
	if err != nil {
		return nil, fmt.Errorf("wails: create purge plans: %w", err)
	}
	out := make([]PurgePlanDTO, len(plans))
	for i, p := range plans {
		out[i] = mapPurgePlan(p)
	}
	return out, nil
}

// ListPurgePlans returns durable purge plans for restart-safe desktop use.
func (a *API) ListPurgePlans() ([]PurgePlanDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.purgeSvc == nil {
		return nil, ErrNoProjectOpen
	}
	plans, err := a.purgeSvc.ListPlans(context.Background())
	if err != nil {
		return nil, fmt.Errorf("wails: list purge plans: %w", err)
	}
	out := make([]PurgePlanDTO, len(plans))
	for i, plan := range plans {
		out[i] = mapPurgePlan(plan)
	}
	return out, nil
}

// ApprovePurgePlan transitions a purge plan from DRAFT to APPROVED using
// the plan's digest. Requires read-write mode.
func (a *API) ApprovePurgePlan(planID, digest string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.purgeSvc == nil {
		return ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return ErrProjectNotReadWrite
	}
	if strings.TrimSpace(planID) == "" {
		return errors.New("wails: plan_id is required")
	}
	if strings.TrimSpace(digest) == "" {
		return errors.New("wails: digest is required")
	}
	if err := a.purgeSvc.ApprovePlan(context.Background(), planID, digest); err != nil {
		return fmt.Errorf("wails: approve purge plan: %w", err)
	}
	return nil
}

// ExecutePurge executes an approved purge plan. When dry_run is true, only
// validation is performed. Requires read-write mode.
func (a *API) ExecutePurge(req ExecutePurgeRequest) (ExecutePurgeResponse, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.purgeSvc == nil {
		return ExecutePurgeResponse{}, ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return ExecutePurgeResponse{}, ErrProjectNotReadWrite
	}
	if strings.TrimSpace(req.PlanID) == "" {
		return ExecutePurgeResponse{}, errors.New("wails: plan_id is required")
	}
	if strings.TrimSpace(req.Digest) == "" {
		return ExecutePurgeResponse{}, errors.New("wails: digest is required")
	}
	if strings.TrimSpace(req.QuarantineRoot) == "" {
		return ExecutePurgeResponse{}, errors.New("wails: quarantine_root is required")
	}

	result, err := a.purgeSvc.ExecutePurge(context.Background(), app.PurgeExecuteInput{
		PlanID:         req.PlanID,
		Digest:         req.Digest,
		QuarantineRoot: req.QuarantineRoot,
		DryRun:         req.DryRun,
		Confirmation:   req.Confirmation,
	})
	if err != nil {
		return ExecutePurgeResponse{}, fmt.Errorf("wails: execute purge: %w", err)
	}
	return ExecutePurgeResponse{
		PlanID:     result.Result.PlanID,
		FinalState: string(result.Result.FinalState),
		Status:     string(result.Result.Status),
		ErrorType:  result.Result.ErrorType,
		Error:      errText(result.Result.Err),
	}, nil
}

// CheckRecoveryLock reports whether any operation plans are stuck in
// EXECUTING state, indicating a crash that needs recovery.
// Available in both read-only and read-write modes.
func (a *API) CheckRecoveryLock() (RecoveryStatusDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.store == nil {
		return RecoveryStatusDTO{}, ErrNoProjectOpen
	}
	return checkRecoveryLock(context.Background(), a.store)
}

type recoveryLockStore interface {
	ListExecutingPlans(context.Context) ([]string, error)
	ListPendingRestores(context.Context) ([]domain.RestoreJournalEntry, error)
	ListRecoverablePurges(context.Context) ([]domain.PurgeJournalEntry, error)
}

func checkRecoveryLock(ctx context.Context, st recoveryLockStore) (RecoveryStatusDTO, error) {
	ids, err := st.ListExecutingPlans(ctx)
	if err != nil {
		return RecoveryStatusDTO{}, fmt.Errorf("wails: check recovery lock: %w", err)
	}
	restores, err := st.ListPendingRestores(ctx)
	if err != nil {
		return RecoveryStatusDTO{}, fmt.Errorf("wails: check restore recovery lock: %w", err)
	}
	purges, err := st.ListRecoverablePurges(ctx)
	if err != nil {
		return RecoveryStatusDTO{}, fmt.Errorf("wails: check purge recovery lock: %w", err)
	}
	total := len(ids) + len(restores) + len(purges)
	return RecoveryStatusDTO{
		LockActive:            total > 0,
		ExecutingCount:        total,
		SourceExecutingCount:  len(ids),
		RestorePendingCount:   len(restores),
		PurgeRecoverableCount: len(purges),
	}, nil
}

// RecoverSourcePlans scans for plans stuck in EXECUTING state and brings
// them to a safe terminal state (rolled back or reset to APPROVED).
// Requires read-write mode.
func (a *API) RecoverSourcePlans() ([]RecoveryResultDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.recoverySvc == nil {
		return nil, ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return nil, ErrProjectNotReadWrite
	}
	results, err := a.recoverySvc.Recover(context.Background())
	if err != nil {
		return nil, fmt.Errorf("wails: recover source plans: %w", err)
	}
	out := make([]RecoveryResultDTO, len(results))
	for i, r := range results {
		out[i] = RecoveryResultDTO{
			PlanID:     r.PlanID,
			Action:     string(r.Action),
			RolledBack: r.RolledBack,
			Errors:     r.Errors,
		}
	}
	return out, nil
}

// RecoverRestores coordinates crash recovery for non-terminal restore
// operations. Requires read-write mode.
func (a *API) RecoverRestores(req RecoverRestoresRequest) ([]RestoreRecoveryResultDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.quarantineSvc == nil {
		return nil, ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return nil, ErrProjectNotReadWrite
	}
	if strings.TrimSpace(req.QuarantineRoot) == "" {
		return nil, errors.New("wails: quarantine_root is required")
	}
	results, err := a.quarantineSvc.RecoverRestores(context.Background(), app.RecoverRestoresInput{
		QuarantineRoot: req.QuarantineRoot,
		SourceRoots:    req.SourceRoots,
	})
	if err != nil {
		return nil, fmt.Errorf("wails: recover restores: %w", err)
	}
	out := make([]RestoreRecoveryResultDTO, len(results))
	for i, r := range results {
		out[i] = RestoreRecoveryResultDTO{
			PlanID:     r.PlanID,
			FinalState: string(r.FinalState),
			Status:     string(r.Status),
			ErrorType:  r.ErrorType,
			Error:      errText(r.Err),
		}
	}
	return out, nil
}

// RecoverPurges coordinates crash recovery for non-terminal purge
// operations. Requires read-write mode.
func (a *API) RecoverPurges(quarantineRoot string) ([]PurgeRecoveryResultDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.purgeSvc == nil {
		return nil, ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return nil, ErrProjectNotReadWrite
	}
	if strings.TrimSpace(quarantineRoot) == "" {
		return nil, errors.New("wails: quarantine_root is required")
	}
	results, err := a.purgeSvc.RecoverPurges(context.Background(), quarantineRoot)
	if err != nil {
		return nil, fmt.Errorf("wails: recover purges: %w", err)
	}
	out := make([]PurgeRecoveryResultDTO, len(results))
	for i, r := range results {
		out[i] = PurgeRecoveryResultDTO{
			PlanID:     r.PlanID,
			FinalState: string(r.FinalState),
			Status:     string(r.Status),
			ErrorType:  r.ErrorType,
			Error:      errText(r.Err),
		}
	}
	return out, nil
}

// ---- V8 audit bindings ----

// ListOperationLogs returns audit log entries for a specific plan.
// Pass an empty planID to return logs for all plans.
// Available in both read-only and read-write modes.
func (a *API) ListOperationLogs(planID string) ([]OperationLogDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.store == nil {
		return nil, ErrNoProjectOpen
	}
	logs, err := a.store.ListLogs(context.Background(), planID)
	if err != nil {
		return nil, fmt.Errorf("wails: list operation logs: %w", err)
	}
	out := make([]OperationLogDTO, len(logs))
	for i, l := range logs {
		out[i] = mapOperationLog(l)
	}
	return out, nil
}

// ListJournalEntries returns execution journal entries for a specific plan.
// Pass an empty planID to return entries for all plans.
// Available in both read-only and read-write modes.
func (a *API) ListJournalEntries(planID string) ([]JournalEntryDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.store == nil {
		return nil, ErrNoProjectOpen
	}
	entries, err := a.store.ListJournalAll(context.Background(), planID)
	if err != nil {
		return nil, fmt.Errorf("wails: list journal entries: %w", err)
	}
	out := make([]JournalEntryDTO, len(entries))
	for i, e := range entries {
		out[i] = mapJournalEntry(e)
	}
	return out, nil
}

// ---- V7.1 Plan execution bindings ----

// SaveDraftPlans generates draft governance plans from scanned files and
// persists them to the database as DRAFT state. Unlike BuildDraftPlans
// (which is read-only), this method creates a task record and saves plans
// so they can be subsequently approved and executed.
//
// Requires read-write mode. If a planning task already exists for the
// same storage, its plans are replaced (SavePlans deletes by task_id).
//
// If storageID is empty, all storages are included.
func (a *API) SaveDraftPlans(storageID string) ([]PlanDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.store == nil {
		return nil, ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return nil, ErrProjectNotReadWrite
	}
	if a.planSvc == nil {
		return nil, ErrNoProjectOpen
	}

	files, err := a.store.ListFiles(context.Background(), strings.TrimSpace(storageID))
	if err != nil {
		return nil, fmt.Errorf("wails: load files for planning: %w", err)
	}
	if len(files) == 0 {
		return []PlanDTO{}, nil
	}

	plans := a.planSvc.BuildPlans(context.Background(), files)
	if len(plans) == 0 {
		return []PlanDTO{}, nil
	}

	// Create a task record so FK constraints on operation_plans are
	// satisfied. The task ID is deterministic-ish but unique per call.
	taskID := fmt.Sprintf("plan-%d", time.Now().UnixNano())
	if err := a.store.CreateTask(context.Background(), domain.OperationTask{
		ID:        taskID,
		RootPath:  files[0].StorageID, // best-effort; used for grouping
		State:     "planning",
		CreatedAt: time.Now(),
	}); err != nil {
		return nil, fmt.Errorf("wails: create planning task: %w", err)
	}

	for i := range plans {
		plans[i].TaskID = taskID
	}

	if err := a.store.SavePlans(context.Background(), taskID, plans); err != nil {
		return nil, fmt.Errorf("wails: save draft plans: %w", err)
	}

	out := make([]PlanDTO, len(plans))
	for i, p := range plans {
		out[i] = mapPlan(p)
	}
	return out, nil
}

// ExecutePlans runs approved plans through the safe-operation pipeline.
// In dry-run mode, plans are validated (stale check, protection rules,
// scope validation) without filesystem writes. In real mode, files are
// moved to quarantine, journal is written, and quarantine items are
// registered for lifecycle management.
//
// Requires read-write mode. Plans must be in APPROVED state. The backend
// re-loads plans from the database by plan_ids — it does not trust
// frontend-supplied plan data.
func (a *API) ExecutePlans(req ExecutePlansRequest) (ExecutePlansResponse, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.store == nil {
		return ExecutePlansResponse{}, ErrNoProjectOpen
	}
	if a.scanRunner == nil {
		return ExecutePlansResponse{}, ErrProjectNotReadWrite
	}
	if a.executionSvc == nil {
		return ExecutePlansResponse{}, ErrNoProjectOpen
	}
	if len(req.PlanIDs) == 0 {
		return ExecutePlansResponse{}, errors.New("wails: at least one plan_id is required")
	}
	if strings.TrimSpace(req.QuarantineRoot) == "" {
		return ExecutePlansResponse{}, errors.New("wails: quarantine_root is required")
	}
	if len(req.SourceRoots) == 0 {
		return ExecutePlansResponse{}, errors.New("wails: at least one source root is required")
	}

	// Retention: default 720h (30d), minimum 24h.
	retentionHours := req.RetentionHours
	if retentionHours <= 0 {
		retentionHours = 720
	}
	if retentionHours < 24 {
		return ExecutePlansResponse{}, errors.New("wails: retention must be at least 24 hours")
	}
	retention := time.Duration(retentionHours) * time.Hour

	// Re-load all plans from DB and filter by requested plan_ids.
	// Only APPROVED plans are eligible for execution.
	allPlans, err := a.store.ListAllPlans(context.Background())
	if err != nil {
		return ExecutePlansResponse{}, fmt.Errorf("wails: load plans for execution: %w", err)
	}

	idSet := make(map[string]bool, len(req.PlanIDs))
	for _, id := range req.PlanIDs {
		idSet[id] = true
	}

	var eligible []domain.OperationPlan
	for _, p := range allPlans {
		if idSet[p.ID] && p.State == domain.PlanApproved {
			eligible = append(eligible, p)
		}
	}

	if len(eligible) == 0 {
		return ExecutePlansResponse{
			Results:  []ExecutePlanResultDTO{},
			Executed: 0,
			Skipped:  0,
			Failed:   0,
		}, nil
	}

	summary, err := a.executionSvc.Execute(context.Background(), app.ExecutionInput{
		Plans:          eligible,
		QuarantineRoot: req.QuarantineRoot,
		SourceRoots:    req.SourceRoots,
		DryRun:         req.DryRun,
		Retention:      retention,
	})
	if err != nil {
		return ExecutePlansResponse{}, fmt.Errorf("wails: execute plans: %w", err)
	}

	results := make([]ExecutePlanResultDTO, len(summary.Results))
	for i, r := range summary.Results {
		results[i] = mapExecutionResult(r)
	}

	return ExecutePlansResponse{
		Results:  results,
		Executed: summary.Executed,
		Skipped:  summary.Skipped,
		Failed:   summary.Failed,
	}, nil
}

// ---- V9 Capability & Readiness bindings ----

// GetAppCapabilities returns the real capability set derived from the
// backend's actual store/service state. This replaces the frontend's
// pure-inference deriveCapabilities() so that protection rules,
// recovery locks, and service availability are reflected accurately.
//
// Returns a minimal "closed" capability set when no project is open.
func (a *API) GetAppCapabilities() (AppCapabilitiesDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.store == nil {
		return AppCapabilitiesDTO{
			ProjectOpen:    false,
			ProjectMode:    "closed",
			CanViewResults: false,
			DisabledReasons: map[string]string{
				"scan-jobs":         "请先打开项目",
				"duplicate-results": "请先打开项目",
				"governance-review": "请先打开项目",
				"execution-center":  "请先打开项目",
				"audit-recovery":    "请先打开项目",
			},
		}, nil
	}

	isRW := a.scanRunner != nil

	// Check recovery lock from real backend state.
	recoveryLock := false
	lockStatus, err := checkRecoveryLock(context.Background(), a.store)
	if err == nil {
		recoveryLock = lockStatus.LockActive
	}

	mode := "read_only"
	if isRW {
		mode = "read_write"
	}

	disabled := map[string]string{}
	if !isRW {
		disabled["execution-center"] = "只读模式，无法执行写操作"
	}
	if recoveryLock {
		disabled["scan-jobs"] = "恢复锁激活中，请先处理未完成执行"
		disabled["execution-center"] = "恢复锁激活中，请先处理未完成执行"
	}

	return AppCapabilitiesDTO{
		ProjectOpen:          true,
		ProjectMode:          mode,
		CanScan:              isRW && !recoveryLock,
		CanViewResults:       true,
		CanEditReviews:       isRW && !recoveryLock,
		CanApprovePlans:      isRW && !recoveryLock,
		CanExecuteQuarantine: isRW && !recoveryLock,
		CanExecutePurge:      isRW && !recoveryLock,
		RecoveryLockActive:   recoveryLock,
		DisabledReasons:      disabled,
	}, nil
}

// GetProjectReadiness checks whether the project is ready for scanning
// and governance. It inspects storage entries, file counts, and plan
// state to produce a per-dimension checklist that the data source page
// displays.
//
// Returns an empty (not-ready) result when no project is open.
func (a *API) GetProjectReadiness() (ProjectReadinessDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.store == nil {
		return ProjectReadinessDTO{
			Ready: false,
			Checks: []ReadinessCheckDTO{
				{Key: "project_open", Label: "项目已打开", Passed: false, Reason: "请先打开项目"},
			},
		}, nil
	}

	ctx := context.Background()
	var checks []ReadinessCheckDTO

	// 1. Storage exists
	storages, err := a.store.ListStorages(ctx)
	if err != nil {
		return ProjectReadinessDTO{}, fmt.Errorf("wails: check storage readiness: %w", err)
	}
	storageOK := len(storages) > 0
	checks = append(checks, ReadinessCheckDTO{
		Key:    "has_storage",
		Label:  "已注册存储",
		Passed: storageOK,
		Reason: func() string {
			if !storageOK {
				return "请先扫描一个目录以注册存储"
			}
			return ""
		}(),
	})

	// 2. Files discovered
	files, err := a.store.ListFiles(ctx, "")
	if err != nil {
		return ProjectReadinessDTO{}, fmt.Errorf("wails: check file readiness: %w", err)
	}
	fileOK := len(files) > 0
	checks = append(checks, ReadinessCheckDTO{
		Key:    "has_files",
		Label:  "已发现文件",
		Passed: fileOK,
		Reason: func() string {
			if !fileOK {
				return "请先完成一次扫描"
			}
			return ""
		}(),
	})

	// 3. Read-write mode (required for new scans)
	isRW := a.scanRunner != nil
	checks = append(checks, ReadinessCheckDTO{
		Key:    "read_write",
		Label:  "读写模式",
		Passed: isRW,
		Reason: func() string {
			if !isRW {
				return "读写模式才能启动新扫描"
			}
			return ""
		}(),
	})

	// 4. Recovery lock inactive
	recoveryLock := false
	lockStatus, lockErr := checkRecoveryLock(ctx, a.store)
	if lockErr == nil {
		recoveryLock = lockStatus.LockActive
	}
	checks = append(checks, ReadinessCheckDTO{
		Key:    "no_recovery_lock",
		Label:  "恢复锁未激活",
		Passed: !recoveryLock,
		Reason: func() string {
			if recoveryLock {
				return "请先在执行中心处理未完成的执行计划"
			}
			return ""
		}(),
	})

	// 5. Plans available (optional — for governance, not scanning)
	plans, _ := a.store.ListAllPlans(ctx)
	planOK := len(plans) > 0
	checks = append(checks, ReadinessCheckDTO{
		Key:    "has_plans",
		Label:  "已有治理计划",
		Passed: planOK,
		Reason: func() string {
			if !planOK {
				return "可选 — 治理复核需要先生成草案"
			}
			return ""
		}(),
	})

	// Ready = first 4 checks pass (plans are optional)
	ready := storageOK && fileOK && isRW && !recoveryLock

	return ProjectReadinessDTO{
		Ready:        ready,
		Checks:       checks,
		StorageCount: len(storages),
		FileCount:    len(files),
		PlanCount:    len(plans),
	}, nil
}
