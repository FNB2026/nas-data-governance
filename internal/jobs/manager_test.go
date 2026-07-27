package jobs_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/events"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// newTestManager creates a JobManager backed by a fresh in-memory SQLite
// database, cleaned up automatically when the test ends.
func newTestManager(t *testing.T) (*jobs.JobManager, *store.SQLiteStore) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return jobs.New(st), st
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestCreate_PersistsQueuedJobAndEvent(t *testing.T) {
	mgr, st := newTestManager(t)
	ctx := context.Background()

	jobID, err := mgr.Create(ctx, "proj-1", jobs.JobScan)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify job run is persisted in QUEUED state.
	run, err := st.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if run.State != jobs.StateQueued {
		t.Errorf("State: expected QUEUED, got %s", run.State)
	}
	if run.Stage != jobs.StageDiscovering {
		t.Errorf("Stage: expected DISCOVERING, got %s", run.Stage)
	}
	if run.ProjectID != "proj-1" {
		t.Errorf("ProjectID: expected proj-1, got %s", run.ProjectID)
	}
	if run.JobType != jobs.JobScan {
		t.Errorf("JobType: expected scan, got %s", run.JobType)
	}
	if run.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}

	// Verify job:created event was emitted.
	evts, err := st.ListEvents(ctx, jobID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
	if evts[0].EventType != events.EventCreated {
		t.Errorf("event type: expected job:created, got %s", evts[0].EventType)
	}
	if evts[0].Sequence != 1 {
		t.Errorf("sequence: expected 1, got %d", evts[0].Sequence)
	}
}

// ---------------------------------------------------------------------------
// Run — successful completion
// ---------------------------------------------------------------------------

func TestRun_SuccessfulCompletion(t *testing.T) {
	mgr, st := newTestManager(t)
	ctx := context.Background()

	jobID, err := mgr.Create(ctx, "proj-1", jobs.JobScan)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Run a job that completes successfully, updating stage and progress.
	err = mgr.Run(ctx, jobID, func(ctx context.Context, r *jobs.Reporter) error {
		if err := r.SetStage(ctx, jobs.StageQuickHashing); err != nil {
			return err
		}
		if err := r.SetProgress(ctx, jobs.ProgressPayload{Discovered: 100, Processed: 50}); err != nil {
			return err
		}
		if err := r.SetStage(ctx, jobs.StageFullHashing); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify final state is COMPLETED.
	run, err := st.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if run.State != jobs.StateCompleted {
		t.Errorf("State: expected COMPLETED, got %s", run.State)
	}
	if run.Stage != jobs.StageFullHashing {
		t.Errorf("Stage: expected FULL_HASHING, got %s", run.Stage)
	}
	if run.StartedAt == nil {
		t.Error("StartedAt should be set")
	}
	if run.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}

	// Verify events: created → stage(running) → stage → progress → stage → completed.
	evts, err := st.ListEvents(ctx, jobID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	// Expected event sequence:
	// 1. job:created (from Create)
	// 2. job:stage (RUNNING, from Run start)
	// 3. job:stage (QUICK_HASHING, from SetStage)
	// 4. job:progress (from SetProgress)
	// 5. job:stage (FULL_HASHING, from SetStage)
	// 6. job:completed (from Run completion)
	if len(evts) != 6 {
		t.Fatalf("expected 6 events, got %d", len(evts))
	}
	expectedTypes := []events.EventType{
		events.EventCreated,
		events.EventStage,
		events.EventStage,
		events.EventProgress,
		events.EventStage,
		events.EventCompleted,
	}
	for i, et := range expectedTypes {
		if evts[i].EventType != et {
			t.Errorf("event %d: expected %s, got %s", i, et, evts[i].EventType)
		}
		if evts[i].Sequence != i+1 {
			t.Errorf("event %d: expected sequence %d, got %d", i, i+1, evts[i].Sequence)
		}
	}
}

// ---------------------------------------------------------------------------
// Run — failure
// ---------------------------------------------------------------------------

func TestRun_Failure(t *testing.T) {
	mgr, st := newTestManager(t)
	ctx := context.Background()

	jobID, err := mgr.Create(ctx, "proj-1", jobs.JobScan)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	jobErr := errors.New("disk full")
	err = mgr.Run(ctx, jobID, func(ctx context.Context, r *jobs.Reporter) error {
		_ = r.SetStage(ctx, jobs.StageQuickHashing)
		return jobErr
	})
	if !errors.Is(err, jobErr) {
		t.Fatalf("Run should return the job error, got %v", err)
	}

	run, err := st.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if run.State != jobs.StateFailed {
		t.Errorf("State: expected FAILED, got %s", run.State)
	}
	if run.ErrorCode == "" {
		t.Error("ErrorCode should be set for failed jobs")
	}
	if run.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}

	// Verify job:failed event.
	evts, _ := st.ListEvents(ctx, jobID)
	if len(evts) < 1 {
		t.Fatal("expected at least 1 event")
	}
	last := evts[len(evts)-1]
	if last.EventType != events.EventFailed {
		t.Errorf("last event: expected job:failed, got %s", last.EventType)
	}
}

// ---------------------------------------------------------------------------
// Cancellation
// ---------------------------------------------------------------------------

func TestRequestCancel_GracefulCancellation(t *testing.T) {
	mgr, st := newTestManager(t)
	ctx := context.Background()

	jobID, _ := mgr.Create(ctx, "proj-1", jobs.JobScan)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = mgr.Run(ctx, jobID, func(ctx context.Context, r *jobs.Reporter) error {
			// Wait for cancellation.
			<-ctx.Done()
			return jobs.ErrCancellationRequested
		})
	}()

	// Wait for the job to be running.
	waitFor(t, func() bool { return mgr.IsRunning(jobID) }, 2*time.Second)

	// Request cancellation.
	if err := mgr.RequestCancel(ctx, jobID); err != nil {
		t.Fatalf("RequestCancel: %v", err)
	}

	wg.Wait()

	run, err := st.GetJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if run.State != jobs.StateCancelled {
		t.Errorf("State: expected CANCELLED, got %s", run.State)
	}

	evts, _ := st.ListEvents(ctx, jobID)
	last := evts[len(evts)-1]
	if last.EventType != events.EventCancelled {
		t.Errorf("last event: expected job:cancelled, got %s", last.EventType)
	}
}

func TestRequestCancel_ContextCanceled(t *testing.T) {
	mgr, st := newTestManager(t)
	ctx := context.Background()

	jobID, _ := mgr.Create(ctx, "proj-1", jobs.JobScan)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = mgr.Run(ctx, jobID, func(ctx context.Context, r *jobs.Reporter) error {
			<-ctx.Done()
			return ctx.Err() // context.Canceled
		})
	}()

	waitFor(t, func() bool { return mgr.IsRunning(jobID) }, 2*time.Second)
	mgr.RequestCancel(ctx, jobID)
	wg.Wait()

	run, _ := st.GetJob(ctx, jobID)
	if run.State != jobs.StateCancelled {
		t.Errorf("State: expected CANCELLED, got %s", run.State)
	}
}

func TestRequestCancel_NotRunning_NoOp(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	// Requesting cancel on a non-running job should be a no-op.
	err := mgr.RequestCancel(ctx, "nonexistent-job")
	if err != nil {
		t.Errorf("RequestCancel on non-running job should be no-op, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Progress and Warnings
// ---------------------------------------------------------------------------

func TestSetProgress_PersistsProgressPayload(t *testing.T) {
	mgr, st := newTestManager(t)
	ctx := context.Background()

	jobID, _ := mgr.Create(ctx, "proj-1", jobs.JobScan)

	_ = mgr.Run(ctx, jobID, func(ctx context.Context, r *jobs.Reporter) error {
		return r.SetProgress(ctx, jobs.ProgressPayload{
			Discovered: 1000,
			Processed:  500,
			Failed:     10,
		})
	})

	run, _ := st.GetJob(ctx, jobID)
	if run.Progress.Discovered != 1000 {
		t.Errorf("Progress.Discovered: expected 1000, got %d", run.Progress.Discovered)
	}
	if run.Progress.Processed != 500 {
		t.Errorf("Progress.Processed: expected 500, got %d", run.Progress.Processed)
	}
	if run.Progress.Failed != 10 {
		t.Errorf("Progress.Failed: expected 10, got %d", run.Progress.Failed)
	}
}

func TestWarn_IncrementsWarningCount(t *testing.T) {
	mgr, st := newTestManager(t)
	ctx := context.Background()

	jobID, _ := mgr.Create(ctx, "proj-1", jobs.JobScan)

	_ = mgr.Run(ctx, jobID, func(ctx context.Context, r *jobs.Reporter) error {
		_ = r.Warn(ctx, map[string]any{"category": "hash_failure"})
		_ = r.Warn(ctx, map[string]any{"category": "hash_failure"})
		return nil
	})

	run, _ := st.GetJob(ctx, jobID)
	if run.WarningCount != 2 {
		t.Errorf("WarningCount: expected 2, got %d", run.WarningCount)
	}

	// Verify job:warning events.
	evts, _ := st.ListEvents(ctx, jobID)
	warningCount := 0
	for _, e := range evts {
		if e.EventType == events.EventWarning {
			warningCount++
		}
	}
	if warningCount != 2 {
		t.Errorf("warning events: expected 2, got %d", warningCount)
	}
}

// ---------------------------------------------------------------------------
// Crash Recovery
// ---------------------------------------------------------------------------

func TestRecover_MarksNonTerminalJobsAsFailed(t *testing.T) {
	mgr, st := newTestManager(t)
	ctx := context.Background()

	// Create a QUEUED job (never started).
	id1, _ := mgr.Create(ctx, "proj-1", jobs.JobScan)

	// Create and start a RUNNING job, then simulate a crash by not
	// completing it.
	id2, _ := mgr.Create(ctx, "proj-1", jobs.JobScan)
	_ = st.UpdateJobState(ctx, id2, jobs.StateRunning, jobs.StageQuickHashing)

	// Create a completed job — should NOT be recovered.
	id3, _ := mgr.Create(ctx, "proj-1", jobs.JobScan)
	_ = st.UpdateJobState(ctx, id3, jobs.StateRunning, jobs.StageDiscovering)
	_ = st.UpdateJobState(ctx, id3, jobs.StateCompleted, jobs.StageFinalizing)

	// Run recovery.
	recovered, err := mgr.Recover(ctx)
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if len(recovered) != 2 {
		t.Fatalf("expected 2 recovered jobs, got %d", len(recovered))
	}

	// Verify both non-terminal jobs are now FAILED.
	run1, _ := st.GetJob(ctx, id1)
	if run1.State != jobs.StateFailed {
		t.Errorf("job 1 state: expected FAILED, got %s", run1.State)
	}
	if run1.ErrorCode != "crash_recovery" {
		t.Errorf("job 1 error code: expected crash_recovery, got %s", run1.ErrorCode)
	}

	run2, _ := st.GetJob(ctx, id2)
	if run2.State != jobs.StateFailed {
		t.Errorf("job 2 state: expected FAILED, got %s", run2.State)
	}

	// Completed job should remain COMPLETED.
	run3, _ := st.GetJob(ctx, id3)
	if run3.State != jobs.StateCompleted {
		t.Errorf("job 3 state: expected COMPLETED, got %s", run3.State)
	}
}

func TestRecover_NoNonTerminalJobs(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	recovered, err := mgr.Recover(ctx)
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if len(recovered) != 0 {
		t.Errorf("expected 0 recovered jobs, got %d", len(recovered))
	}
}

// ---------------------------------------------------------------------------
// List Recent
// ---------------------------------------------------------------------------

func TestListRecent_ReturnsJobsByProject(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	// Create jobs for different projects.
	id1, _ := mgr.Create(ctx, "proj-A", jobs.JobScan)
	id2, _ := mgr.Create(ctx, "proj-A", jobs.JobAnalyze)
	id3, _ := mgr.Create(ctx, "proj-B", jobs.JobScan)

	// List recent jobs for proj-A.
	recent, err := mgr.ListRecent(ctx, "proj-A", 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2 jobs for proj-A, got %d", len(recent))
	}

	// Verify they're the right jobs (ordered by created_at DESC, so id2 first).
	if recent[0].ID != id2 {
		t.Errorf("first job: expected %s, got %s", id2, recent[0].ID)
	}
	if recent[1].ID != id1 {
		t.Errorf("second job: expected %s, got %s", id1, recent[1].ID)
	}

	// Verify proj-B has only 1 job.
	recentB, _ := mgr.ListRecent(ctx, "proj-B", 10)
	if len(recentB) != 1 {
		t.Errorf("expected 1 job for proj-B, got %d", len(recentB))
	}
	if recentB[0].ID != id3 {
		t.Errorf("expected %s, got %s", id3, recentB[0].ID)
	}
}

// ---------------------------------------------------------------------------
// List Events
// ---------------------------------------------------------------------------

func TestListEvents_OrderedBySequence(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	jobID, _ := mgr.Create(ctx, "proj-1", jobs.JobScan)

	_ = mgr.Run(ctx, jobID, func(ctx context.Context, r *jobs.Reporter) error {
		_ = r.SetStage(ctx, jobs.StageQuickHashing)
		_ = r.SetStage(ctx, jobs.StageFullHashing)
		_ = r.SetProgress(ctx, jobs.ProgressPayload{Discovered: 10})
		return nil
	})

	evts, err := mgr.ListEvents(ctx, jobID)
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}

	// Verify strictly increasing sequence.
	for i := 0; i < len(evts); i++ {
		if evts[i].Sequence != i+1 {
			t.Errorf("event %d: expected sequence %d, got %d", i, i+1, evts[i].Sequence)
		}
	}
}

// ---------------------------------------------------------------------------
// Privacy Sanitization
// ---------------------------------------------------------------------------

func TestEventPayload_SanitizesSensitiveKeys(t *testing.T) {
	mgr, st := newTestManager(t)
	ctx := context.Background()

	jobID, _ := mgr.Create(ctx, "proj-1", jobs.JobScan)

	_ = mgr.Run(ctx, jobID, func(ctx context.Context, r *jobs.Reporter) error {
		// Attempt to put sensitive data in a warning payload.
		return r.Warn(ctx, map[string]any{
			"category": "hash_failure",
			"path":     "/secret/path/to/file",
			"filename": "secret.txt",
			"error":    "open /secret: denied",
		})
	})

	// Verify the persisted event payload does not contain sensitive keys.
	evts, _ := st.ListEvents(ctx, jobID)
	for _, e := range evts {
		if e.EventType != events.EventWarning {
			continue
		}
		if _, ok := e.Payload["path"]; ok {
			t.Error("warning event payload should not contain 'path'")
		}
		if _, ok := e.Payload["filename"]; ok {
			t.Error("warning event payload should not contain 'filename'")
		}
		if _, ok := e.Payload["error"]; ok {
			t.Error("warning event payload should not contain 'error'")
		}
		// Safe key should be preserved.
		if _, ok := e.Payload["category"]; !ok {
			t.Error("warning event payload should contain 'category'")
		}
	}
}

func TestProgressPayload_NoSensitiveData(t *testing.T) {
	mgr, st := newTestManager(t)
	ctx := context.Background()

	jobID, _ := mgr.Create(ctx, "proj-1", jobs.JobScan)

	_ = mgr.Run(ctx, jobID, func(ctx context.Context, r *jobs.Reporter) error {
		return r.SetProgress(ctx, jobs.ProgressPayload{
			Discovered: 42,
			Processed:  10,
		})
	})

	// Verify the progress_json in the database contains only counters.
	run, _ := st.GetJob(ctx, jobID)
	if run.Progress.Discovered != 42 {
		t.Errorf("Discovered: expected 42, got %d", run.Progress.Discovered)
	}
	if run.Progress.Processed != 10 {
		t.Errorf("Processed: expected 10, got %d", run.Progress.Processed)
	}

	// Verify the job:progress event payload also has only counters.
	evts, _ := st.ListEvents(ctx, jobID)
	for _, e := range evts {
		if e.EventType != events.EventProgress {
			continue
		}
		if _, ok := e.Payload["path"]; ok {
			t.Error("progress event payload should not contain 'path'")
		}
		if _, ok := e.Payload["discovered"]; !ok {
			t.Error("progress event payload should contain 'discovered'")
		}
	}
}

// ---------------------------------------------------------------------------
// State Machine Validation
// ---------------------------------------------------------------------------

func TestCanTransitionTo_LegalTransitions(t *testing.T) {
	tests := []struct {
		from, to jobs.JobState
		want     bool
	}{
		{jobs.StateQueued, jobs.StateRunning, true},
		{jobs.StateQueued, jobs.StateCancelled, true},
		{jobs.StateQueued, jobs.StateCompleted, false},
		{jobs.StateQueued, jobs.StateFailed, true},
		{jobs.StateRunning, jobs.StateCancelRequested, true},
		{jobs.StateRunning, jobs.StateCompleted, true},
		{jobs.StateRunning, jobs.StateFailed, true},
		{jobs.StateRunning, jobs.StateQueued, false},
		{jobs.StateCancelRequested, jobs.StateCancelled, true},
		{jobs.StateCancelRequested, jobs.StateCompleted, true},
		{jobs.StateCancelRequested, jobs.StateFailed, true},
		{jobs.StateCancelRequested, jobs.StateRunning, false},
		{jobs.StateCompleted, jobs.StateRunning, false},
		{jobs.StateFailed, jobs.StateRunning, false},
		{jobs.StateCancelled, jobs.StateRunning, false},
	}
	for _, tt := range tests {
		got := jobs.CanTransitionTo(tt.from, tt.to)
		if got != tt.want {
			t.Errorf("CanTransitionTo(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	if !jobs.StateCompleted.IsTerminal() {
		t.Error("COMPLETED should be terminal")
	}
	if !jobs.StateFailed.IsTerminal() {
		t.Error("FAILED should be terminal")
	}
	if !jobs.StateCancelled.IsTerminal() {
		t.Error("CANCELLED should be terminal")
	}
	if jobs.StateQueued.IsTerminal() {
		t.Error("QUEUED should not be terminal")
	}
	if jobs.StateRunning.IsTerminal() {
		t.Error("RUNNING should not be terminal")
	}
	if jobs.StateCancelRequested.IsTerminal() {
		t.Error("CANCEL_REQUESTED should not be terminal")
	}
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

func TestGet_ReturnsJobRun(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	jobID, _ := mgr.Create(ctx, "proj-1", jobs.JobScan)

	run, err := mgr.Get(ctx, jobID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if run.ID != jobID {
		t.Errorf("ID: expected %s, got %s", jobID, run.ID)
	}
}

func TestGet_NotFoundReturnsError(t *testing.T) {
	mgr, _ := newTestManager(t)
	ctx := context.Background()

	_, err := mgr.Get(ctx, "nonexistent")
	if err == nil {
		t.Error("Get should return error for nonexistent job")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// waitFor polls fn until it returns true or the timeout expires.
func waitFor(t *testing.T, fn func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("waitFor: condition not met within timeout")
}
