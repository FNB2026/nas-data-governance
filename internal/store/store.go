// Package store persists governance artifacts (storages, file instances,
// directory contexts, operation tasks/plans/logs) to a project-owned
// database. It writes only to its own database file — never to user
// file systems — so it stays within AGENTS rule 1's "default read-only"
// boundary for the user's data.
package store

import (
	"context"
	"errors"

	"nas-data-governance/internal/domain"
)

// ErrNotFound is returned when a single-row lookup has no match.
var ErrNotFound = errors.New("store: not found")

// Store is the persistence port used by scan/plan/execute layers.
// All methods are context-aware so callers can cancel long operations.
type Store interface {
	Close() error

	// Init applies any pending schema migrations. Idempotent.
	Init(ctx context.Context) error

	// RegisterStorage inserts or replaces a storage by ID.
	RegisterStorage(ctx context.Context, s domain.Storage) error
	ListStorages(ctx context.Context) ([]domain.Storage, error)

	// UpsertFiles inserts or replaces file instances by (storage_id, path).
	// Returns the database row IDs so callers can attach contexts.
	UpsertFiles(ctx context.Context, files []domain.FileInstance) ([]int64, error)
	ListFiles(ctx context.Context, storageID string) ([]domain.FileInstance, error)
	// FileID resolves the row id for (storage_id, path); returns ErrNotFound
	// when absent. Used to attach directory_contexts.
	FileID(ctx context.Context, storageID, path string) (int64, error)

	// SaveContext upserts a directory context for the given file row.
	// ruleVersion identifies the classifier revision so old contexts can be
	// invalidated when the classifier changes.
	SaveContext(ctx context.Context, fileID int64, c domain.DirectoryContext, ruleVersion string) error

	// CreateTask inserts a new operation task and returns its row id.
	CreateTask(ctx context.Context, task domain.OperationTask) error
	GetTask(ctx context.Context, id string) (domain.OperationTask, error)

	// SavePlans replaces all plans for a task in one transaction.
	SavePlans(ctx context.Context, taskID string, plans []domain.OperationPlan) error
	ListPlans(ctx context.Context, taskID string) ([]domain.OperationPlan, error)

	// AppendLog adds one audit entry. Used by the future executor.
	AppendLog(ctx context.Context, planID, eventType string, detail map[string]any) error
	ListLogs(ctx context.Context, planID string) ([]domain.OperationLog, error)
}
