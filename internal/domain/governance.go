package domain

// GovernanceSubject is an explicit group selection, never an action or approval.
type GovernanceSubject struct {
	GroupID          string `json:"group_id"`
	StorageID        string `json:"storage_id"`
	SHA256           string `json:"sha256"`
	SubjectVersion   string `json:"subject_version"`
	DecisionRevision int64  `json:"decision_revision"`
}
type GovernanceQuery struct {
	StorageID    string `json:"storage_id"`
	Search       string `json:"search"`
	DecisionType string `json:"decision_type"`
	MinSize      int64  `json:"min_size"`
	MaxSize      int64  `json:"max_size"`
	Sort         string `json:"sort"`
	Descending   bool   `json:"descending"`
	Page         int    `json:"page"`
	PageSize     int    `json:"page_size"`
}
type GovernanceGroup struct {
	GovernanceSubject
	SamplePath   string   `json:"sample_path"`
	Size         int64    `json:"size"`
	PathCount    int      `json:"path_count"`
	DecisionType string   `json:"decision_type"`
	Risk         string   `json:"risk"`
	Role         string   `json:"role"`
	SafetyFlags  []string `json:"safety_flags"`
}
type GovernancePage struct {
	Items              []GovernanceGroup `json:"items"`
	TotalCount         int               `json:"total_count"`
	Page               int               `json:"page"`
	PageSize           int               `json:"page_size"`
	UnavailableFilters []string          `json:"unavailable_filters"`
}
type GovernanceCapabilities struct {
	CanRecordDecisions bool     `json:"can_record_decisions"`
	CanPreview         bool     `json:"can_preview"`
	RecoveryLockActive bool     `json:"recovery_lock_active"`
	ScanActive         bool     `json:"scan_active"`
	MaxBatchSize       int      `json:"max_batch_size"`
	ReasonCodes        []string `json:"reason_codes"`
	ProjectScope       string   `json:"project_scope"`
}
type PreviewBatchRequest struct {
	Items        []GovernanceSubject `json:"items"`
	DecisionType ReviewDecisionType  `json:"decision_type"`
	Reason       string              `json:"reason"`
}
type PreviewItem struct {
	GovernanceSubject
	Eligibility     string `json:"eligibility"`
	ReasonCode      string `json:"reason_code"`
	SourceFreshness string `json:"source_freshness,omitempty"`
	SourceCheckedAt string `json:"source_checked_at,omitempty"`
}

// SourceFreshness is a point-in-time, read-only observation of the selected
// duplicate members. It is never an execution authorization or a claim that
// the source cannot change after the observation.
type SourceFreshness struct {
	Status     string
	CheckedAt  string
	ReasonCode string
}

// SourceFreshnessRejection returns only allowlisted observations consistent
// with their status. An error string, path or unproven cause is never evidence.
// Reasons cannot turn a non-MATCH observation into permission to record.
func SourceFreshnessRejection(check SourceFreshness, present bool) string {
	if present {
		allowed := false
		switch check.Status {
		case "UNVERIFIED":
			switch check.ReasonCode {
			case "SOURCE_PLATFORM_PROOF_UNAVAILABLE", "SOURCE_VERIFICATION_CANCELLED", "SOURCE_VERIFICATION_TIMEOUT", "SOURCE_CATALOG_UNAVAILABLE":
				allowed = true
			}
		case "SOURCE_UNAVAILABLE":
			switch check.ReasonCode {
			case "SOURCE_MOUNT_BOUNDARY_REJECTED", "SOURCE_SYMLINK_REJECTED", "SOURCE_VERIFICATION_API_UNAVAILABLE", "SOURCE_READ_FAILED", "SOURCE_FILE_UNAVAILABLE", "SOURCE_REGISTRATION_UNAVAILABLE":
				allowed = true
			}
		case "AUTHORIZATION_REQUIRED":
			allowed = check.ReasonCode == "SOURCE_PERMISSION_DENIED" || check.ReasonCode == "SOURCE_AUTHORIZATION_MISMATCH"
		case "SOURCE_CHANGED":
			allowed = check.ReasonCode == "SOURCE_CONTENT_CHANGED" || check.ReasonCode == "SOURCE_IDENTITY_OR_METADATA_CHANGED"
		}
		if allowed {
			return check.ReasonCode
		}
	}
	return SourceFreshnessReason(check.Status, present)
}

func SourceFreshnessReason(status string, present bool) string {
	if !present {
		return "SOURCE_UNVERIFIED"
	}
	switch status {
	case "SOURCE_CHANGED":
		return "SOURCE_CHANGED"
	case "SOURCE_UNAVAILABLE":
		return "SOURCE_UNAVAILABLE"
	case "AUTHORIZATION_REQUIRED":
		return "AUTHORIZATION_REQUIRED"
	case "NETWORK_PAUSED":
		return "NETWORK_PAUSED"
	case "LIMIT_EXCEEDED":
		return "SOURCE_LIMIT_EXCEEDED"
	default:
		return "SOURCE_UNVERIFIED"
	}
}

type GovernancePreview struct {
	PreviewToken string        `json:"preview_token"`
	ExpiresAt    string        `json:"expires_at"`
	ProjectScope string        `json:"project_scope"`
	Items        []PreviewItem `json:"items"`
	RecordsOnly  bool          `json:"records_only"`
}
type CommitBatchRequest struct {
	PreviewToken   string `json:"preview_token"`
	IdempotencyKey string `json:"idempotency_key"`
}
type GovernanceOutcome struct {
	GroupID          string `json:"group_id"`
	Status           string `json:"status"`
	ReasonCode       string `json:"reason_code"`
	DecisionRevision int64  `json:"decision_revision"`
}
type GovernanceCounts struct {
	Total     int `json:"total"`
	Saved     int `json:"saved"`
	Unchanged int `json:"unchanged"`
	Conflict  int `json:"conflict"`
	Blocked   int `json:"blocked"`
	Failed    int `json:"failed"`
}
type GovernanceBatchResult struct {
	BatchID        string              `json:"batch_id"`
	ProjectScope   string              `json:"project_scope"`
	Status         string              `json:"status"`
	RequestedCount int                 `json:"requested_count"`
	Counts         GovernanceCounts    `json:"counts"`
	Items          []GovernanceOutcome `json:"items"`
}

// History exposes durable identifiers and accounting only, never retry keys,
// preview tokens, notes, source paths, or write permission.
type GovernanceBatchQuery struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}
type GovernanceBatchSummary struct {
	BatchID        string `json:"batch_id"`
	Status         string `json:"status"`
	RequestedCount int    `json:"requested_count"`
	RecordedCount  int    `json:"recorded_count"`
	CreatedAt      string `json:"created_at"`
}
type GovernanceBatchPage struct {
	ProjectScope string                   `json:"project_scope"`
	Items        []GovernanceBatchSummary `json:"items"`
	TotalCount   int                      `json:"total_count"`
	Page         int                      `json:"page"`
	PageSize     int                      `json:"page_size"`
}
