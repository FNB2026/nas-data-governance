package domain

import "time"

type DirectoryRole string

const (
	RoleUnknown       DirectoryRole = "unknown"
	RoleRaw           DirectoryRole = "raw_source"
	RoleFormalArchive DirectoryRole = "formal_archive"
	RoleProjectWork   DirectoryRole = "project_work"
	RoleTemporary     DirectoryRole = "temporary"
	RoleUnorganized   DirectoryRole = "unorganized"
	RoleBackup        DirectoryRole = "backup"
	RoleCache         DirectoryRole = "cache_derived"
	RoleSensitive     DirectoryRole = "sensitive"
	RoleSystem        DirectoryRole = "system_application"
)

// Storage describes one indexed volume. The schema lives in schemas/001_initial.sql.
type Storage struct {
	ID        string    `json:"id"`
	RootPath  string    `json:"root_path"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}

type DirectoryContext struct {
	Role           DirectoryRole `json:"role"`
	AuthorityLevel int           `json:"authority_level"`
	PrivacyLevel   string        `json:"privacy_level"`
	Protected      bool          `json:"protected"`
	MatchedTerms   []string      `json:"matched_terms,omitempty"`
	// ParentChain records the role/authority of up to 6 ancestor directories,
	// nearest first. White paper §5-7 requires comparison across 1-6 levels.
	ParentChain []ChainNode `json:"parent_chain,omitempty"`
	// BranchPoint is the nearest ancestor where sibling directories diverge
	// in role; empty when the chain is uniform. Used to detect cross-archive
	// duplicates that share a shallow root but differ in storage duty.
	BranchPoint string `json:"branch_point,omitempty"`
	// BusinessAnchor is a stable identifier (client name, project code) found
	// along the path, used to distinguish "same content, different business
	// purpose" cases. Empty when no anchor is detected.
	BusinessAnchor string `json:"business_anchor,omitempty"`
}

// ChainNode is one entry in DirectoryContext.ParentChain.
type ChainNode struct {
	Path      string        `json:"path"`
	Name      string        `json:"name"`
	Role      DirectoryRole `json:"role"`
	Authority int           `json:"authority"`
}

// RetentionScore explains why a particular duplicate copy is retained.
// Higher Total wins; ties fall back to path/mtime stable ordering.
type RetentionScore struct {
	Total     int      `json:"total"`
	Authority int      `json:"authority"`  // from DirectoryContext.AuthorityLevel
	Stability int      `json:"stability"`  // older mtime = more stable
	PathDepth int      `json:"path_depth"` // deeper organized path wins
	RoleBonus int      `json:"role_bonus"` // raw/archive bonus; temporary/cache penalty
	Reasons   []string `json:"reasons"`
}

type FileInstance struct {
	StorageID     string    `json:"storage_id"`
	Path          string    `json:"path"`
	Name          string    `json:"name"`
	Size          int64     `json:"size"`
	Mode          uint32    `json:"mode"`
	ModifiedAt    time.Time `json:"modified_at"`
	Device        uint64    `json:"device"`
	Inode         uint64    `json:"inode"`
	IsSymlink     bool      `json:"is_symlink"`
	QuickHash     string    `json:"quick_hash,omitempty"`
	ContentSHA256 string    `json:"content_sha256,omitempty"`
	DiscoveredAt  time.Time `json:"discovered_at"`
	// Format carries detected format metadata when format analysis has run.
	// Empty until analyze is called; not populated by the scanner.
	Format FormatInfo `json:"format,omitempty"`
}

// FormatCategory groups formats by their role in the asset lifecycle.
type FormatCategory string

const (
	CategoryImage    FormatCategory = "image"
	CategoryVideo    FormatCategory = "video"
	CategoryAudio    FormatCategory = "audio"
	CategoryDocument FormatCategory = "document"
	CategoryArchive  FormatCategory = "archive"
	CategoryCode     FormatCategory = "code"
	CategoryOther    FormatCategory = "other"
	CategoryUnknown  FormatCategory = "unknown"
)

// FormatInfo holds the result of lightweight format analysis.
// Per K-006 (progressive analysis), this is header-only: no OCR, no media
// decoding, no AI. The goal is to identify the real format and extract
// cheap metadata that helps distinguish "same content, different encoding"
// from true duplicates.
type FormatInfo struct {
	// Format is the canonical format name (e.g., "jpeg", "png", "mp4").
	Format string `json:"format,omitempty"`
	// Category groups the format for lifecycle decisions.
	Category FormatCategory `json:"category,omitempty"`
	// MIME is the best-guess MIME type.
	MIME string `json:"mime,omitempty"`
	// Width and Height apply to images and video frames.
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
	// Duration is media duration in seconds (fractional).
	Duration float64 `json:"duration,omitempty"`
	// Pages applies to PDF and Office documents.
	Pages int `json:"pages,omitempty"`
	// Codec identifies the encoding used (e.g., "h264", "aac").
	Codec string `json:"codec,omitempty"`
	// ArchiveEntryCount is the number of entries in an archive.
	ArchiveEntryCount int `json:"archive_entry_count,omitempty"`
}

type DuplicateGroup struct {
	SHA256 string         `json:"sha256"`
	Size   int64          `json:"size"`
	Files  []FileInstance `json:"files"`
}

type PlanState string

const (
	PlanDraft        PlanState = "DRAFT"         // planner output, awaiting review
	PlanApproved     PlanState = "APPROVED"      // human-approved for execution
	PlanStaleChecked PlanState = "STALE_CHECKED" // pre-execution state verified
	PlanExecuting    PlanState = "EXECUTING"     // action in progress
	PlanVerified     PlanState = "VERIFIED"      // post-execution checksum passed
	PlanRolledBack   PlanState = "ROLLED_BACK"   // rollback completed
)

type PlannedAction struct {
	Path   string        `json:"path"`
	Action OperationType `json:"action"`
	Reason string        `json:"reason"`
	// TargetPath is the destination for MOVE, COPY, and RENAME actions.
	// Empty for DELETE, QUARANTINE, KEEP, SKIP, REVIEW.
	TargetPath string           `json:"target_path,omitempty"`
	Context    DirectoryContext `json:"context"`
	// File is the plan-time snapshot used by the executor's stale check.
	// Empty for actions that don't touch the filesystem (REVIEW, SKIP).
	File FileInstance `json:"file,omitempty"`
}

type OperationPlan struct {
	ID            string          `json:"id"`
	TaskID        string          `json:"task_id,omitempty"`
	State         PlanState       `json:"state"`
	ContentSHA256 string          `json:"content_sha256"`
	Size          int64           `json:"size"`
	Risk          RiskLevel       `json:"risk"`
	RetainPath    string          `json:"retain_path,omitempty"`
	RetainScore   RetentionScore  `json:"retain_score,omitempty"`
	Actions       []PlannedAction `json:"actions"`
	Evidence      []string        `json:"evidence"`
}

// OperationTask groups plans produced in one planner run for traceability.
type OperationTask struct {
	ID        string    `json:"id"`
	RootPath  string    `json:"root_path"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

// TaskState 类型化 operation_tasks.state 字段。此前为 ad-hoc 字符串，
// P1-4 引入枚举常量以便 runner 和 CLI 一致引用。
type TaskState string

const (
	TaskQueued    TaskState = "queued"    // 任务已创建，等待执行
	TaskRunning   TaskState = "running"   // 任务正在执行（scan/analyze/execute）
	TaskCompleted TaskState = "completed" // 任务成功完成
	TaskFailed    TaskState = "failed"    // 任务失败
	TaskCancelled TaskState = "cancelled" // 任务被用户取消（Ctrl+C）
)

// OperationLog is one audit entry appended during plan execution.
type OperationLog struct {
	ID        int64          `json:"id"`
	PlanID    string         `json:"plan_id"`
	EventType string         `json:"event_type"`
	Detail    map[string]any `json:"detail"`
	CreatedAt time.Time      `json:"created_at"`
}

type OperationType string

const (
	OperationKeep       OperationType = "KEEP"
	OperationMove       OperationType = "MOVE"
	OperationCopy       OperationType = "COPY"
	OperationRename     OperationType = "RENAME"
	OperationDelete     OperationType = "DELETE"
	OperationQuarantine OperationType = "QUARANTINE"
	OperationSkip       OperationType = "SKIP"
	OperationReview     OperationType = "REVIEW"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// RelationType classifies the relationship between two file instances.
// Only "identical" (same size + same full hash) is safe for automated
// dedup; the others default to review per K-002.
type RelationType string

const (
	RelationIdentical  RelationType = "identical"  // 字节级完全重复
	RelationDerivative RelationType = "derivative" // 原始与派生（同内容不同编码）
	RelationVersion    RelationType = "version"    // 版本关系（命名模式）
	RelationSimilar    RelationType = "similar"    // 视觉/听觉近似
)

// FileRelation describes a relationship between two file paths. Score is a
// 0-1 confidence where applicable; 0 means N/A.
type FileRelation struct {
	Type     RelationType `json:"type"`
	A        string       `json:"a"`
	B        string       `json:"b"`
	Score    float64      `json:"score,omitempty"`
	Evidence []string     `json:"evidence,omitempty"`
}

// AssetGroup is a collection of files belonging to the same project, event,
// or business matter. Clustering is path/anchor based (K-001/K-002); it is
// read-only and never modifies the filesystem.
type AssetGroup struct {
	ID       string         `json:"id"`
	Anchor   string         `json:"anchor,omitempty"`
	RootPath string         `json:"root_path"`
	Members  []FileInstance `json:"members"`
	Evidence []string       `json:"evidence"`
}

// MergeSuggestion proposes consolidating sibling directories that hold the
// same business purpose. It is a read-only recommendation; execution still
// requires plan approval (K-008).
type MergeSuggestion struct {
	ID         string   `json:"id"`
	TargetDir  string   `json:"target_dir"`
	SourceDirs []string `json:"source_dirs"`
	Reason     string   `json:"reason"`
	Confidence float64  `json:"confidence"`
	Evidence   []string `json:"evidence"`
}

// RuleSource identifies where a rule came from.
type RuleSource string

const (
	RuleSourceBuiltin RuleSource = "builtin" // 硬编码内置规则
	RuleSourceLearned RuleSource = "learned" // 学习产出
	RuleSourceUser    RuleSource = "user"    // 用户显式声明
)

// RuleStatus is the lifecycle state of a learned rule.
type RuleStatus string

const (
	RuleDraft     RuleStatus = "draft"     // 草案，待审批
	RuleProbation RuleStatus = "probation" // 试用期，审批后观察 N 天
	RuleApproved  RuleStatus = "approved"  // 正式生效
	RuleDisabled  RuleStatus = "disabled"  // 已禁用（可回滚启用）
	RuleRejected  RuleStatus = "rejected"  // 审批拒绝，归档不生效
)

// Rule is the persisted representation of a directory classification rule.
// Builtin rules are loaded from code constants; learned rules from the DB.
// Per K-008, learned rules have priority <= 60 and never override
// protection rules (priority 90-100).
type Rule struct {
	ID         string     `json:"id"`
	Version    int        `json:"version"`
	Priority   int        `json:"priority"`
	Enabled    bool       `json:"enabled"`
	Source     RuleSource `json:"source"`
	BatchID    string     `json:"batch_id,omitempty"`
	Confidence float64    `json:"confidence"`
	Status     RuleStatus `json:"status"`
	ApprovedAt *time.Time `json:"approved_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Definition string     `json:"definition_yaml"`
}
