package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/events"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
)

// Compile-time assertions: SQLiteStore must implement jobs.JobStore.
var _ jobs.JobStore = (*SQLiteStore)(nil)

// CreateJob inserts a new job run into job_runs.
func (s *SQLiteStore) CreateJob(ctx context.Context, run jobs.JobRun) error {
	progressJSON, err := json.Marshal(run.Progress)
	if err != nil {
		return fmt.Errorf("store: CreateJob: marshal progress: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO job_runs(id, project_id, job_type, state, stage, created_at, started_at, completed_at, cancel_requested, progress_json, warning_count, error_code)
		 VALUES(?, ?, ?, ?, ?, ?, NULL, NULL, 0, ?, 0, '')`,
		run.ID, run.ProjectID, string(run.JobType),
		string(run.State), string(run.Stage),
		run.CreatedAt.UTC().Format(time.RFC3339Nano),
		string(progressJSON),
	)
	if err != nil {
		return fmt.Errorf("store: CreateJob: %w", err)
	}
	return nil
}

// GetJob returns the job run by ID.
func (s *SQLiteStore) GetJob(ctx context.Context, id string) (jobs.JobRun, error) {
	return s.scanJobRow(ctx, `SELECT id, project_id, job_type, state, stage, created_at, started_at, completed_at, cancel_requested, progress_json, warning_count, error_code
		FROM job_runs WHERE id = ?`, id)
}

// UpdateJobState transitions the job to a new state and stage.
// Sets started_at on first RUNNING transition and completed_at on
// any terminal transition. Validates the transition using the state
// machine defined in jobs.CanTransitionTo.
func (s *SQLiteStore) UpdateJobState(ctx context.Context, id string, state jobs.JobState, stage jobs.JobStage) error {
	// Load current state for transition validation.
	current, err := s.GetJob(ctx, id)
	if err != nil {
		return fmt.Errorf("store: UpdateJobState: load current: %w", err)
	}

	if !jobs.CanTransitionTo(current.State, state) {
		// Special case: the JobManager's Run() may call UpdateJobState
		// with StateRunning when the job is already RUNNING (e.g., for
		// a stage-only update from Reporter.SetStage). Allow this.
		if current.State == state {
			// Same-state update (stage change only) is allowed.
		} else {
			return fmt.Errorf("%w: %s → %s", jobs.ErrInvalidTransition, current.State, state)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	var startedAt, completedAt any

	switch {
	case state == jobs.StateRunning && current.StartedAt == nil:
		startedAt = now
	case state.IsTerminal():
		startedAt = nullIfNil(current.StartedAt)
		completedAt = now
	default:
		startedAt = nullIfNil(current.StartedAt)
		completedAt = nullIfNil(current.CompletedAt)
	}

	cancelRequested := 0
	if state == jobs.StateCancelRequested {
		cancelRequested = 1
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE job_runs SET state = ?, stage = ?, started_at = ?, completed_at = ?, cancel_requested = ? WHERE id = ?`,
		string(state), string(stage), startedAt, completedAt, cancelRequested, id)
	if err != nil {
		return fmt.Errorf("store: UpdateJobState: %w", err)
	}
	return nil
}

// UpdateJobProgress persists a new progress snapshot.
func (s *SQLiteStore) UpdateJobProgress(ctx context.Context, id string, progress jobs.ProgressPayload) error {
	progressJSON, err := json.Marshal(progress)
	if err != nil {
		return fmt.Errorf("store: UpdateJobProgress: marshal: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE job_runs SET progress_json = ? WHERE id = ?`,
		string(progressJSON), id)
	if err != nil {
		return fmt.Errorf("store: UpdateJobProgress: %w", err)
	}
	return nil
}

// IncrementJobWarnings atomically increments the warning count.
func (s *SQLiteStore) IncrementJobWarnings(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE job_runs SET warning_count = warning_count + 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: IncrementJobWarnings: %w", err)
	}
	return nil
}

// SetJobError records the error code for a failed job.
func (s *SQLiteStore) SetJobError(ctx context.Context, id string, errorCode string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE job_runs SET error_code = ? WHERE id = ?`, errorCode, id)
	if err != nil {
		return fmt.Errorf("store: SetJobError: %w", err)
	}
	return nil
}

// ListJobsByState returns jobs matching any of the given states.
func (s *SQLiteStore) ListJobsByState(ctx context.Context, states []jobs.JobState) ([]jobs.JobRun, error) {
	if len(states) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(states))
	args := make([]any, len(states))
	for i, st := range states {
		placeholders[i] = "?"
		args[i] = string(st)
	}
	q := fmt.Sprintf(`SELECT id, project_id, job_type, state, stage, created_at, started_at, completed_at, cancel_requested, progress_json, warning_count, error_code
		FROM job_runs WHERE state IN (%s) ORDER BY created_at ASC`, strings.Join(placeholders, ","))
	return s.scanJobRows(ctx, q, args...)
}

// ListJobsByProject returns the most recent jobs for a project.
func (s *SQLiteStore) ListJobsByProject(ctx context.Context, projectID string, limit int) ([]jobs.JobRun, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.scanJobRows(ctx,
		`SELECT id, project_id, job_type, state, stage, created_at, started_at, completed_at, cancel_requested, progress_json, warning_count, error_code
		FROM job_runs WHERE project_id = ? ORDER BY created_at DESC LIMIT ?`,
		projectID, limit)
}

// AppendEvent persists a new event for the given job, assigning the
// next sequence number atomically within a transaction.
func (s *SQLiteStore) AppendEvent(ctx context.Context, jobID string, eventType events.EventType, stage, state string, payload map[string]any) (events.Event, error) {
	sanitized := events.SanitizePayload(payload)
	payloadJSON, err := json.Marshal(sanitized)
	if err != nil {
		return events.Event{}, fmt.Errorf("store: AppendEvent: marshal payload: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return events.Event{}, fmt.Errorf("store: AppendEvent: begin: %w", err)
	}
	defer tx.Rollback()

	// Get the next sequence number.
	var seq int
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sequence), 0) + 1 FROM job_events WHERE job_id = ?`, jobID).Scan(&seq)
	if err != nil {
		return events.Event{}, fmt.Errorf("store: AppendEvent: next sequence: %w", err)
	}

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx,
		`INSERT INTO job_events(job_id, sequence, event_type, stage, state, payload_json, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?)`,
		jobID, seq, string(eventType), stage, state, string(payloadJSON),
		now.Format(time.RFC3339Nano))
	if err != nil {
		return events.Event{}, fmt.Errorf("store: AppendEvent: insert: %w", err)
	}

	id, _ := result.LastInsertId()
	if err := tx.Commit(); err != nil {
		return events.Event{}, fmt.Errorf("store: AppendEvent: commit: %w", err)
	}

	return events.Event{
		ID:        id,
		JobID:     jobID,
		Sequence:  seq,
		EventType: eventType,
		Stage:     stage,
		State:     state,
		Payload:   sanitized,
		CreatedAt: now,
	}, nil
}

// ListEvents returns all events for a job ordered by sequence.
func (s *SQLiteStore) ListEvents(ctx context.Context, jobID string) ([]events.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, job_id, sequence, event_type, stage, state, payload_json, created_at
		FROM job_events WHERE job_id = ? ORDER BY sequence`, jobID)
	if err != nil {
		return nil, fmt.Errorf("store: ListEvents: %w", err)
	}
	defer rows.Close()

	var result []events.Event
	for rows.Next() {
		var (
			id              int64
			jid             string
			seq             int
			eventType       string
			stage           string
			state           string
			payloadJSON     string
			createdAt       string
		)
		if err := rows.Scan(&id, &jid, &seq, &eventType, &stage, &state, &payloadJSON, &createdAt); err != nil {
			return nil, fmt.Errorf("store: ListEvents: scan: %w", err)
		}
		ev := events.Event{
			ID:        id,
			JobID:     jid,
			Sequence:  seq,
			EventType: events.EventType(eventType),
			Stage:     stage,
			State:     state,
			CreatedAt: parseTime(createdAt),
		}
		if payloadJSON != "" && payloadJSON != "{}" {
			var payload map[string]any
			if err := json.Unmarshal([]byte(payloadJSON), &payload); err == nil {
				ev.Payload = payload
			}
		}
		if ev.Payload == nil {
			ev.Payload = map[string]any{}
		}
		result = append(result, ev)
	}
	return result, rows.Err()
}

// scanJobRow queries a single job run.
func (s *SQLiteStore) scanJobRow(ctx context.Context, query string, args ...any) (jobs.JobRun, error) {
	var (
		id, projectID, jobType, state, stage, createdAt string
		startedAt, completedAt                          sql.NullString
		cancelRequested                                 int
		progressJSON                                    string
		warningCount                                    int
		errorCode                                       string
	)
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&id, &projectID, &jobType, &state, &stage, &createdAt,
		&startedAt, &completedAt, &cancelRequested, &progressJSON,
		&warningCount, &errorCode)
	if err == sql.ErrNoRows {
		return jobs.JobRun{}, fmt.Errorf("%w: %s", jobs.ErrNotFound, id)
	}
	if err != nil {
		return jobs.JobRun{}, fmt.Errorf("store: scan job row: %w", err)
	}

	run := jobs.JobRun{
		ID:              id,
		ProjectID:       projectID,
		JobType:         jobs.JobType(jobType),
		State:           jobs.JobState(state),
		Stage:           jobs.JobStage(stage),
		CreatedAt:       parseTime(createdAt),
		CancelRequested: cancelRequested != 0,
		WarningCount:    warningCount,
		ErrorCode:       errorCode,
	}
	if startedAt.Valid {
		t := parseTime(startedAt.String)
		run.StartedAt = &t
	}
	if completedAt.Valid {
		t := parseTime(completedAt.String)
		run.CompletedAt = &t
	}
	if progressJSON != "" && progressJSON != "{}" {
		_ = json.Unmarshal([]byte(progressJSON), &run.Progress)
	}
	return run, nil
}

// scanJobRows queries multiple job runs.
func (s *SQLiteStore) scanJobRows(ctx context.Context, query string, args ...any) ([]jobs.JobRun, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query job rows: %w", err)
	}
	defer rows.Close()

	var result []jobs.JobRun
	for rows.Next() {
		var (
			id, projectID, jobType, state, stage, createdAt string
			startedAt, completedAt                          sql.NullString
			cancelRequested                                 int
			progressJSON                                    string
			warningCount                                    int
			errorCode                                       string
		)
		if err := rows.Scan(
			&id, &projectID, &jobType, &state, &stage, &createdAt,
			&startedAt, &completedAt, &cancelRequested, &progressJSON,
			&warningCount, &errorCode); err != nil {
			return nil, fmt.Errorf("store: scan job rows: %w", err)
		}
		run := jobs.JobRun{
			ID:              id,
			ProjectID:       projectID,
			JobType:         jobs.JobType(jobType),
			State:           jobs.JobState(state),
			Stage:           jobs.JobStage(stage),
			CreatedAt:       parseTime(createdAt),
			CancelRequested: cancelRequested != 0,
			WarningCount:    warningCount,
			ErrorCode:       errorCode,
		}
		if startedAt.Valid {
			t := parseTime(startedAt.String)
			run.StartedAt = &t
		}
		if completedAt.Valid {
			t := parseTime(completedAt.String)
			run.CompletedAt = &t
		}
		if progressJSON != "" && progressJSON != "{}" {
			_ = json.Unmarshal([]byte(progressJSON), &run.Progress)
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

// nullIfNil converts a *time.Time to a value suitable for SQL NULL.
func nullIfNil(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// parseTime parses an RFC3339Nano timestamp string, returning zero on error.
func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
