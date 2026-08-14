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

	"github.com/FNB2026/nas-data-governance/internal/domain"
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

// FormatRecord binds analyzed metadata to a stable storage/path identity.
// Paths remain inside the project-owned private database/report layer.
type FormatRecord struct {
	StorageID string
	Path      string
	Info      domain.FormatInfo
}

// Store is the persistence port used by scan/plan/execute layers.
// All methods are context-aware so callers can cancel long operations.
type Store interface {
	Close() error

	// Init applies any pending schema migrations. Idempotent.
	Init(ctx context.Context) error

	// RegisterStorage inserts a new storage. If the ID already exists, the
	// root_path must match — an ID is never rebound to a different root.
	RegisterStorage(ctx context.Context, s domain.Storage) error
	ListStorages(ctx context.Context) ([]domain.Storage, error)

	// UpsertFiles inserts or replaces file instances by (storage_id, path).
	// Returns the database row IDs so callers can attach contexts.
	UpsertFiles(ctx context.Context, files []domain.FileInstance) ([]int64, error)
	// ListFiles returns all files when storageID is empty.
	ListFiles(ctx context.Context, storageID string) ([]domain.FileInstance, error)
	// FileID resolves the row id for (storage_id, path); returns ErrNotFound
	// when absent. Used to attach directory_contexts.
	FileID(ctx context.Context, storageID, path string) (int64, error)

	// ---------------- incremental scan (P0-2) ----------------

	// ListFileMetadata returns lightweight metadata for all active files
	// in a storage. Used by the incremental scanner to detect unchanged
	// files (same size + mtime + inode) and skip hash recomputation.
	ListFileMetadata(ctx context.Context, storageID string) ([]FileMeta, error)

	// MarkFilesMissing sets file_status='missing' for the given paths
	// that were not seen during the current scan. Called after a scan
	// completes to flag deleted files. Returns the count updated.
	MarkFilesMissing(ctx context.Context, storageID string, paths []string) (int64, error)
	// MarkFilesUnavailable preserves unseen rows from an incomplete traversal
	// for audit while excluding them from the current active snapshot.
	MarkFilesUnavailable(ctx context.Context, storageID string, paths []string) (int64, error)

	// MarkFileActive sets file_status='active' for a single path. Called
	// when an incremental scan encounters a previously-missing file that
	// reappeared.
	MarkFileActive(ctx context.Context, storageID, path string) error

	// ---------------- scan checkpoints (P0-2) ----------------

	// StartCheckpoint creates a new scan checkpoint in 'running' state.
	// Returns the checkpoint ID.
	StartCheckpoint(ctx context.Context, storageID string) (int64, error)

	// UpdateCheckpoint updates the checkpoint's progress fields.
	UpdateCheckpoint(ctx context.Context, checkpointID int64, lastPath string, scannedCount int) error

	// CompleteCheckpoint marks a checkpoint as 'completed' or 'aborted'.
	CompleteCheckpoint(ctx context.Context, checkpointID int64, status string) error

	// LastCheckpoint returns the most recent resumable checkpoint for a
	// storage — one whose status is 'running' (crash-interrupted) or
	// 'aborted' (user-cancelled) and not superseded by a later 'completed'
	// checkpoint. Returns ErrNotFound if none exists.
	LastCheckpoint(ctx context.Context, storageID string) (Checkpoint, error)

	// SaveContext upserts a directory context for the given file row.
	// ruleVersion identifies the classifier revision so old contexts can be
	// invalidated when the classifier changes.
	SaveContext(ctx context.Context, fileID int64, c domain.DirectoryContext, ruleVersion string) error

	// SaveFormat stores format analysis results for a file row. The format
	// JSON blob captures all FormatInfo fields; format_name is denormalized
	// for quick filtering by category.
	SaveFormat(ctx context.Context, fileID int64, info domain.FormatInfo) error
	// SaveFormatsByPath persists one validated batch in a transaction and
	// returns (saved, missing file rows). It avoids one transaction per file.
	SaveFormatsByPath(ctx context.Context, records []FormatRecord) (int, int, error)
	// ListFormats returns persisted formats, optionally filtered by storage.
	// Analyze resume uses it to skip already completed NAS reads.
	ListFormats(ctx context.Context, storageID string) ([]FormatRecord, error)

	// CreateTask inserts a new operation task and returns its row id.
	CreateTask(ctx context.Context, task domain.OperationTask) error
	GetTask(ctx context.Context, id string) (domain.OperationTask, error)
	// ListTasks returns all operation tasks, oldest first. Used by feedback
	// learning to traverse historical plans across all tasks.
	ListTasks(ctx context.Context) ([]domain.OperationTask, error)
	// UpdateTaskState transitions a task's state. Used by the runner/CLI
	// to track task lifecycle (queued → running → completed/failed/cancelled).
	UpdateTaskState(ctx context.Context, taskID string, state domain.TaskState) error

	// SavePlans replaces all plans for a task in one transaction.
	SavePlans(ctx context.Context, taskID string, plans []domain.OperationPlan) error
	ListPlans(ctx context.Context, taskID string) ([]domain.OperationPlan, error)
	// GetPlan loads a single plan by its ID. Used by crash recovery (P0-1)
	// to inspect a plan found in EXECUTING state after a restart.
	GetPlan(ctx context.Context, planID string) (domain.OperationPlan, error)
	// UpdatePlanState persists a plan's state transition. Used by crash
	// recovery to mark a plan as ROLLED_BACK or reset to APPROVED.
	UpdatePlanState(ctx context.Context, planID string, state domain.PlanState) error
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

	// ---------------- execution journal (P0-1 崩溃恢复) ----------------

	// BeginJournal 为 plan 的所有 filesystem action 写入 pending 记录。
	// 已存在的记录不重复写入（UNIQUE(plan_id, action_index) 幂等）。
	// 仅记录 touchesFilesystem=true 的 action。
	BeginJournal(ctx context.Context, taskID, planID string, actions []domain.PlannedAction) error

	// MarkJournalDone 标记 (plan_id, action_index) 执行完成，并记录实际
	// 目标路径。actualTargetPath 对 MOVE/COPY/RENAME 等于 action.TargetPath；
	// 对 QUARANTINE/DELETE 是运行时解析出的隔离路径。回滚时需要此路径。
	MarkJournalDone(ctx context.Context, planID string, actionIndex int, actualTargetPath string) error

	// MarkJournalFailed 标记 (plan_id, action_index) 执行失败。
	MarkJournalFailed(ctx context.Context, planID string, actionIndex int) error

	// MarkJournalRolledBack 标记 (plan_id, action_index) 已回滚。
	MarkJournalRolledBack(ctx context.Context, planID string, actionIndex int, rollbackErr error) error

	// ListJournalDone 列出 plan 中 status=done 的记录（用于回滚）。
	// 返回顺序按 action_index 升序；回滚时应倒序处理。
	ListJournalDone(ctx context.Context, planID string) ([]JournalEntry, error)

	// ListJournalPending 列出 plan 中 status=pending 的记录（用于继续执行）。
	// 返回顺序按 action_index 升序。
	ListJournalPending(ctx context.Context, planID string) ([]JournalEntry, error)

	// ListJournalAll 列出 plan 的全部 journal 记录（用于恢复判断）。
	ListJournalAll(ctx context.Context, planID string) ([]JournalEntry, error)

	// ListExecutingPlans 列出 state=EXECUTING 的 plan（崩溃恢复入口）。
	// 返回 plan_id 列表。
	ListExecutingPlans(ctx context.Context) ([]string, error)

	// ---------------- group decisions (V1 ReviewDecision) ----------------

	// SaveGroupDecision inserts or updates a review decision for a duplicate
	// group. The decision is independent of PlanState: it records the user's
	// review intent (KEEP_ALL, DRAFT_ACTION, etc.) without creating or
	// modifying an operation plan. Upsert is keyed on group_id; the latest
	// decision wins.
	SaveGroupDecision(ctx context.Context, d domain.GroupDecision) error

	// GetGroupDecision returns the latest decision for a group, or
	// ErrNotFound when no decision has been recorded.
	GetGroupDecision(ctx context.Context, groupID string) (domain.GroupDecision, error)

	// ListGroupDecisions returns decisions filtered by type. An empty
	// decisionType means no filter (all decisions).
	ListGroupDecisions(ctx context.Context, decisionType domain.ReviewDecisionType) ([]domain.GroupDecision, error)
}

// JournalEntry 是 execution_journal 表的一行。
type JournalEntry struct {
	PlanID         string
	TaskID         string
	ActionIndex    int
	ActionType     string // quarantine/move/copy/delete/rename
	SourcePath     string
	TargetPath     string // 空表示无目标
	ContentSHA256  string
	FileSize       int64
	Status         string // pending/done/failed
	RollbackStatus string // pending/done/failed（空表示未尝试）
	StartedAt      *time.Time
	CompletedAt    *time.Time
}

// FileMeta is the lightweight metadata returned by ListFileMetadata.
// It carries only the fields needed for incremental change detection
// and hash cache reuse. PhysicalReliable is required because an inode from
// SMB/NFS/WebDAV/FUSE must never authorize reuse of a cached content hash.
// Path is included so callers can match scanned files to DB records.
type FileMeta struct {
	Path             string
	Size             int64
	ModifiedAt       time.Time
	Device           uint64
	Inode            uint64
	PhysicalReliable bool
	QuickHash        string
	ContentSHA256    string
}

// Checkpoint is one row of scan_checkpoints. It records where a scan
// left off so it can be resumed after an interrupt.
type Checkpoint struct {
	ID              int64
	StorageID       string
	LastScannedPath string
	ScannedCount    int
	Status          string // running | completed | aborted | paused_network
	StartedAt       time.Time
	UpdatedAt       time.Time
}
