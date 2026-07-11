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
	Path    string           `json:"path"`
	Action  OperationType    `json:"action"`
	Reason  string           `json:"reason"`
	Context DirectoryContext `json:"context"`
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
