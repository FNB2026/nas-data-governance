package wails

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/events"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
	"github.com/FNB2026/nas-data-governance/internal/query"
	"github.com/FNB2026/nas-data-governance/internal/report"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// StorageInfo is the DTO for a storage entry returned by ListStorages.
type StorageInfo struct {
	ID        string `json:"id"`
	RootPath  string `json:"root_path"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"created_at"` // RFC3339
}

// ListGroupsRequest is the input DTO for ListDuplicateGroups.
// Cursor is an opaque base64 token; empty string means first page.
type ListGroupsRequest struct {
	StorageID           string `json:"storage_id,omitempty"`
	PageSize            int    `json:"page_size,omitempty"`
	Cursor              string `json:"cursor,omitempty"`
	MinReclaimableBytes int64  `json:"min_reclaimable_bytes,omitempty"`
}

// GroupSummary is the list-view DTO for a duplicate group.
type GroupSummary struct {
	GroupID                  string `json:"group_id"`
	SHA256                   string `json:"sha256"`
	Size                     int64  `json:"size"`
	StorageID                string `json:"storage_id"`
	PathCount                int    `json:"path_count"`
	PhysicalCopyCount        int    `json:"physical_copy_count"`
	HardlinkAliasCount       int    `json:"hardlink_alias_count"`
	PhysicalReclaimableBytes int64  `json:"physical_reclaimable_bytes"`
	SamplePath               string `json:"sample_path"`
	DecisionType             string `json:"decision_type,omitempty"`
}

// ListGroupsResponse is the paginated output DTO for ListDuplicateGroups.
// NextCursor is empty when there are no more pages.
type ListGroupsResponse struct {
	Groups     []GroupSummary `json:"groups"`
	NextCursor string         `json:"next_cursor,omitempty"`
	TotalCount int            `json:"total_count"`
}

// FileItem is the DTO for a file instance in the group detail view.
type FileItem struct {
	StorageID     string `json:"storage_id"`
	Path          string `json:"path"`
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	ModifiedAt    string `json:"modified_at"`
	IsSymlink     bool   `json:"is_symlink"`
	QuickHash     string `json:"quick_hash,omitempty"`
	ContentSHA256 string `json:"content_sha256,omitempty"`
	// Physical identity (hardlink awareness)
	PhysicalDevice    uint64 `json:"physical_device,omitempty"`
	PhysicalInode     uint64 `json:"physical_inode,omitempty"`
	PhysicalLinkCount uint64 `json:"physical_link_count,omitempty"`
	PhysicalReliable  bool   `json:"physical_reliable"`
	// Format info
	FormatKind string `json:"format_kind,omitempty"`
	FormatMIME string `json:"format_mime,omitempty"`
}

// GroupDetailResponse is the output DTO for GetGroupDetail.
// It embeds GroupSummary and adds the full file member list.
type GroupDetailResponse struct {
	GroupSummary
	Files []FileItem `json:"files"`
}

// ---- V4 Scan & Job DTOs ----

// StartScanRequest is the input DTO for StartScan.
type StartScanRequest struct {
	Root      string `json:"root"`
	StorageID string `json:"storage_id"`
	FullScan  bool   `json:"full_scan,omitempty"`
	Workers   int    `json:"workers,omitempty"`
}

// StartScanResponse is the output DTO for StartScan.
type StartScanResponse struct {
	JobID string `json:"job_id"`
}

// ScanJobProgress is the progress DTO returned by GetScanProgress.
// All fields are privacy-safe aggregate counters (ADR-0006 §10).
type ScanJobProgress struct {
	JobID        string `json:"job_id"`
	State        string `json:"state"`
	Stage        string `json:"stage"`
	Discovered   int64  `json:"discovered"`
	Processed    int64  `json:"processed"`
	Failed       int64  `json:"failed"`
	WarningCount int    `json:"warning_count"`
	ErrorCode    string `json:"error_code,omitempty"`
	CreatedAt    string `json:"created_at"`
	StartedAt    string `json:"started_at,omitempty"`
	CompletedAt  string `json:"completed_at,omitempty"`
}

// JobSummary is the list-view DTO for a job in ListRecentJobs.
type JobSummary struct {
	JobID       string `json:"job_id"`
	JobType     string `json:"job_type"`
	State       string `json:"state"`
	Stage       string `json:"stage"`
	Discovered  int64  `json:"discovered,omitempty"`
	Processed   int64  `json:"processed,omitempty"`
	Failed      int64  `json:"failed,omitempty"`
	ErrorCode   string `json:"error_code,omitempty"`
	CreatedAt   string `json:"created_at"`
	CompletedAt string `json:"completed_at,omitempty"`
}

// JobEvent is the DTO for a single structured event in GetJobDetail.
type JobEvent struct {
	Sequence  int            `json:"sequence"`
	EventType string         `json:"event_type"`
	Stage     string         `json:"stage"`
	State     string         `json:"state"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt string         `json:"created_at"`
}

// JobDetailResponse is the output DTO for GetJobDetail.
type JobDetailResponse struct {
	ScanJobProgress
	Events []JobEvent `json:"events"`
}

// ---- V5 Diagnostic DTOs ----

// DiagnoseFormatsRequest is the input DTO for DiagnoseFormats.
type DiagnoseFormatsRequest struct {
	StorageID           string `json:"storage_id,omitempty"`
	LargeUnknownMinimum int64  `json:"large_unknown_minimum,omitempty"`
}

// DiagnoseGovernanceRequest is the input DTO for DiagnoseGovernance.
type DiagnoseGovernanceRequest struct {
	StorageID         string `json:"storage_id,omitempty"`
	LargeMediaMinimum int64  `json:"large_media_minimum,omitempty"`
}

// DiagnoseMergesRequest is the input DTO for DiagnoseMerges.
type DiagnoseMergesRequest struct {
	StorageID string `json:"storage_id,omitempty"`
}

// ---- mapping helpers ----

func mapStorages(storages []domain.Storage) []StorageInfo {
	out := make([]StorageInfo, len(storages))
	for i, s := range storages {
		createdAt := ""
		if !s.CreatedAt.IsZero() {
			createdAt = s.CreatedAt.UTC().Format(time.RFC3339)
		}
		out[i] = StorageInfo{
			ID:        s.ID,
			RootPath:  s.RootPath,
			Kind:      s.Kind,
			CreatedAt: createdAt,
		}
	}
	return out
}

func mapGroupSummary(g query.DuplicateGroupSummary) GroupSummary {
	return GroupSummary{
		GroupID:                  g.GroupID,
		SHA256:                   g.SHA256,
		Size:                     g.Size,
		StorageID:                g.StorageID,
		PathCount:                g.PathCount,
		PhysicalCopyCount:        g.PhysicalCopyCount,
		HardlinkAliasCount:       g.HardlinkAliasCount,
		PhysicalReclaimableBytes: g.PhysicalReclaimableBytes,
		SamplePath:               g.SamplePath,
		DecisionType:             g.DecisionType,
	}
}

func mapGroupPage(page query.GroupPage) ListGroupsResponse {
	groups := make([]GroupSummary, len(page.Groups))
	for i, g := range page.Groups {
		groups[i] = mapGroupSummary(g)
	}
	return ListGroupsResponse{
		Groups:     groups,
		NextCursor: encodeCursor(page.NextCursor),
		TotalCount: page.TotalCount,
	}
}

func mapFileItem(f domain.FileInstance) FileItem {
	modifiedAt := ""
	if !f.ModifiedAt.IsZero() {
		modifiedAt = f.ModifiedAt.UTC().Format(time.RFC3339)
	}
	return FileItem{
		StorageID:         f.StorageID,
		Path:              f.Path,
		Name:              f.Name,
		Size:              f.Size,
		ModifiedAt:        modifiedAt,
		IsSymlink:         f.IsSymlink,
		QuickHash:         f.QuickHash,
		ContentSHA256:     f.ContentSHA256,
		PhysicalDevice:    f.Physical.Device,
		PhysicalInode:     f.Physical.Inode,
		PhysicalLinkCount: f.Physical.LinkCount,
		PhysicalReliable:  f.Physical.Reliable,
		FormatKind:        f.Format.Format,
		FormatMIME:        f.Format.MIME,
	}
}

func mapGroupDetail(d query.GroupDetail) GroupDetailResponse {
	files := make([]FileItem, len(d.Files))
	for i, f := range d.Files {
		files[i] = mapFileItem(f)
	}
	return GroupDetailResponse{
		GroupSummary: mapGroupSummary(d.DuplicateGroupSummary),
		Files:        files,
	}
}

// ---- V4 job mapping helpers ----

func formatTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func mapScanJobProgress(j jobs.JobRun) ScanJobProgress {
	return ScanJobProgress{
		JobID:        j.ID,
		State:        string(j.State),
		Stage:        string(j.Stage),
		Discovered:   j.Progress.Discovered,
		Processed:    j.Progress.Processed,
		Failed:       j.Progress.Failed,
		WarningCount: j.WarningCount,
		ErrorCode:    j.ErrorCode,
		CreatedAt:    formatTime(&j.CreatedAt),
		StartedAt:    formatTime(j.StartedAt),
		CompletedAt:  formatTime(j.CompletedAt),
	}
}

func mapJobSummary(j jobs.JobRun) JobSummary {
	return JobSummary{
		JobID:       j.ID,
		JobType:     string(j.JobType),
		State:       string(j.State),
		Stage:       string(j.Stage),
		Discovered:  j.Progress.Discovered,
		Processed:   j.Progress.Processed,
		Failed:      j.Progress.Failed,
		ErrorCode:   j.ErrorCode,
		CreatedAt:   formatTime(&j.CreatedAt),
		CompletedAt: formatTime(j.CompletedAt),
	}
}

func mapJobEvent(e events.Event) JobEvent {
	return JobEvent{
		Sequence:  e.Sequence,
		EventType: string(e.EventType),
		Stage:     e.Stage,
		State:     e.State,
		Payload:   e.Payload,
		CreatedAt: formatTime(&e.CreatedAt),
	}
}

// ---- cursor encoding ----

// encodeCursor serializes a GroupCursor into an opaque base64 token.
// Returns empty string for nil cursor (no more pages).
func encodeCursor(c *query.GroupCursor) string {
	if c == nil {
		return ""
	}
	data, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

// decodeCursor parses an opaque base64 token back into a GroupCursor.
// Returns nil (no error) for empty string (first page).
func decodeCursor(s string) (*query.GroupCursor, error) {
	if s == "" {
		return nil, nil
	}
	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("wails: invalid cursor encoding: %w", err)
	}
	var c query.GroupCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("wails: decode cursor: %w", err)
	}
	return &c, nil
}

// toQuery converts a ListGroupsRequest into a query.GroupQuery.
func (req ListGroupsRequest) toQuery() (query.GroupQuery, error) {
	if req.PageSize < 0 || req.PageSize > 200 {
		return query.GroupQuery{}, fmt.Errorf("wails: page_size must be between 1 and 200, or 0 for the default")
	}
	if req.MinReclaimableBytes < 0 {
		return query.GroupQuery{}, fmt.Errorf("wails: min_reclaimable_bytes must not be negative")
	}
	cursor, err := decodeCursor(req.Cursor)
	if err != nil {
		return query.GroupQuery{}, err
	}
	if cursor != nil && (cursor.SHA256 == "" || cursor.StorageID == "" || cursor.ReclaimableEstimate < 0) {
		return query.GroupQuery{}, fmt.Errorf("wails: cursor is incomplete or invalid")
	}
	return query.GroupQuery{
		StorageID:           req.StorageID,
		PageSize:            req.PageSize,
		Cursor:              cursor,
		MinReclaimableBytes: req.MinReclaimableBytes,
	}, nil
}

// ---- V6 Governance DTOs ----

// PlanActionDTO is the DTO for a single planned action within a plan.
type PlanActionDTO struct {
	Path        string `json:"path"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	TargetPath  string `json:"target_path,omitempty"`
	ContextRole string `json:"context_role,omitempty"`
}

// PlanDTO is the DTO for an operation plan, suitable for frontend display.
// It omits RetainScore (which has nested reasons) and the full File snapshot
// from PlannedAction; instead it exposes the flat fields the UI needs.
type PlanDTO struct {
	ID            string          `json:"id"`
	GroupID       string          `json:"group_id"`
	TaskID        string          `json:"task_id,omitempty"`
	State         string          `json:"state"`
	ContentSHA256 string          `json:"content_sha256"`
	Size          int64           `json:"size"`
	Risk          string          `json:"risk"`
	RetainPath    string          `json:"retain_path,omitempty"`
	Actions       []PlanActionDTO `json:"actions"`
	Evidence      []string        `json:"evidence"`
}

// GroupDecisionDTO is the DTO for a persisted review decision.
type GroupDecisionDTO struct {
	ID             string `json:"id"`
	GroupID        string `json:"group_id"`
	DecisionType   string `json:"decision_type"`
	RetainedFileID *int64 `json:"retained_file_id,omitempty"`
	Reason         string `json:"reason,omitempty"`
	RuleID         string `json:"rule_id,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// SaveDecisionRequest is the input DTO for SaveGroupDecision.
type SaveDecisionRequest struct {
	GroupID      string `json:"group_id"`
	DecisionType string `json:"decision_type"`
	Reason       string `json:"reason,omitempty"`
}

// ApprovePlansRequest is the input DTO for ApprovePlans.
type ApprovePlansRequest struct {
	PlanIDs []string `json:"plan_ids"`
}

// ApprovePlansResponse is the output DTO for ApprovePlans.
type ApprovePlansResponse struct {
	Approved []PlanDTO `json:"approved"`
}

// ---- V6 governance mapping helpers ----

func mapPlan(p domain.OperationPlan) PlanDTO {
	actions := make([]PlanActionDTO, len(p.Actions))
	for i, a := range p.Actions {
		actions[i] = PlanActionDTO{
			Path:        a.Path,
			Action:      string(a.Action),
			Reason:      a.Reason,
			TargetPath:  a.TargetPath,
			ContextRole: string(a.Context.Role),
		}
	}
	groupID := p.GroupID
	if groupID == "" && len(p.Actions) > 0 && p.Actions[0].File.StorageID != "" {
		groupID = report.StableGroupID(p.Actions[0].File.StorageID, p.ContentSHA256)
	}
	return PlanDTO{
		ID:            p.ID,
		GroupID:       groupID,
		TaskID:        p.TaskID,
		State:         string(p.State),
		ContentSHA256: p.ContentSHA256,
		Size:          p.Size,
		Risk:          string(p.Risk),
		RetainPath:    p.RetainPath,
		Actions:       actions,
		Evidence:      p.Evidence,
	}
}

func mapDecision(d domain.GroupDecision) GroupDecisionDTO {
	return GroupDecisionDTO{
		ID:             d.ID,
		GroupID:        d.GroupID,
		DecisionType:   string(d.DecisionType),
		RetainedFileID: d.RetainedFileID,
		Reason:         d.Reason,
		RuleID:         d.RuleID,
		CreatedAt:      d.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:      d.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// ---- V7 Execution DTOs ----

// QuarantineItemDTO is the DTO for a quarantined file lifecycle entry.
type QuarantineItemDTO struct {
	ID             string `json:"id"`
	PlanID         string `json:"plan_id"`
	ActionIndex    int    `json:"action_index"`
	SourcePath     string `json:"source_path"`
	QuarantinePath string `json:"quarantine_path"`
	ContentSHA256  string `json:"content_sha256"`
	FileSize       int64  `json:"file_size"`
	QuarantinedAt  string `json:"quarantined_at"`
	RetainUntil    string `json:"retain_until"`
	Status         string `json:"status"`
	HoldReason     string `json:"hold_reason,omitempty"`
	RestoredAt     string `json:"restored_at,omitempty"`
	PurgedAt       string `json:"purged_at,omitempty"`
}

// RestorePlanDTO is the DTO for a restore plan.
type RestorePlanDTO struct {
	ID             string `json:"id"`
	ItemID         string `json:"item_id"`
	State          string `json:"state"`
	QuarantinePath string `json:"quarantine_path"`
	RestorePath    string `json:"restore_path"`
	ExpectedSHA256 string `json:"expected_sha256"`
	ExpectedSize   int64  `json:"expected_size"`
	ApprovalDigest string `json:"approval_digest"`
	CreatedAt      string `json:"created_at"`
	ApprovedAt     string `json:"approved_at,omitempty"`
	RestoredAt     string `json:"restored_at,omitempty"`
}

// PurgePlanDTO is the DTO for a purge plan.
type PurgePlanDTO struct {
	ID               string `json:"id"`
	ItemID           string `json:"item_id"`
	State            string `json:"state"`
	ExpectedPath     string `json:"expected_path"`
	ExpectedSHA256   string `json:"expected_sha256"`
	ExpectedSize     int64  `json:"expected_size"`
	RetainUntil      string `json:"retain_until"`
	ApprovalDigest   string `json:"approval_digest"`
	ConfirmationText string `json:"confirmation_text"`
	CreatedAt        string `json:"created_at"`
	ApprovedAt       string `json:"approved_at,omitempty"`
	DryRunVerifiedAt string `json:"dry_run_verified_at,omitempty"`
	PurgedAt         string `json:"purged_at,omitempty"`
}

// ExecuteRestoreRequest is the input DTO for ExecuteRestore.
type ExecuteRestoreRequest struct {
	PlanID         string   `json:"plan_id"`
	Digest         string   `json:"digest"`
	QuarantineRoot string   `json:"quarantine_root"`
	SourceRoots    []string `json:"source_roots"`
	DryRun         bool     `json:"dry_run"`
}

// ExecuteRestoreResponse is the output DTO for ExecuteRestore.
type ExecuteRestoreResponse struct {
	PlanID     string `json:"plan_id"`
	FinalState string `json:"final_state"`
	Status     string `json:"status"`
	ErrorType  string `json:"error_type,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ExecutePurgeRequest is the input DTO for ExecutePurge.
type ExecutePurgeRequest struct {
	PlanID         string `json:"plan_id"`
	Digest         string `json:"digest"`
	QuarantineRoot string `json:"quarantine_root"`
	DryRun         bool   `json:"dry_run"`
	Confirmation   string `json:"confirmation,omitempty"`
}

// ExecutePurgeResponse is the output DTO for ExecutePurge.
type ExecutePurgeResponse struct {
	PlanID     string `json:"plan_id"`
	FinalState string `json:"final_state"`
	Status     string `json:"status"`
	ErrorType  string `json:"error_type,omitempty"`
	Error      string `json:"error,omitempty"`
}

// RecoveryStatusDTO reports whether any plans are stuck in EXECUTING state.
type RecoveryStatusDTO struct {
	LockActive            bool `json:"lock_active"`
	ExecutingCount        int  `json:"executing_count"`
	SourceExecutingCount  int  `json:"source_executing_count"`
	RestorePendingCount   int  `json:"restore_pending_count"`
	PurgeRecoverableCount int  `json:"purge_recoverable_count"`
}

// RecoverRestoresRequest is the input DTO for RecoverRestores.
type RecoverRestoresRequest struct {
	QuarantineRoot string   `json:"quarantine_root"`
	SourceRoots    []string `json:"source_roots"`
}

// RecoveryResultDTO is the DTO for a single source-execution recovery outcome.
type RecoveryResultDTO struct {
	PlanID     string   `json:"plan_id"`
	Action     string   `json:"action"`
	RolledBack int      `json:"rolled_back"`
	Errors     []string `json:"errors,omitempty"`
}

// RestoreRecoveryResultDTO is the DTO for a restore crash-recovery outcome.
type RestoreRecoveryResultDTO struct {
	PlanID     string `json:"plan_id,omitempty"`
	FinalState string `json:"final_state,omitempty"`
	Status     string `json:"status"`
	ErrorType  string `json:"error_type,omitempty"`
	Error      string `json:"error,omitempty"`
}

// PurgeRecoveryResultDTO is the DTO for a purge crash-recovery outcome.
type PurgeRecoveryResultDTO struct {
	PlanID     string `json:"plan_id,omitempty"`
	FinalState string `json:"final_state,omitempty"`
	Status     string `json:"status"`
	ErrorType  string `json:"error_type,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ---- V7 mapping helpers ----

func mapQuarantineItem(item domain.QuarantineItem) QuarantineItemDTO {
	dto := QuarantineItemDTO{
		ID:             item.ID,
		PlanID:         item.PlanID,
		ActionIndex:    item.ActionIndex,
		SourcePath:     item.SourcePath,
		QuarantinePath: item.QuarantinePath,
		ContentSHA256:  item.ContentSHA256,
		FileSize:       item.FileSize,
		QuarantinedAt:  item.QuarantinedAt.UTC().Format(time.RFC3339),
		RetainUntil:    item.RetainUntil.UTC().Format(time.RFC3339),
		Status:         string(item.Status),
		HoldReason:     item.HoldReason,
	}
	if item.RestoredAt != nil && !item.RestoredAt.IsZero() {
		dto.RestoredAt = item.RestoredAt.UTC().Format(time.RFC3339)
	}
	if item.PurgedAt != nil && !item.PurgedAt.IsZero() {
		dto.PurgedAt = item.PurgedAt.UTC().Format(time.RFC3339)
	}
	return dto
}

func mapRestorePlan(p domain.RestorePlan) RestorePlanDTO {
	dto := RestorePlanDTO{
		ID:             p.ID,
		ItemID:         p.ItemID,
		State:          string(p.State),
		QuarantinePath: p.QuarantinePath,
		RestorePath:    p.RestorePath,
		ExpectedSHA256: p.ExpectedSHA256,
		ExpectedSize:   p.ExpectedSize,
		ApprovalDigest: p.ApprovalDigest,
		CreatedAt:      p.CreatedAt.UTC().Format(time.RFC3339),
	}
	if p.ApprovedAt != nil && !p.ApprovedAt.IsZero() {
		dto.ApprovedAt = p.ApprovedAt.UTC().Format(time.RFC3339)
	}
	if p.RestoredAt != nil && !p.RestoredAt.IsZero() {
		dto.RestoredAt = p.RestoredAt.UTC().Format(time.RFC3339)
	}
	return dto
}

func mapPurgePlan(p domain.PurgePlan) PurgePlanDTO {
	dto := PurgePlanDTO{
		ID:               p.ID,
		ItemID:           p.ItemID,
		State:            string(p.State),
		ExpectedPath:     p.ExpectedPath,
		ExpectedSHA256:   p.ExpectedSHA256,
		ExpectedSize:     p.ExpectedSize,
		RetainUntil:      p.RetainUntil.UTC().Format(time.RFC3339),
		ApprovalDigest:   p.ApprovalDigest,
		ConfirmationText: app.PurgeConfirmationText(p),
		CreatedAt:        p.CreatedAt.UTC().Format(time.RFC3339),
	}
	if p.ApprovedAt != nil && !p.ApprovedAt.IsZero() {
		dto.ApprovedAt = p.ApprovedAt.UTC().Format(time.RFC3339)
	}
	if p.DryRunVerifiedAt != nil && !p.DryRunVerifiedAt.IsZero() {
		dto.DryRunVerifiedAt = p.DryRunVerifiedAt.UTC().Format(time.RFC3339)
	}
	if p.PurgedAt != nil && !p.PurgedAt.IsZero() {
		dto.PurgedAt = p.PurgedAt.UTC().Format(time.RFC3339)
	}
	return dto
}

func errText(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

// ---- V8 Audit DTOs ----

// OperationLogDTO is the DTO for an audit log entry.
type OperationLogDTO struct {
	ID        int            `json:"id"`
	PlanID    string         `json:"plan_id"`
	EventType string         `json:"event_type"`
	Detail    map[string]any `json:"detail,omitempty"`
	CreatedAt string         `json:"created_at"`
}

// JournalEntryDTO is the DTO for an execution journal entry.
type JournalEntryDTO struct {
	PlanID         string `json:"plan_id"`
	TaskID         string `json:"task_id"`
	ActionIndex    int    `json:"action_index"`
	ActionType     string `json:"action_type"`
	SourcePath     string `json:"source_path"`
	TargetPath     string `json:"target_path,omitempty"`
	ContentSHA256  string `json:"content_sha256"`
	FileSize       int64  `json:"file_size"`
	Status         string `json:"status"`
	RollbackStatus string `json:"rollback_status,omitempty"`
	StartedAt      string `json:"started_at,omitempty"`
	CompletedAt    string `json:"completed_at,omitempty"`
}

// ---- V8 mapping helpers ----

func mapOperationLog(l domain.OperationLog) OperationLogDTO {
	return OperationLogDTO{
		ID:        int(l.ID),
		PlanID:    l.PlanID,
		EventType: l.EventType,
		Detail:    l.Detail,
		CreatedAt: l.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func mapJournalEntry(e store.JournalEntry) JournalEntryDTO {
	dto := JournalEntryDTO{
		PlanID:         e.PlanID,
		TaskID:         e.TaskID,
		ActionIndex:    e.ActionIndex,
		ActionType:     e.ActionType,
		SourcePath:     e.SourcePath,
		TargetPath:     e.TargetPath,
		ContentSHA256:  e.ContentSHA256,
		FileSize:       e.FileSize,
		Status:         e.Status,
		RollbackStatus: e.RollbackStatus,
	}
	if e.StartedAt != nil && !e.StartedAt.IsZero() {
		dto.StartedAt = e.StartedAt.UTC().Format(time.RFC3339)
	}
	if e.CompletedAt != nil && !e.CompletedAt.IsZero() {
		dto.CompletedAt = e.CompletedAt.UTC().Format(time.RFC3339)
	}
	return dto
}
