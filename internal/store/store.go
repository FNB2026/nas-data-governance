// Package store persists governance artifacts (storages, file instances,
// directory contexts, operation tasks/plans/logs) to a project-owned
// database. It writes only to its own database file — never to user
// file systems — so it stays within AGENTS rule 1's "default read-only"
// boundary for the user's data.
package store

import (
	"context"
	"errors"
	"time"

	"nas-data-governance/internal/domain"
)

// ErrNotFound is returned when a single-row lookup has no match.
var ErrNotFound = errors.New("store: not found")

// LearningBatch is the persisted record of one learning run.
type LearningBatch struct {
	ID          string
	Source      string // stats | corpus | feedback
	StartedAt   time.Time
	CompletedAt *time.Time
	RuleCount   int
	Status      string // running | completed | failed
}

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

	// SaveFormat stores format analysis results for a file row. The format
	// JSON blob captures all FormatInfo fields; format_name is denormalized
	// for quick filtering by category.
	SaveFormat(ctx context.Context, fileID int64, info domain.FormatInfo) error

	// CreateTask inserts a new operation task and returns its row id.
	CreateTask(ctx context.Context, task domain.OperationTask) error
	GetTask(ctx context.Context, id string) (domain.OperationTask, error)
	// ListTasks returns all operation tasks, oldest first. Used by feedback
	// learning to traverse historical plans across all tasks.
	ListTasks(ctx context.Context) ([]domain.OperationTask, error)

	// SavePlans replaces all plans for a task in one transaction.
	SavePlans(ctx context.Context, taskID string, plans []domain.OperationPlan) error
	ListPlans(ctx context.Context, taskID string) ([]domain.OperationPlan, error)
	// ListAllPlans returns plans across all tasks, ordered by task creation
	// time then plan id. Used by feedback learning (L4) to scan the full
	// decision history. Each plan carries RetainScore/RetainPath/Evidence;
	// paths are NOT used for learning — only score components and action
	// types (K-009).
	ListAllPlans(ctx context.Context) ([]domain.OperationPlan, error)

	// AppendLog adds one audit entry. Used by the future executor.
	AppendLog(ctx context.Context, planID, eventType string, detail map[string]any) error
	ListLogs(ctx context.Context, planID string) ([]domain.OperationLog, error)

	// ---------------- rules (L1 infrastructure) ----------------

	// SaveRule inserts or replaces a rule by ID. For learned rules the
	// caller sets Source=learned, Status=draft, BatchID, Confidence.
	SaveRule(ctx context.Context, r domain.Rule) error

	// ListRules returns rules filtered by source and/or status. Either
	// may be empty to skip filtering on that dimension.
	ListRules(ctx context.Context, source domain.RuleSource, status domain.RuleStatus) ([]domain.Rule, error)

	// UpdateRuleStatus transitions a rule's lifecycle state. Used by
	// the CLI approve/reject/disable commands. approvedAt is set when
	// status becomes probation or approved.
	UpdateRuleStatus(ctx context.Context, ruleID string, status domain.RuleStatus, approvedAt *time.Time) error

	// DisableBatch sets status=disabled for all rules in a batch. Used
	// for whole-batch rollback (K-008).
	DisableBatch(ctx context.Context, batchID string) error

	// ---------------- learning batches ----------------

	// SaveLearningBatch inserts or updates a learning batch record.
	SaveLearningBatch(ctx context.Context, b LearningBatch) error
}
