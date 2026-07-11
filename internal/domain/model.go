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

type DirectoryContext struct {
	Role           DirectoryRole `json:"role"`
	AuthorityLevel int           `json:"authority_level"`
	PrivacyLevel   string        `json:"privacy_level"`
	Protected      bool          `json:"protected"`
	MatchedTerms   []string      `json:"matched_terms,omitempty"`
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
}

type DuplicateGroup struct {
	SHA256 string         `json:"sha256"`
	Size   int64          `json:"size"`
	Files  []FileInstance `json:"files"`
}

type PlanState string

const PlanDraft PlanState = "DRAFT"

type PlannedAction struct {
	Path    string           `json:"path"`
	Action  OperationType    `json:"action"`
	Reason  string           `json:"reason"`
	Context DirectoryContext `json:"context"`
}

type OperationPlan struct {
	ID            string          `json:"id"`
	State         PlanState       `json:"state"`
	ContentSHA256 string          `json:"content_sha256"`
	Size          int64           `json:"size"`
	Risk          RiskLevel       `json:"risk"`
	RetainPath    string          `json:"retain_path,omitempty"`
	Actions       []PlannedAction `json:"actions"`
	Evidence      []string        `json:"evidence"`
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
