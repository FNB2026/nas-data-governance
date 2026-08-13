package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/events"
)

// JobFunc is the function executed by a running job. It receives a
// context that will be cancelled if cancellation is requested, and a
// Reporter for updating stage and progress.
//
// Return nil to complete the job successfully. Return any error to
// fail the job. Return ErrCancellationRequested to indicate graceful
// cancellation (the job will be marked CANCELLED, not FAILED).
type JobFunc func(ctx context.Context, reporter *Reporter) error

// Reporter allows a running job to update its stage, progress, and
// warning count. It is safe for concurrent use within a single job.
type Reporter struct {
	jobID string
	store JobStore
	mu    sync.Mutex
	stage JobStage
}

// SetStage transitions the job to a new processing stage and emits a
// job:stage event. The stage transition is informational (not a state
// machine transition) — the job remains RUNNING.
func (r *Reporter) SetStage(ctx context.Context, stage JobStage) error {
	r.mu.Lock()
	r.stage = stage
	r.mu.Unlock()
	if err := r.store.UpdateJobState(ctx, r.jobID, StateRunning, stage); err != nil {
		return fmt.Errorf("jobs: set stage %s: %w", stage, err)
	}
	_, err := r.store.AppendEvent(ctx, r.jobID, events.EventStage, string(stage), string(StateRunning), nil)
	return err
}

// SetProgress persists a new progress snapshot and emits a throttled
// job:progress event. The caller is responsible for throttling — this
// method does not rate-limit. The payload is sanitized before
// persistence.
func (r *Reporter) SetProgress(ctx context.Context, progress ProgressPayload) error {
	if err := r.store.UpdateJobProgress(ctx, r.jobID, progress); err != nil {
		return fmt.Errorf("jobs: update progress: %w", err)
	}
	r.mu.Lock()
	stage := r.stage
	r.mu.Unlock()
	payload := map[string]any{
		"discovered":      progress.Discovered,
		"processed":       progress.Processed,
		"failed":          progress.Failed,
		"total":           progress.Total,
		"bytes_processed": progress.BytesProcessed,
	}
	_, err := r.store.AppendEvent(ctx, r.jobID, events.EventProgress, string(stage), string(StateRunning),
		events.SanitizePayload(payload))
	return err
}

// Warn increments the warning counter and emits a job:warning event.
// The optional payload may carry a sanitized warning category or count.
func (r *Reporter) Warn(ctx context.Context, payload map[string]any) error {
	if err := r.store.IncrementJobWarnings(ctx, r.jobID); err != nil {
		return fmt.Errorf("jobs: increment warnings: %w", err)
	}
	r.mu.Lock()
	stage := r.stage
	r.mu.Unlock()
	_, err := r.store.AppendEvent(ctx, r.jobID, events.EventWarning, string(stage), string(StateRunning),
		events.SanitizePayload(payload))
	return err
}

// activeJob tracks an in-memory job for cancellation propagation.
type activeJob struct {
	cancel context.CancelFunc
}

// JobManager coordinates job lifecycle: creation, execution,
// cancellation, completion, failure, and crash recovery.
//
// It sits above internal/runner (which provides in-process concurrency
// limiting) and below the application services (which provide the
// actual business logic). The manager persists state to JobStore and
// records structured events via the store's embedded events.Recorder.
type JobManager struct {
	store JobStore

	mu     sync.Mutex
	active map[string]*activeJob // jobID → active job
}

// New creates a JobManager backed by the given store.
func New(store JobStore) *JobManager {
	return &JobManager{
		store:  store,
		active: make(map[string]*activeJob),
	}
}

// Create creates a new job in QUEUED state and emits a job:created event.
// Returns the generated job ID.
func (m *JobManager) Create(ctx context.Context, projectID string, jobType JobType) (string, error) {
	id := generateJobID()
	now := time.Now().UTC()
	run := JobRun{
		ID:        id,
		ProjectID: projectID,
		JobType:   jobType,
		State:     StateQueued,
		Stage:     StageDiscovering,
		CreatedAt: now,
	}
	if err := m.store.CreateJob(ctx, run); err != nil {
		return "", fmt.Errorf("jobs: create: %w", err)
	}
	_, err := m.store.AppendEvent(ctx, id, events.EventCreated, string(StageDiscovering), string(StateQueued), map[string]any{
		"job_type":   string(jobType),
		"project_id": projectID,
	})
	if err != nil {
		return "", fmt.Errorf("jobs: emit created event: %w", err)
	}
	return id, nil
}

// Run starts a QUEUED job and executes the provided function in the
// current goroutine. The function receives a context that will be
// cancelled if RequestCancel is called for this job, and a Reporter
// for updating stage and progress.
//
// On return:
//   - nil error           → COMPLETED + job:completed event
//   - ErrCancellationRequested → CANCELLED + job:cancelled event
//   - ErrNetworkPaused    → PAUSED_NETWORK + job:paused_network event
//   - any other error     → FAILED + job:failed event
//
// This method blocks until the job function returns. For async
// execution, call it in a goroutine.
func (m *JobManager) Run(ctx context.Context, jobID string, fn JobFunc) error {
	// Reserve the in-memory execution slot before changing persistent state.
	// This closes the race where two callers could both transition the same
	// job to RUNNING and overwrite each other's cancellation handle.
	jobCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	if _, exists := m.active[jobID]; exists {
		m.mu.Unlock()
		cancel()
		return ErrJobAlreadyRunning
	}
	m.active[jobID] = &activeJob{cancel: cancel}

	// Keep the manager lock through the short persistent transition so a
	// cancellation request cannot observe an active-but-still-QUEUED job.
	if err := m.store.UpdateJobState(ctx, jobID, StateRunning, StageDiscovering); err != nil {
		delete(m.active, jobID)
		m.mu.Unlock()
		cancel()
		return fmt.Errorf("jobs: start %s: %w", jobID, err)
	}
	m.mu.Unlock()

	defer func() {
		cancel()
		m.mu.Lock()
		delete(m.active, jobID)
		m.mu.Unlock()
	}()

	_, err := m.store.AppendEvent(ctx, jobID, events.EventStage, string(StageDiscovering), string(StateRunning), nil)
	if err != nil {
		return fmt.Errorf("jobs: emit running event: %w", err)
	}

	reporter := &Reporter{jobID: jobID, store: m.store, stage: StageDiscovering}

	// Execute the job function.
	jobErr := fn(jobCtx, reporter)

	// Determine final state.
	finalState := StateCompleted
	eventType := events.EventCompleted
	if jobErr != nil {
		if errors.Is(jobErr, ErrCancellationRequested) || errors.Is(jobErr, context.Canceled) {
			finalState = StateCancelled
			eventType = events.EventCancelled
		} else if errors.Is(jobErr, ErrNetworkPaused) {
			finalState = StatePausedNetwork
			eventType = events.EventPausedNetwork
		} else {
			finalState = StateFailed
			eventType = events.EventFailed
		}
	}

	// Transition to terminal state.
	reporter.mu.Lock()
	finalStage := reporter.stage
	reporter.mu.Unlock()

	if err := m.store.UpdateJobState(ctx, jobID, finalState, finalStage); err != nil {
		return fmt.Errorf("jobs: transition to %s: %w (job error: %v)", finalState, err, jobErr)
	}

	// Record error code for failed jobs.
	if finalState == StateFailed && jobErr != nil {
		_ = m.store.SetJobError(ctx, jobID, jobErr.Error())
	}

	// Emit terminal event.
	payload := map[string]any{}
	if finalState == StateFailed {
		payload["error_code"] = "job_failed"
	}
	_, emitErr := m.store.AppendEvent(ctx, jobID, eventType, string(finalStage), string(finalState),
		events.SanitizePayload(payload))
	if emitErr != nil {
		return fmt.Errorf("jobs: emit %s event: %w (job error: %v)", eventType, emitErr, jobErr)
	}

	// Return the original error (if any) to the caller.
	return jobErr
}

// RequestCancel requests cancellation of a running job. The job's
// context will be cancelled, causing the job function to return.
// The state transitions to CANCEL_REQUESTED immediately; it becomes
// CANCELLED when the job function actually returns.
//
// If the job is not running, this is a no-op.
func (m *JobManager) RequestCancel(ctx context.Context, jobID string) error {
	m.mu.Lock()
	aj, ok := m.active[jobID]
	m.mu.Unlock()

	if !ok {
		return nil // not running, nothing to cancel
	}

	// Transition to CANCEL_REQUESTED, preserving the current stage.
	// If the transition fails (e.g., already terminal), still cancel
	// the context to unblock the job function.
	current, err := m.store.GetJob(ctx, jobID)
	if err == nil {
		_ = m.store.UpdateJobState(ctx, jobID, StateCancelRequested, current.Stage)
	}

	// Cancel the context.
	aj.cancel()
	return nil
}

// Get returns the current state of a job.
func (m *JobManager) Get(ctx context.Context, jobID string) (JobRun, error) {
	return m.store.GetJob(ctx, jobID)
}

// ListEvents returns the event history for a job, ordered by sequence.
func (m *JobManager) ListEvents(ctx context.Context, jobID string) ([]events.Event, error) {
	return m.store.ListEvents(ctx, jobID)
}

// ListRecent returns the most recent jobs for a project.
func (m *JobManager) ListRecent(ctx context.Context, projectID string, limit int) ([]JobRun, error) {
	return m.store.ListJobsByProject(ctx, projectID, limit)
}

// Recover finds non-terminal jobs (QUEUED, RUNNING, CANCEL_REQUESTED)
// and marks them as FAILED with error_code "crash_recovery". This
// should be called once at application startup, before any new jobs
// are started.
//
// Returns the IDs of jobs that were recovered.
func (m *JobManager) Recover(ctx context.Context) ([]string, error) {
	nonTerminal := []JobState{StateQueued, StateRunning, StateCancelRequested}
	jobs, err := m.store.ListJobsByState(ctx, nonTerminal)
	if err != nil {
		return nil, fmt.Errorf("jobs: recover: list non-terminal: %w", err)
	}

	var recovered []string
	for _, j := range jobs {
		// Determine the current stage to preserve it in the terminal record.
		if err := m.store.UpdateJobState(ctx, j.ID, StateFailed, j.Stage); err != nil {
			// If the transition fails (e.g., already terminal from a
			// concurrent recovery), skip this job.
			continue
		}
		_ = m.store.SetJobError(ctx, j.ID, "crash_recovery")
		_, _ = m.store.AppendEvent(ctx, j.ID, events.EventFailed, string(j.Stage), string(StateFailed), map[string]any{
			"error_code": "crash_recovery",
		})
		recovered = append(recovered, j.ID)
	}
	return recovered, nil
}

// IsRunning returns true if the job is currently executing (in the
// active map). This is an in-memory check; it does not query the store.
func (m *JobManager) IsRunning(jobID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.active[jobID]
	return ok
}

// generateJobID creates a random 16-byte hex-encoded job ID.
// Using crypto/rand ensures IDs are not predictable, which matters
// because job IDs may appear in logs and event streams.
func generateJobID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID (should never happen in practice).
		return fmt.Sprintf("job-%d", time.Now().UnixNano())
	}
	return "job-" + hex.EncodeToString(b)
}
