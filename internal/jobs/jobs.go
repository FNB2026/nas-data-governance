// Package jobs provides persistent background task management with a
// state machine, cancellation support, and crash recovery.
//
// Per ADR-0006 §2 (Layered Architecture), JobManager sits above the
// existing internal/runner semaphore executor. Runner remains an
// in-process concurrency limiter; JobManager adds:
//   - Persistent state (QUEUED → RUNNING → terminal) in job_runs table
//   - Structured event logging via internal/events
//   - Cancellation requests that propagate to the job's context
//   - Crash recovery: on restart, find non-terminal jobs and mark them
//     as FAILED or resume them
//   - Progress tracking with privacy-safe payloads
//
// Privacy: ProgressPayload and event payloads must NOT contain paths,
// filenames, or other location-identifying data (ADR-0006 §10).
package jobs

import (
	"context"
	"errors"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/events"
)

// ---------------------------------------------------------------------------
// State Machine
// ---------------------------------------------------------------------------

// JobState represents the lifecycle state of a job.
//
// Valid transitions:
//
//	QUEUED          → RUNNING | CANCELLED | FAILED
//	RUNNING         → CANCEL_REQUESTED | COMPLETED | FAILED
//	CANCEL_REQUESTED → CANCELLED | COMPLETED | FAILED
//	COMPLETED       (terminal)
//	FAILED          (terminal)
//	CANCELLED       (terminal)
type JobState string

const (
	StateQueued          JobState = "QUEUED"
	StateRunning         JobState = "RUNNING"
	StateCancelRequested JobState = "CANCEL_REQUESTED"
	StateCompleted       JobState = "COMPLETED"
	StateFailed          JobState = "FAILED"
	StateCancelled       JobState = "CANCELLED"
)

// IsTerminal returns true if no further state transitions are possible.
func (s JobState) IsTerminal() bool {
	return s == StateCompleted || s == StateFailed || s == StateCancelled
}

// validTransitions defines the legal state machine edges.
var validTransitions = map[JobState]map[JobState]bool{
	StateQueued: {
		StateRunning:   true,
		StateCancelled: true,
		StateFailed:    true, // crash recovery: QUEUED job never started
	},
	StateRunning: {
		StateCancelRequested: true,
		StateCompleted:       true,
		StateFailed:          true,
	},
	StateCancelRequested: {
		StateCancelled: true,
		StateCompleted: true,
		StateFailed:    true,
	},
}

// CanTransitionTo checks whether the transition from → to is legal.
func CanTransitionTo(from, to JobState) bool {
	if from.IsTerminal() {
		return false
	}
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

// ErrInvalidTransition is returned when a state transition is not allowed.
var ErrInvalidTransition = errors.New("jobs: invalid state transition")

// ---------------------------------------------------------------------------
// Job Stage
// ---------------------------------------------------------------------------

// JobStage represents the current processing stage within a running job.
// These map to the scan pipeline phases and other operation phases.
type JobStage string

const (
	StageDiscovering        JobStage = "DISCOVERING"
	StageMetadataIndexing   JobStage = "METADATA_INDEXING"
	StageQuickHashing       JobStage = "QUICK_HASHING"
	StageFullHashing        JobStage = "FULL_HASHING"
	StageContextClassifying JobStage = "CONTEXT_CLASSIFYING"
	StageFormatAnalyzing    JobStage = "FORMAT_ANALYZING"
	StageGrouping           JobStage = "GROUPING"
	StagePlanning           JobStage = "PLANNING"
	StageFinalizing         JobStage = "FINALIZING"
)

// ---------------------------------------------------------------------------
// Job Type
// ---------------------------------------------------------------------------

// JobType identifies the kind of background operation.
type JobType string

const (
	JobScan      JobType = "scan"
	JobAnalyze   JobType = "analyze"
	JobRelations JobType = "relations"
	JobPlan      JobType = "plan"
	JobExecute   JobType = "execute"
	JobRestore   JobType = "restore"
	JobPurge     JobType = "purge"
	JobLearn     JobType = "learn"
)

// ---------------------------------------------------------------------------
// Progress Payload
// ---------------------------------------------------------------------------

// ProgressPayload is the structured progress snapshot stored in
// progress_json and emitted as job:progress event payloads.
//
// Privacy: these are aggregate counters only — no paths, no filenames.
type ProgressPayload struct {
	Discovered int64 `json:"discovered,omitempty"`
	Processed  int64 `json:"processed,omitempty"`
	Failed     int64 `json:"failed,omitempty"`
	Total      int64 `json:"total,omitempty"`
	// BytesProcessed is optional, for stages that read file content.
	BytesProcessed int64 `json:"bytes_processed,omitempty"`
}

// ---------------------------------------------------------------------------
// Domain Types
// ---------------------------------------------------------------------------

// JobRun is the domain representation of a job_runs row.
type JobRun struct {
	ID              string          `json:"id"`
	ProjectID       string          `json:"project_id"`
	JobType         JobType         `json:"job_type"`
	State           JobState        `json:"state"`
	Stage           JobStage        `json:"stage"`
	CreatedAt       time.Time       `json:"created_at"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
	CancelRequested bool            `json:"cancel_requested"`
	Progress        ProgressPayload `json:"progress"`
	WarningCount    int             `json:"warning_count"`
	ErrorCode       string          `json:"error_code,omitempty"`
}

// ---------------------------------------------------------------------------
// Store Interface
// ---------------------------------------------------------------------------

// JobStore is the persistence port for job runs and events. The store
// package provides a concrete implementation backed by SQLite.
//
// Methods are grouped:
//   - Job lifecycle: Create, Get, UpdateState, UpdateProgress, IncrementWarnings, SetError
//   - Queries: ListByState, ListByProject
//   - Events: delegated to events.Recorder (embedded)
type JobStore interface {
	// CreateJob inserts a new job run. The ID, CreatedAt, State, and Stage
	// fields must be set by the caller.
	CreateJob(ctx context.Context, run JobRun) error

	// GetJob returns the job run by ID, or ErrNotFound if absent.
	GetJob(ctx context.Context, id string) (JobRun, error)

	// UpdateJobState transitions the job to a new state and stage.
	// Sets started_at on first RUNNING transition and completed_at on
	// any terminal transition. Returns ErrInvalidTransition if the
	// transition is not legal.
	UpdateJobState(ctx context.Context, id string, state JobState, stage JobStage) error

	// UpdateJobProgress persists a new progress snapshot.
	UpdateJobProgress(ctx context.Context, id string, progress ProgressPayload) error

	// IncrementJobWarnings atomically increments the warning count.
	IncrementJobWarnings(ctx context.Context, id string) error

	// SetJobError records the error code for a failed job.
	SetJobError(ctx context.Context, id string, errorCode string) error

	// ListJobsByState returns jobs matching any of the given states.
	// Used by crash recovery to find non-terminal jobs.
	ListJobsByState(ctx context.Context, states []JobState) ([]JobRun, error)

	// ListJobsByProject returns the most recent jobs for a project,
	// ordered by created_at DESC, limited to `limit` results.
	ListJobsByProject(ctx context.Context, projectID string, limit int) ([]JobRun, error)

	// Event recording is delegated to events.Recorder.
	events.Recorder
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ErrNotFound is returned when a single-job lookup has no match.
var ErrNotFound = errors.New("jobs: not found")

// ErrJobAlreadyRunning is returned when attempting to start a job that
// is already RUNNING.
var ErrJobAlreadyRunning = errors.New("jobs: job already running")

// ErrCancellationRequested is returned from a job function when it
// detects that cancellation was requested and stopped gracefully.
var ErrCancellationRequested = errors.New("jobs: cancellation requested")
