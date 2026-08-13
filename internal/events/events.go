// Package events defines structured event types and a persistence-agnostic
// recorder interface for job lifecycle milestones.
//
// Per ADR-0006 §10 (Privacy & Security):
//   - Event payloads must NOT contain file paths, filenames, database paths,
//     quarantine paths, or error messages that embed original paths.
//   - Events are low-frequency milestones (stage transitions, progress
//     checkpoints, warnings, completion), NOT per-file records.
//   - The SanitizePayload function strips known-sensitive keys before
//     persistence; callers should also avoid putting sensitive data in
//     payloads in the first place.
package events

import (
	"context"
	"time"
)

// EventType identifies a job lifecycle milestone.
type EventType string

const (
	// EventCreated is emitted when a job is first persisted (QUEUED).
	EventCreated EventType = "job:created"
	// EventStage is emitted when the job transitions to a new processing stage.
	EventStage EventType = "job:stage"
	// EventProgress is emitted at throttled intervals with aggregate counters.
	EventProgress EventType = "job:progress"
	// EventWarning is emitted when a non-fatal issue occurs (e.g., hash failure).
	EventWarning EventType = "job:warning"
	// EventCompleted is emitted when the job finishes successfully.
	EventCompleted EventType = "job:completed"
	// EventFailed is emitted when the job terminates with an error.
	EventFailed EventType = "job:failed"
	// EventCancelled is emitted when a cancellation request was honored.
	EventCancelled EventType = "job:cancelled"
	// EventPausedNetwork is emitted when a remote source becomes unavailable
	// after a durable resume checkpoint has been preserved.
	EventPausedNetwork EventType = "job:paused_network"
)

// Event is one structured milestone record for a job. The Sequence field
// provides a strict ordering guarantee within a single job — the recorder
// assigns and persists it atomically.
type Event struct {
	// ID is the database-assigned primary key (0 for new events).
	ID int64 `json:"id"`
	// JobID is the foreign key to job_runs.id.
	JobID string `json:"job_id"`
	// Sequence is the per-job monotonic counter, starting at 1.
	Sequence int `json:"sequence"`
	// EventType is one of the Event* constants.
	EventType EventType `json:"event_type"`
	// Stage is the job stage at the time of the event.
	Stage string `json:"stage"`
	// State is the job state at the time of the event.
	State string `json:"state"`
	// Payload carries structured, privacy-safe data about the event.
	// See SanitizePayload for the list of forbidden keys.
	Payload map[string]any `json:"payload"`
	// CreatedAt is the wall-clock time of the event.
	CreatedAt time.Time `json:"created_at"`
}

// Recorder is the persistence port for structured events. The store
// package provides a concrete implementation backed by SQLite.
type Recorder interface {
	// AppendEvent persists a new event for the given job, assigning the
	// next sequence number atomically. The caller fills in EventType,
	// Stage, State, and Payload; the recorder sets ID, Sequence, and
	// CreatedAt.
	AppendEvent(ctx context.Context, jobID string, eventType EventType, stage, state string, payload map[string]any) (Event, error)

	// ListEvents returns all events for a job ordered by sequence.
	ListEvents(ctx context.Context, jobID string) ([]Event, error)
}

// sensitiveKeys are payload keys that must never appear in persisted
// event payloads because they may contain paths, filenames, or other
// location-identifying data.
var sensitiveKeys = map[string]bool{
	"path":            true,
	"file":            true,
	"filename":        true,
	"filepath":        true,
	"source_path":     true,
	"target_path":     true,
	"actual_path":     true,
	"original_path":   true,
	"db_path":         true,
	"database":        true,
	"quarantine":      true,
	"quarantine_path": true,
	"error":           true, // errors may embed paths
	"err":             true,
	"detail":          true, // free-form detail may embed paths
	"reason":          true, // free-form reason may embed paths
}

// SanitizePayload returns a new map with all sensitive keys removed.
// This is a defense-in-depth measure: callers should construct payloads
// that never contain sensitive data, but SanitizePayload catches
// accidental leaks before persistence.
//
// The function also drops nil values to keep payloads compact.
func SanitizePayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return map[string]any{}
	}
	result := make(map[string]any, len(payload))
	for k, v := range payload {
		if v == nil {
			continue
		}
		if sensitiveKeys[k] {
			continue
		}
		result[k] = v
	}
	return result
}
