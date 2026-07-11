package domain

import "time"

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
