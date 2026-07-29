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

// PhysicalIdentity captures the physical storage identity of a file instance.
// Multiple file paths that share the same Device+Inode are hardlinks to a
// single physical data object; deleting one path does not free the data
// block. The duplicate report must distinguish path instances from physical
// objects to avoid overstating reclaimable capacity.
//
// Per ADR-0006 / V1 correctness: when Device and Inode are both non-zero and
// the filesystem provides stable inode numbers, PhysicalKey is
// "storage_id:device:inode". When either is zero or the filesystem cannot
// guarantee stable inodes (e.g., some SMB/FUSE mounts), PhysicalKey is empty
// and each path is treated as a separate physical copy (conservative).
type PhysicalIdentity struct {
	Device    uint64 `json:"device"`
	Inode     uint64 `json:"inode"`
	LinkCount uint64 `json:"link_count,omitempty"`
	// Reliable indicates whether Device+Inode can be trusted as a stable
	// physical identity. False on filesystems that do not guarantee stable
	// inode numbers.
	Reliable bool `json:"reliable"`
}

// PhysicalKey returns a stable identifier for the physical data object.
// Returns "" when the identity is not reliable, forcing conservative
// treatment (each path = one physical copy).
func (p PhysicalIdentity) PhysicalKey(storageID string) string {
	if !p.Reliable || p.Device == 0 || p.Inode == 0 {
		return ""
	}
	return storageID + ":" + uint64ToString(p.Device) + ":" + uint64ToString(p.Inode)
}

// uint64ToString avoids fmt.Sprintf allocation in the hot path.
func uint64ToString(n uint64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
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
	// Physical captures the hardlink-level identity of this file instance.
	// Populated by the scanner when Device and Inode are available; used by
	// the duplicate report to avoid counting hardlinks as independent
	// reclaimable copies. Zero value means "not assessed; treat conservatively".
	Physical PhysicalIdentity `json:"physical,omitempty"`
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
	// Role distinguishes primary content from protected project/sidecar data.
	// It is advisory metadata only; destructive decisions still require review.
	Role FormatRole `json:"role,omitempty"`
	// Protected prevents ordinary cleanup planning for project sources and
	// sidecars. Regenerable means a cache may be rebuilt, but never implies
	// that it is safe to delete without verifying its primary/project.
	Protected   bool `json:"protected,omitempty"`
	Regenerable bool `json:"regenerable,omitempty"`
}

type FormatRole string

const (
	FormatRolePrimary          FormatRole = "primary"
	FormatRoleProjectSource    FormatRole = "project_source"
	FormatRoleMetadataSidecar  FormatRole = "metadata_sidecar"
	FormatRoleRegenerableCache FormatRole = "regenerable_cache"
)

type DuplicateGroup struct {
	SHA256 string         `json:"sha256"`
	Size   int64          `json:"size"`
	Files  []FileInstance `json:"files"`
	// GroupID is a stable identifier for this duplicate group, independent
	// of array order or pagination. Format: SHA256 of
	// governance_domain_id + storage_id + content_sha256.
	// Empty when not computed (legacy callers).
	GroupID string `json:"group_id,omitempty"`
	// PathCount is len(Files).
	PathCount int `json:"path_count"`
	// PhysicalCopyCount is the number of distinct physical data objects.
	// Hardlinks sharing one inode count as one physical copy. When physical
	// identity is unreliable, PhysicalCopyCount equals PathCount.
	PhysicalCopyCount int `json:"physical_copy_count"`
	// HardlinkAliasCount is the number of paths that share a physical
	// object with at least one other path (i.e., paths - physical copies).
	HardlinkAliasCount int `json:"hardlink_alias_count"`
	// PhysicalReclaimableBytes is the maximum bytes that could be freed by
	// removing duplicate paths while keeping at least one physical object.
	// For hardlinks, only the last path deletion frees data; so reclaimable
	// = (physical_copies - 1) * size. When identity is unreliable,
	// reclaimable = (path_count - 1) * size.
	PhysicalReclaimableBytes int64 `json:"physical_reclaimable_bytes"`
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
	RelationSidecar    RelationType = "sidecar"    // 主资产与受保护侧车/缓存依赖
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
	ID             string         `json:"id"`
	Anchor         string         `json:"anchor,omitempty"`
	RootPath       string         `json:"root_path"`
	Members        []FileInstance `json:"members"`
	Evidence       []string       `json:"evidence"`
	ReviewRequired bool           `json:"review_required,omitempty"`
	ReviewReason   string         `json:"review_reason,omitempty"`
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

// ReviewDecisionType classifies the human review decision for a duplicate
// group. This is intentionally separate from PlanState: a review decision
// records the user's intent ("keep all", "draft an action"), while PlanState
// tracks the execution lifecycle of an operation plan (DRAFT → APPROVED →
// EXECUTING → VERIFIED/ROLLED_BACK).
//
// Per ADR-0006 / V1 item 6: ReviewDecision and PlanState are separated so
// that a user can mark a group "KEEP_ALL" without creating a plan, and a
// plan can be approved/rejected independently of the review decision.
type ReviewDecisionType string

const (
	// DecisionUnreviewed is the zero value; no decision has been recorded.
	DecisionUnreviewed ReviewDecisionType = ""
	// DecisionKeepAll means the user reviewed and decided to keep all
	// copies. The group is closed for further automated suggestions.
	DecisionKeepAll ReviewDecisionType = "KEEP_ALL"
	// DecisionDraftAction means the user wants a governance plan drafted
	// (e.g., quarantine redundant copies). The plan still goes through the
	// normal DRAFT → APPROVED → EXECUTING lifecycle.
	DecisionDraftAction ReviewDecisionType = "DRAFT_ACTION"
	// DecisionDeferred means the user is not ready to decide yet. The
	// group remains in the review queue.
	DecisionDeferred ReviewDecisionType = "DEFERRED"
	// DecisionRejectedSuggestion means the user disagrees with the system's
	// duplicate assessment. The group is excluded from future automated
	// suggestions unless re-opened.
	DecisionRejectedSuggestion ReviewDecisionType = "REJECTED_SUGGESTION"
	// DecisionCrossArchive marks the group as cross-archive duplicates.
	// These are "related copies" only, not candidates for automated
	// dedup (per V1 item 5: governance domain boundary).
	DecisionCrossArchive ReviewDecisionType = "CROSS_ARCHIVE"
	// DecisionBackupRelation marks the group as a backup relationship.
	// Backup copies are retained; the group is excluded from dedup.
	DecisionBackupRelation ReviewDecisionType = "BACKUP_RELATION"
	// DecisionPrimaryRetention records which specific file copy the user
	// chose as the primary retention. RetainedFileID points to the
	// file_instances row.
	DecisionPrimaryRetention ReviewDecisionType = "PRIMARY_RETENTION"
)

// GroupDecision is the persisted review decision for one duplicate group.
// It maps to the group_decisions table (migration 008).
type GroupDecision struct {
	ID             string             `json:"id"`
	GroupID        string             `json:"group_id"`
	DecisionType   ReviewDecisionType `json:"decision_type"`
	RetainedFileID *int64             `json:"retained_file_id,omitempty"`
	Reason         string             `json:"reason,omitempty"`
	RuleID         string             `json:"rule_id,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}
