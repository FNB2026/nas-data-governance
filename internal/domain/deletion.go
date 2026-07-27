package domain

import "time"

// QuarantineStatus is the managed lifecycle of one isolated file.
type QuarantineStatus string

const (
	QuarantineActive        QuarantineStatus = "QUARANTINED"
	QuarantineHold          QuarantineStatus = "HOLD"
	QuarantinePurgeEligible QuarantineStatus = "PURGE_ELIGIBLE"
	QuarantineRestored      QuarantineStatus = "RESTORED"
	QuarantinePurged        QuarantineStatus = "PURGED"
	QuarantineRolledBack    QuarantineStatus = "ROLLED_BACK"
)

// QuarantineItem is the private, durable record for one recoverable removal.
// Real paths belong only in owner-only plans, journals, and databases.
type QuarantineItem struct {
	ID             string           `json:"id"`
	PlanID         string           `json:"plan_id"`
	ActionIndex    int              `json:"action_index"`
	SourcePath     string           `json:"source_path"`
	QuarantinePath string           `json:"quarantine_path"`
	ContentSHA256  string           `json:"content_sha256"`
	FileSize       int64            `json:"file_size"`
	QuarantinedAt  time.Time        `json:"quarantined_at"`
	RetainUntil    time.Time        `json:"retain_until"`
	Status         QuarantineStatus `json:"status"`
	HoldReason     string           `json:"hold_reason,omitempty"`
	RestoredAt     *time.Time       `json:"restored_at,omitempty"`
	PurgedAt       *time.Time       `json:"purged_at,omitempty"`
}

type RestorePlanState string

const (
	RestoreDraft      RestorePlanState = "DRAFT"
	RestoreApproved   RestorePlanState = "APPROVED"
	RestoreCompleted  RestorePlanState = "RESTORED"
	RestoreRolledBack RestorePlanState = "ROLLED_BACK"
)

// RestorePlan moves one managed quarantine item back to its original path.
// It is independently planned and approved and never overwrites a target.
type RestorePlan struct {
	ID             string           `json:"id"`
	ItemID         string           `json:"item_id"`
	State          RestorePlanState `json:"state"`
	QuarantinePath string           `json:"quarantine_path"`
	RestorePath    string           `json:"restore_path"`
	ExpectedSHA256 string           `json:"expected_sha256"`
	ExpectedSize   int64            `json:"expected_size"`
	ApprovalDigest string           `json:"approval_digest"`
	CreatedAt      time.Time        `json:"created_at"`
	ApprovedAt     *time.Time       `json:"approved_at,omitempty"`
	RestoredAt     *time.Time       `json:"restored_at,omitempty"`
}

type RestoreJournalEntry struct {
	PlanID         string
	ItemID         string
	QuarantinePath string
	RestorePath    string
	ContentSHA256  string
	FileSize       int64
	Status         string
	StartedAt      time.Time
	CompletedAt    *time.Time
}

// PurgePlanState is deliberately separate from OperationPlan state. PURGE is
// irreversible after its commit point and must never be accepted by the
// source-directory executor.
type PurgePlanState string

const (
	PurgeDraft      PurgePlanState = "DRAFT"
	PurgeApproved   PurgePlanState = "APPROVED"
	PurgeStaged     PurgePlanState = "STAGED"
	PurgeCommitted  PurgePlanState = "PURGED"
	PurgeRolledBack PurgePlanState = "ROLLED_BACK"
	PurgeFailed     PurgePlanState = "FAILED"
)

// PurgePlan is a snapshot-bound, single-item permanent deletion proposal.
// ApprovalDigest must be presented both at approval and execution time.
type PurgePlan struct {
	ID             string         `json:"id"`
	ItemID         string         `json:"item_id"`
	State          PurgePlanState `json:"state"`
	ExpectedPath   string         `json:"expected_path"`
	ExpectedSHA256 string         `json:"expected_sha256"`
	ExpectedSize   int64          `json:"expected_size"`
	RetainUntil    time.Time      `json:"retain_until"`
	ApprovalDigest string         `json:"approval_digest"`
	CreatedAt      time.Time      `json:"created_at"`
	ApprovedAt     *time.Time     `json:"approved_at,omitempty"`
	PurgedAt       *time.Time     `json:"purged_at,omitempty"`
}

// PurgeJournalEntry records the irreversible executor's crash boundary.
type PurgeJournalEntry struct {
	PlanID         string
	ItemID         string
	QuarantinePath string
	StagingPath    string
	ContentSHA256  string
	FileSize       int64
	Status         string
	StartedAt      time.Time
	CompletedAt    *time.Time
}
