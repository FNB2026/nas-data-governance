package store

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestGovernanceHistoryReopensReadOnlyWithoutKeysOrMutations(t *testing.T) {
	ctx := context.Background()
	s, refs, before := governanceFixture(t, 2)
	ids := []string{}
	for _, kind := range []domain.ReviewDecisionType{domain.DecisionKeepAll, domain.DecisionDeferred, domain.DecisionDraftAction} {
		p := previewFor(t, s, refs, kind)
		r, e := s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: governanceTestKey(t)})
		if e != nil || r.Status != "COMPLETE" {
			t.Fatal("fixture did not complete")
		}
		ids = append(ids, r.BatchID)
	}
	path := s.path
	if e := s.Close(); e != nil {
		t.Fatal("fixture close")
	}
	bytesBefore, e := os.ReadFile(path)
	if e != nil {
		t.Fatal("fixture database snapshot")
	}
	ro, e := OpenReadOnly(ctx, path)
	if e != nil {
		t.Fatal("readonly reopen")
	}
	defer ro.Close()
	page, e := ro.ListGovernanceBatches(ctx, domain.GovernanceBatchQuery{Page: 1, PageSize: 2})
	if e != nil || page.TotalCount != 3 || len(page.Items) != 2 || page.Page != 1 || page.PageSize != 2 {
		t.Fatal("history not discoverable after losing client keys")
	}
	second, e := ro.ListGovernanceBatches(ctx, domain.GovernanceBatchQuery{Page: 2, PageSize: 2})
	if e != nil || second.TotalCount != 3 || len(second.Items) != 1 {
		t.Fatal("history page accounting")
	}
	seen := map[string]bool{}
	for _, item := range append(page.Items, second.Items...) {
		if seen[item.BatchID] || item.Status != "COMPLETE" || item.RequestedCount != 2 || item.RecordedCount != 2 {
			t.Fatal("duplicate or inconsistent summary")
		}
		seen[item.BatchID] = true
		r, e := ro.GetBatchDecisionResult(ctx, item.BatchID)
		if e != nil || r.Counts.Total != 2 {
			t.Fatal("discovered receipt unreadable")
		}
	}
	for _, id := range ids {
		if !seen[id] {
			t.Fatal("history lost a durable batch")
		}
	}
	wire, _ := json.Marshal(page)
	for _, field := range []string{"idempotency_key", "preview_token", "request_digest", "synthetic reason", "payload_json", "source_path"} {
		if strings.Contains(string(wire), field) {
			t.Fatal("history exposed request or private metadata")
		}
	}
	if e := ro.Close(); e != nil {
		t.Fatal("readonly close")
	}
	bytesAfter, e := os.ReadFile(path)
	if e != nil || !reflect.DeepEqual(bytesBefore, bytesAfter) {
		t.Fatal("history changed database bytes")
	}
	checkSourceBytes(t, before)
}

func TestGovernanceHistoryKeepsInterruptedStateAndRejectsInvalidAccounting(t *testing.T) {
	ctx := context.Background()
	s, refs, before := governanceFixture(t, 2)
	if _, e := s.db.Exec(`CREATE TRIGGER synthetic_history_failure BEFORE INSERT ON governance_outcomes BEGIN SELECT RAISE(ABORT,'synthetic'); END`); e != nil {
		t.Fatal("fault injection")
	}
	p := previewFor(t, s, refs, domain.DecisionKeepAll)
	r, e := s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: governanceTestKey(t)})
	if e != nil || r.Status != "IN_PROGRESS" {
		t.Fatal("interrupted fixture")
	}
	page, e := s.ListGovernanceBatches(ctx, domain.GovernanceBatchQuery{})
	if e != nil || len(page.Items) != 1 || page.Items[0].Status != "IN_PROGRESS" || page.Items[0].RecordedCount != 0 {
		t.Fatal("history falsely completed interrupted batch")
	}
	if auditCount(t, s) != 0 {
		t.Fatal("query wrote audit")
	}
	var state string
	if s.db.QueryRow(`SELECT state FROM governance_batches WHERE id=?`, r.BatchID).Scan(&state) != nil || state != "IN_PROGRESS" {
		t.Fatal("query finalized batch")
	}
	for _, q := range []domain.GovernanceBatchQuery{{Page: -1}, {PageSize: 51}, {Page: 1_000_001}, {PageSize: -1}} {
		if _, e := s.ListGovernanceBatches(ctx, q); e == nil {
			t.Fatal("unbounded query accepted")
		}
	}
	if _, e := s.db.Exec(`UPDATE governance_batches SET state='COMPLETE' WHERE id=?`, r.BatchID); e != nil {
		t.Fatal("synthetic inconsistent finalization")
	}
	if _, e := s.ListGovernanceBatches(ctx, domain.GovernanceBatchQuery{}); e == nil {
		t.Fatal("invalid terminal summary accepted")
	}
	checkSourceBytes(t, before)
}

func TestGovernanceHistoryProjectIsolationAndFractionalTimeOrdering(t *testing.T) {
	ctx := context.Background()
	s, refs, _ := governanceFixture(t, 1)
	ids := []string{}
	for i := 0; i < 2; i++ {
		p := previewFor(t, s, refs, domain.DecisionKeepAll)
		r, e := s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: governanceTestKey(t)})
		if e != nil {
			t.Fatal("fixture commit")
		}
		ids = append(ids, r.BatchID)
	}
	for i, stamp := range []string{"2026-10-13T00:00:00Z", "2026-10-13T00:00:00.1Z"} {
		if _, e := s.db.Exec(`UPDATE governance_batches SET created_at=? WHERE id=?`, stamp, ids[i]); e != nil {
			t.Fatal("synthetic clock fixture")
		}
	}
	page, e := s.ListGovernanceBatches(ctx, domain.GovernanceBatchQuery{})
	if e != nil || len(page.Items) != 2 || page.Items[0].BatchID != ids[1] {
		t.Fatal("fractional timestamp order")
	}
	other, _, _ := governanceFixture(t, 1)
	page, e = other.ListGovernanceBatches(ctx, domain.GovernanceBatchQuery{})
	if e != nil || page.TotalCount != 0 || len(page.Items) != 0 {
		t.Fatal("history crossed projects")
	}
	if _, e = other.GetBatchDecisionResult(ctx, ids[0]); e == nil {
		t.Fatal("foreign batch lookup accepted")
	}
}

func TestGovernanceHistoryRejectsForeignOutcomeWithEqualCount(t *testing.T) {
	ctx := context.Background()
	s, refs, _ := governanceFixture(t, 1)
	p := previewFor(t, s, refs, domain.DecisionKeepAll)
	r, e := s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: governanceTestKey(t)})
	if e != nil || r.Status != "COMPLETE" {
		t.Fatal("complete fixture required")
	}
	if _, e = s.db.Exec(`UPDATE governance_outcomes SET group_id='synthetic-foreign-group' WHERE batch_id=?`, r.BatchID); e != nil {
		t.Fatal("foreign outcome injection")
	}
	if _, e = s.ListGovernanceBatches(ctx, domain.GovernanceBatchQuery{}); e == nil {
		t.Fatal("equal row count falsely certified COMPLETE for different group")
	}
}

func TestGovernanceHistoryValidatesPreviewIdentityAndTerminalOutcomes(t *testing.T) {
	for _, mode := range []string{"duplicate-members", "foreign-scope", "wrong-token", "foreign-request-member", "uncommitted-terminal"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s, refs, before := governanceFixture(t, 2)
			p := previewFor(t, s, refs, domain.DecisionKeepAll)
			r, e := s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: governanceTestKey(t)})
			if e != nil || r.Status != "COMPLETE" {
				t.Fatal("terminal fixture required")
			}
			var raw, payload string
			if s.db.QueryRow(`SELECT preview_json,payload_json FROM governance_previews WHERE token=?`, p.PreviewToken).Scan(&raw, &payload) != nil {
				t.Fatal("snapshot fixture")
			}
			var snap domain.GovernancePreview
			var req domain.PreviewBatchRequest
			if json.Unmarshal([]byte(raw), &snap) != nil || json.Unmarshal([]byte(payload), &req) != nil {
				t.Fatal("snapshot decode")
			}
			switch mode {
			case "duplicate-members":
				snap.Items[1].GroupID = snap.Items[0].GroupID
			case "foreign-scope":
				snap.ProjectScope = "synthetic-foreign-scope"
			case "wrong-token":
				snap.PreviewToken = "synthetic-foreign-token"
			case "foreign-request-member":
				req.Items[1].GroupID = "synthetic-foreign-group"
			case "uncommitted-terminal":
				if _, e = s.db.Exec(`UPDATE governance_outcomes SET status='FAILED',reason_code='NOT_COMMITTED' WHERE batch_id=?`, r.BatchID); e != nil {
					t.Fatal("outcome injection")
				}
			}
			b, _ := json.Marshal(snap)
			q, _ := json.Marshal(req)
			if _, e = s.db.Exec(`UPDATE governance_previews SET preview_json=?,payload_json=? WHERE token=?`, string(b), string(q), p.PreviewToken); e != nil {
				t.Fatal("snapshot injection")
			}
			if _, e = s.ListGovernanceBatches(ctx, domain.GovernanceBatchQuery{}); e == nil {
				t.Fatal("inconsistent history accepted")
			}
			if auditCount(t, s) != 2 {
				t.Fatal("read query changed audit")
			}
			var state string
			if s.db.QueryRow(`SELECT state FROM governance_batches WHERE id=?`, r.BatchID).Scan(&state) != nil || state != "COMPLETE" {
				t.Fatal("query repaired state")
			}
			checkSourceBytes(t, before)
		})
	}
}
