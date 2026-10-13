package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// The durable old-decision audit must use the same chronological identity as
// the displayed decision. RFC3339Nano strings do not sort chronologically.
func TestDecisionAuditSnapshotSelectsChronologicalLatest(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	for _, item := range []struct{ id, at string }{
		{"older", "2026-10-13T00:00:00.12345Z"},
		{"newer", "2026-10-13T00:00:00.123450001Z"},
	} {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO group_decisions(id,group_id,decision_type,created_at,updated_at,revision) VALUES(?,?,'KEEP_ALL',?,?,1)`, item.id, "synthetic-audit-group", item.at, item.at); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	raw, _, err := decisionAuditSnapshot(ctx, tx, "synthetic-audit-group")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ID != "newer" {
		t.Fatal("audit snapshot did not select chronological latest decision")
	}
}

func TestDecisionAuditSnapshotRejectsInvalidTimestampWithoutSideEffects(t *testing.T) {
	for _, invalid := range []string{
		"synthetic-private-timestamp",
		"2026-10-13T00:00:00+24:00",
		"2026-10-13T00:00:00+23:60",
		"9999-12-31T23:30:00-01:00",
		"0000-01-01T00:30:00+01:00",
	} {
		t.Run(invalid, func(t *testing.T) {
			ctx := context.Background()
			s := newTestStore(t)
			if _, err := s.db.ExecContext(ctx, `INSERT INTO group_decisions(id,group_id,decision_type,created_at,updated_at,revision) VALUES('synthetic-invalid','synthetic-audit-group','KEEP_ALL',?,?,1)`, invalid, invalid); err != nil {
				t.Fatal(err)
			}
			tx, err := s.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			_, _, err = decisionAuditSnapshot(ctx, tx, "synthetic-audit-group")
			if err == nil || !strings.Contains(err.Error(), ErrGovernanceRejected.Error()) || strings.Contains(err.Error(), invalid) {
				t.Fatal("invalid audit timestamp was not safely refused")
			}
			var count int
			if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM governance_decision_audit`).Scan(&count); err != nil || count != 0 {
				t.Fatal("refused read changed durable audit")
			}
			var stored string
			if err = tx.QueryRowContext(ctx, `SELECT updated_at FROM group_decisions WHERE id='synthetic-invalid'`).Scan(&stored); err != nil || stored != invalid {
				t.Fatal("refused read changed historical decision")
			}
		})
	}
}

func TestDecisionAuditSnapshotRejectsInvalidCreatedAt(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	const privateValue = "synthetic-private-created-time"
	if _, err := s.db.ExecContext(ctx, `INSERT INTO group_decisions(id,group_id,decision_type,created_at,updated_at,revision) VALUES('synthetic-invalid','synthetic-audit-group','KEEP_ALL',?,'2026-10-13T00:00:00Z',1)`, privateValue); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	_, _, err = decisionAuditSnapshot(ctx, tx, "synthetic-audit-group")
	if err == nil || strings.Contains(err.Error(), privateValue) {
		t.Fatal("invalid audit creation timestamp was not safely refused")
	}
}

func TestGovernanceChronologicalDecisionIdentityAcrossReadAndWrite(t *testing.T) {
	for _, operation := range []string{"workbench", "batch_unchanged", "single_revision"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			s, refs, before := governanceFixture(t, 1)
			for _, row := range []struct {
				id, kind, reason, at string
				revision             int
			}{
				{"older", "KEEP_ALL", "older reason", "2026-10-13T00:00:00.12345Z", 7},
				{"newer", "DEFERRED", "synthetic reason", "2026-10-13T00:00:00.123450001Z", 9},
			} {
				if _, err := s.db.ExecContext(ctx, `INSERT INTO group_decisions(id,group_id,decision_type,reason,created_at,updated_at,revision) VALUES(?,?,?,?,?,?,?)`, row.id, refs[0].GroupID, row.kind, row.reason, row.at, row.at, row.revision); err != nil {
					t.Fatal(err)
				}
			}
			switch operation {
			case "workbench":
				page, err := s.ListGovernanceGroups(ctx, domain.GovernanceQuery{DecisionType: "DEFERRED", PageSize: 20})
				if err != nil || len(page.Items) != 1 || page.Items[0].DecisionType != "DEFERRED" || page.Items[0].DecisionRevision != 9 {
					t.Fatal("workbench did not select chronological decision identity")
				}
			case "single_revision":
				if err := s.SaveGroupDecision(ctx, domain.GroupDecision{GroupID: refs[0].GroupID, DecisionType: domain.DecisionKeepAll, Revision: 9}); err != nil {
					t.Fatal("latest chronological revision was refused")
				}
				var revision int
				if err := s.db.QueryRow(`SELECT revision FROM group_decisions WHERE group_id=?`, refs[0].GroupID).Scan(&revision); err != nil || revision != 10 {
					t.Fatal("wrong durable revision")
				}
			case "batch_unchanged":
				page, err := s.ListGovernanceGroups(ctx, domain.GovernanceQuery{PageSize: 20})
				if err != nil || len(page.Items) != 1 {
					t.Fatal("workbench unavailable")
				}
				p := previewFor(t, s, []domain.GovernanceSubject{page.Items[0].GovernanceSubject}, domain.DecisionDeferred)
				result, err := s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: governanceTestKey(t)})
				if err != nil || result.Counts.Unchanged != 1 || result.Counts.Saved != 0 {
					t.Fatal("unchanged latest decision was incorrectly rewritten")
				}
				var raw string
				if err := s.db.QueryRow(`SELECT old_decision FROM governance_decision_audit`).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var old struct {
					ID string `json:"id"`
				}
				if json.Unmarshal([]byte(raw), &old) != nil || old.ID != "newer" {
					t.Fatal("durable audit used wrong old decision")
				}
			}
			checkSourceBytes(t, before)
		})
	}
}
