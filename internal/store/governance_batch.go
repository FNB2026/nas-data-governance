package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/report"
)

func (s *SQLiteStore) PreviewBatchDecisions(ctx context.Context, req domain.PreviewBatchRequest, persist bool) (domain.GovernancePreview, error) {
	v := domain.GovernancePreview{Items: []domain.PreviewItem{}, RecordsOnly: true}
	b, _ := json.Marshal(req)
	if len(req.Items) == 0 || len(req.Items) > 100 || len(b) > 131072 || len(req.Reason) > 2048 || !supportedDecision(req.DecisionType) {
		return v, ErrGovernanceRejected
	}
	seen := map[string]bool{}
	for _, ref := range req.Items {
		if seen[ref.GroupID] || len(ref.SubjectVersion) > 64 || ref.DecisionRevision < 0 || len(ref.SHA256) != 64 || ref.GroupID != report.StableGroupID(ref.StorageID, ref.SHA256) || len(ref.StorageID) > 256 {
			return v, ErrGovernanceRejected
		}
		seen[ref.GroupID] = true
	}
	caps, e := s.GovernanceCapabilities(ctx)
	if e != nil {
		return v, e
	}
	v.ProjectScope = caps.ProjectScope
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if e != nil {
		return v, ErrGovernanceRejected
	}
	defer tx.Rollback()
	for _, input := range req.Items {
		ref, _, e := governanceSubject(ctx, tx, input)
		item := domain.PreviewItem{GovernanceSubject: ref, Eligibility: "ELIGIBLE", ReasonCode: "RECORD_ONLY"}
		if e != nil {
			item.Eligibility = "BLOCKED"
			item.ReasonCode = "GROUP_UNAVAILABLE"
		} else if input.SubjectVersion != "" && (input.SubjectVersion != ref.SubjectVersion || input.DecisionRevision != ref.DecisionRevision) {
			item.Eligibility = "CONFLICT"
			item.ReasonCode = "SUBJECT_CHANGED"
		}
		if !persist || !caps.CanRecordDecisions {
			item.Eligibility = "BLOCKED"
			item.ReasonCode = "RECORDING_UNAVAILABLE"
		}
		v.Items = append(v.Items, item)
	}
	if tx.Commit() != nil {
		return v, ErrGovernanceRejected
	}
	if !persist || !caps.CanRecordDecisions {
		return v, nil
	}
	token, e := opaqueID()
	if e != nil {
		return v, e
	}
	v.PreviewToken = token
	v.ExpiresAt = time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339Nano)
	payload, _ := json.Marshal(req)
	snapshot, _ := json.Marshal(v)
	tx, e = s.db.BeginTx(ctx, nil)
	if e != nil {
		return v, ErrGovernanceRejected
	}
	defer tx.Rollback()
	if guardedGovernanceWrite(ctx, s, tx) != nil {
		return domain.GovernancePreview{}, ErrGovernanceRejected
	}
	var live int
	if tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM governance_previews WHERE expires_at>?`, time.Now().UTC().Format(time.RFC3339Nano)).Scan(&live) != nil || live >= 1000 {
		return domain.GovernancePreview{}, ErrGovernanceRejected
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO governance_previews(token,scope,expires_at,payload_json,preview_json) VALUES(?,?,?,?,?)`, token, v.ProjectScope, v.ExpiresAt, string(payload), string(snapshot)); e != nil {
		return domain.GovernancePreview{}, ErrGovernanceRejected
	}
	if tx.Commit() != nil {
		return domain.GovernancePreview{}, ErrGovernanceRejected
	}
	return v, nil
}

func (s *SQLiteStore) CommitBatchDecisions(ctx context.Context, req domain.CommitBatchRequest) (domain.GovernanceBatchResult, error) {
	s.governanceMu.Lock()
	defer s.governanceMu.Unlock()
	if !validKey(req.PreviewToken) || !validKey(req.IdempotencyKey) {
		return domain.GovernanceBatchResult{}, ErrGovernanceRejected
	}
	digest := requestHash(req)
	initialCaps, e := s.GovernanceCapabilities(ctx)
	if e != nil || !initialCaps.CanRecordDecisions {
		return domain.GovernanceBatchResult{}, ErrGovernanceRejected
	}
	var batchID, existingDigest, state, batchScope string
	e = s.db.QueryRowContext(ctx, `SELECT id,request_digest,state,scope FROM governance_batches WHERE idempotency_key=?`, req.IdempotencyKey).Scan(&batchID, &existingDigest, &state, &batchScope)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return domain.GovernanceBatchResult{}, ErrGovernanceRejected
	}
	if e == nil {
		if existingDigest != digest || batchScope != initialCaps.ProjectScope {
			return domain.GovernanceBatchResult{}, ErrGovernanceRejected
		}
		if state != "IN_PROGRESS" {
			return s.GetBatchDecisionResult(ctx, batchID)
		}
	}
	var scope, expiry, payload, snapshot string
	if s.db.QueryRowContext(ctx, `SELECT scope,expires_at,payload_json,preview_json FROM governance_previews WHERE token=?`, req.PreviewToken).Scan(&scope, &expiry, &payload, &snapshot) != nil {
		return domain.GovernanceBatchResult{}, ErrGovernanceRejected
	}
	var input domain.PreviewBatchRequest
	var preview domain.GovernancePreview
	if len(payload) > 131072 || len(snapshot) > 131072 || json.Unmarshal([]byte(payload), &input) != nil || json.Unmarshal([]byte(snapshot), &preview) != nil || len(input.Items) != len(preview.Items) || len(input.Items) == 0 || len(input.Items) > 100 || !supportedDecision(input.DecisionType) || len(input.Reason) > 2048 || preview.ProjectScope != scope || preview.PreviewToken != req.PreviewToken {
		return domain.GovernanceBatchResult{}, ErrGovernanceRejected
	}
	caps, e := s.GovernanceCapabilities(ctx)
	if e != nil || !caps.CanRecordDecisions || scope != caps.ProjectScope {
		return domain.GovernanceBatchResult{}, ErrGovernanceRejected
	}
	expires, e := time.Parse(time.RFC3339Nano, expiry)
	if e != nil || !time.Now().Before(expires) {
		return domain.GovernanceBatchResult{}, ErrGovernanceRejected
	}
	if batchID == "" {
		batchID, e = opaqueID()
		if e != nil {
			return domain.GovernanceBatchResult{}, e
		}
		tx, e := s.db.BeginTx(ctx, nil)
		if e != nil {
			return domain.GovernanceBatchResult{}, ErrGovernanceRejected
		}
		defer tx.Rollback()
		if guardedGovernanceWrite(ctx, s, tx) != nil {
			return domain.GovernanceBatchResult{}, ErrGovernanceRejected
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO governance_batches(id,scope,idempotency_key,request_digest,preview_token,requested_count,state,created_at) VALUES(?,?,?,?,?,?,'IN_PROGRESS',?)`, batchID, scope, req.IdempotencyKey, digest, req.PreviewToken, len(input.Items), time.Now().UTC().Format(time.RFC3339Nano)); e != nil {
			return domain.GovernanceBatchResult{}, ErrGovernanceRejected
		}
		if tx.Commit() != nil {
			return domain.GovernanceBatchResult{}, ErrGovernanceRejected
		}
	}
	for i := range preview.Items {
		if ctx.Err() != nil {
			return s.GetBatchDecisionResult(context.Background(), batchID)
		}
		if e = s.commitDecisionItem(ctx, batchID, input, preview.Items[i]); e != nil {
			return s.GetBatchDecisionResult(context.Background(), batchID)
		}
	}
	if _, e = s.db.ExecContext(ctx, `UPDATE governance_batches SET state='COMPLETE' WHERE id=? AND state='IN_PROGRESS' AND requested_count=(SELECT COUNT(*) FROM governance_outcomes WHERE batch_id=?)`, batchID, batchID); e != nil {
		return domain.GovernanceBatchResult{}, ErrGovernanceRejected
	}
	return s.GetBatchDecisionResult(ctx, batchID)
}

// Each decision, durable audit and outcome commit atomically. A crash leaves
// only completed item transactions; same-key replay processes missing items.
func (s *SQLiteStore) commitDecisionItem(ctx context.Context, batchID string, input domain.PreviewBatchRequest, item domain.PreviewItem) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return ErrGovernanceRejected
	}
	defer tx.Rollback()
	var exists int
	if tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM governance_outcomes WHERE batch_id=? AND group_id=?`, batchID, item.GroupID).Scan(&exists) != nil {
		return ErrGovernanceRejected
	}
	if exists > 0 {
		return nil
	}
	o := domain.GovernanceOutcome{GroupID: item.GroupID, Status: "BLOCKED", ReasonCode: item.ReasonCode, DecisionRevision: item.DecisionRevision}
	old := ""
	oldSnapshot, createdAt, snapshotErr := decisionAuditSnapshot(ctx, tx, item.GroupID)
	if snapshotErr != nil {
		return snapshotErr
	}
	ref, decision, subjectErr := governanceSubject(ctx, tx, item.GovernanceSubject)
	if subjectErr == nil {
		old = decision
		o.DecisionRevision = ref.DecisionRevision
	}
	if guardedGovernanceWrite(ctx, s, tx) != nil {
		o.ReasonCode = "SAFETY_STATE_CHANGED"
	} else if item.Eligibility != "ELIGIBLE" {
		if item.Eligibility == "CONFLICT" {
			o.Status = "CONFLICT"
		}
	} else if subjectErr != nil {
		o.Status = "CONFLICT"
		o.ReasonCode = "GROUP_UNAVAILABLE"
	} else if ref.SubjectVersion != item.SubjectVersion || ref.DecisionRevision != item.DecisionRevision {
		o.Status = "CONFLICT"
		o.ReasonCode = "SUBJECT_CHANGED"
	} else {
		var reason string
		err := tx.QueryRowContext(ctx, `SELECT COALESCE(reason,'') FROM group_decisions WHERE group_id=? ORDER BY updated_at DESC,id DESC LIMIT 1`, item.GroupID).Scan(&reason)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ErrGovernanceRejected
		}
		if old == string(input.DecisionType) && reason == input.Reason {
			o.Status = "UNCHANGED"
			o.ReasonCode = "ALREADY_RECORDED"
		} else {
			o.Status = "SAVED"
			o.ReasonCode = "RECORDED_ONLY"
			o.DecisionRevision = ref.DecisionRevision + 1
			// Conditional expected revision is checked in the same immediate tx;
			// updated writers share one owner and connection. No last-write-wins.
			if _, e = tx.ExecContext(ctx, `DELETE FROM group_decisions WHERE group_id=?`, item.GroupID); e != nil {
				return ErrGovernanceRejected
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if createdAt == "" {
				createdAt = now
			}
			if _, e = tx.ExecContext(ctx, `INSERT INTO group_decisions(id,group_id,decision_type,reason,created_at,updated_at,revision) VALUES(?,?,?,?,?,?,?)`, item.GroupID, item.GroupID, string(input.DecisionType), input.Reason, createdAt, now, o.DecisionRevision); e != nil {
				return ErrGovernanceRejected
			}
		}
	}
	if governanceAudit(ctx, tx, batchID, item.GroupID, oldSnapshot, string(input.DecisionType), item.SubjectVersion, o.DecisionRevision, input.Reason, "batch", o.Status, o.ReasonCode, time.Now().UTC().Format(time.RFC3339Nano)) != nil {
		return ErrGovernanceRejected
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO governance_outcomes(batch_id,group_id,status,reason_code,decision_revision) VALUES(?,?,?,?,?)`, batchID, o.GroupID, o.Status, o.ReasonCode, o.DecisionRevision); e != nil {
		return ErrGovernanceRejected
	}
	if tx.Commit() != nil {
		return ErrGovernanceRejected
	}
	return nil
}

func (s *SQLiteStore) GetBatchDecisionResult(ctx context.Context, batchID string) (domain.GovernanceBatchResult, error) {
	v := domain.GovernanceBatchResult{Items: []domain.GovernanceOutcome{}}
	var token string
	column := "id"
	selector := batchID
	if strings.HasPrefix(selector, "key:") {
		column = "idempotency_key"
		selector = strings.TrimPrefix(selector, "key:")
	}
	if !validKey(selector) {
		return v, ErrGovernanceRejected
	}
	if s.db.QueryRowContext(ctx, `SELECT id,scope,state,requested_count,preview_token FROM governance_batches WHERE `+column+`=?`, selector).Scan(&v.BatchID, &v.ProjectScope, &v.Status, &v.RequestedCount, &token) != nil {
		return v, ErrGovernanceRejected
	}
	batchID = v.BatchID
	var currentScope string
	if s.db.QueryRowContext(ctx, `SELECT scope FROM governance_project WHERE id=1`).Scan(&currentScope) != nil {
		return v, ErrGovernanceRejected
	}
	// Copied backups retain historical rows but cannot replay original tokens.
	if !strings.HasPrefix(v.ProjectScope, currentScope+"-") || s.scopeBinding == "" {
		return v, ErrGovernanceRejected
	}
	var raw string
	if s.db.QueryRowContext(ctx, `SELECT preview_json FROM governance_previews WHERE token=? AND scope=?`, token, v.ProjectScope).Scan(&raw) != nil {
		return v, ErrGovernanceRejected
	}
	var preview domain.GovernancePreview
	if json.Unmarshal([]byte(raw), &preview) != nil {
		return v, ErrGovernanceRejected
	}
	v.ProjectScope = currentScope + "-" + s.scopeBinding // private backup histories remain readable
	for _, item := range preview.Items {
		o := domain.GovernanceOutcome{GroupID: item.GroupID, Status: "FAILED", ReasonCode: "NOT_COMMITTED"}
		e := s.db.QueryRowContext(ctx, `SELECT status,reason_code,decision_revision FROM governance_outcomes WHERE batch_id=? AND group_id=?`, batchID, item.GroupID).Scan(&o.Status, &o.ReasonCode, &o.DecisionRevision)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return v, ErrGovernanceRejected
		}
		v.Items = append(v.Items, o)
		switch o.Status {
		case "SAVED":
			v.Counts.Saved++
		case "UNCHANGED":
			v.Counts.Unchanged++
		case "CONFLICT":
			v.Counts.Conflict++
		case "BLOCKED":
			v.Counts.Blocked++
		default:
			v.Counts.Failed++
		}
		v.Counts.Total++
	}
	return v, nil
}
