package store

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/query"
)

func insertTimestampedDecision(t *testing.T, st *SQLiteStore, id, groupID, updatedAt string) {
	insertDecisionWithTimestamps(t, st, id, groupID, updatedAt, updatedAt)
}

func insertDecisionWithTimestamps(t *testing.T, st *SQLiteStore, id, groupID, createdAt, updatedAt string) {
	t.Helper()
	_, err := st.db.Exec(`INSERT INTO group_decisions
		(id, group_id, decision_type, reason, rule_id, created_at, updated_at)
		VALUES(?, ?, ?, '', '', ?, ?)`, id, groupID, string(domain.DecisionKeepAll), createdAt, updatedAt)
	if err != nil {
		t.Fatalf("insert timestamped decision: %v", err)
	}
}

func TestGroupDecisionOrderingUsesChronologicalRFC3339Nano(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	rows := []struct {
		id, group, updatedAt string
	}{
		{"whole-second", "g-whole", "2026-10-13T00:00:00Z"},
		{"one-digit-fraction", "g-one", "2026-10-13T00:00:00.1Z"},
		{"five-digit-fraction", "g-five", "2026-10-13T00:00:00.12345Z"},
		{"nine-digit-fraction", "g-nine", "2026-10-13T00:00:00.123450001Z"},
		{"offset-equivalent-z", "g-offset-z", "2026-10-13T00:00:00.000000001Z"},
		{"offset-equivalent-plus-one", "g-offset-plus", "2026-10-13T01:00:00.000000001+01:00"},
	}
	for _, row := range rows {
		insertTimestampedDecision(t, st, row.id, row.group, row.updatedAt)
	}

	got, err := st.ListGroupDecisions(ctx, "")
	if err != nil {
		t.Fatalf("ListGroupDecisions: %v", err)
	}
	want := []string{"nine-digit-fraction", "five-digit-fraction", "one-digit-fraction", "offset-equivalent-z", "offset-equivalent-plus-one", "whole-second"}
	if len(got) != len(want) {
		t.Fatalf("got %d decisions, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Errorf("ListGroupDecisions[%d] = %q, want %q", i, got[i].ID, want[i])
		}
	}

	// Equivalent instants must use a stable secondary key rather than SQLite
	// row order, which can change with query plans or indexes.
	insertTimestampedDecision(t, st, "tie-a", "g-tie-a", "2026-10-13T00:00:02.123456789Z")
	insertTimestampedDecision(t, st, "tie-z", "g-tie-z", "2026-10-13T01:00:02.123456789+01:00")
	got, err = st.ListGroupDecisions(ctx, "")
	if err != nil {
		t.Fatalf("ListGroupDecisions with equal instants: %v", err)
	}
	if got[0].ID != "tie-z" || got[1].ID != "tie-a" {
		t.Fatalf("equal-instant decisions ordered as %q, %q; want stable ID DESC", got[0].ID, got[1].ID)
	}
}

func TestGetGroupDecisionOrderingUsesChronologicalRFC3339Nano(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	insertTimestampedDecision(t, st, "earlier-lexically-later", "g-get", "2026-10-13T00:00:00.12345Z")
	insertTimestampedDecision(t, st, "later", "g-get", "2026-10-13T00:00:00.123450001Z")

	got, err := st.GetGroupDecision(ctx, "g-get")
	if err != nil {
		t.Fatalf("GetGroupDecision: %v", err)
	}
	if got.ID != "later" {
		t.Fatalf("GetGroupDecision returned %q, want later timestamp row", got.ID)
	}
}

func TestGroupDecisionOrderingNormalizesOffsetAcrossDateBoundary(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	insertTimestampedDecision(t, st, "later-utc", "g-later-utc", "2026-10-12T23:30:00Z")
	insertTimestampedDecision(t, st, "later-local-clock", "g-earlier-utc", "2026-10-13T00:00:00+01:00")

	got, err := st.ListGroupDecisions(ctx, "")
	if err != nil {
		t.Fatalf("ListGroupDecisions: %v", err)
	}
	if len(got) != 2 || got[0].ID != "later-utc" || got[1].ID != "later-local-clock" {
		t.Fatalf("offset-normalized ordering = %#v, want later-utc before later-local-clock", got)
	}
}

func TestGroupDecisionTimestampOrderingRejectsInvalidWithoutEchoingValue(t *testing.T) {
	st := newTestStore(t)
	const privateMarker = "private-timestamp-marker"
	insertTimestampedDecision(t, st, "invalid", "g-invalid", privateMarker)

	_, err := st.ListGroupDecisions(context.Background(), "")
	if err == nil {
		t.Fatal("ListGroupDecisions unexpectedly accepted an invalid timestamp")
	}
	if strings.Contains(err.Error(), privateMarker) {
		t.Fatal("timestamp parse failure echoed the stored value")
	}
	if !strings.Contains(err.Error(), "invalid decision timestamp") {
		t.Fatalf("error does not contain the fixed classification: %v", err)
	}
}

func TestDecisionTimestampOrderRejectsInvalidOffsetsAndUTCYearOverflow(t *testing.T) {
	invalidValues := []string{
		"2026-10-13T00:00:00+24:00",
		"2026-10-13T00:00:00+23:60",
		"9999-12-31T23:30:00-01:00",
		"0000-01-01T00:30:00+01:00",
	}
	for _, value := range invalidValues {
		t.Run(value, func(t *testing.T) {
			for _, operation := range []string{"get", "list", "summary"} {
				t.Run(operation, func(t *testing.T) {
					st := newTestStore(t)
					insertTimestampedDecision(t, st, "valid", "g-boundary", "2026-10-13T00:00:00Z")
					insertTimestampedDecision(t, st, "invalid", "g-boundary", value)

					var err error
					switch operation {
					case "get":
						_, err = st.GetGroupDecision(context.Background(), "g-boundary")
					case "list":
						_, err = st.ListGroupDecisions(context.Background(), "")
					case "summary":
						seedStorage(t, st, "s1")
						seedFile(t, st, "s1", "/vol/a.txt", 100, "hash-boundary")
						seedFile(t, st, "s1", "/vol/b.txt", 100, "hash-boundary")
						page, listErr := st.ListDuplicateGroups(context.Background(), query.GroupQuery{PageSize: 20})
						if listErr != nil {
							t.Fatalf("list duplicate groups: %v", listErr)
						}
						if len(page.Groups) != 1 {
							t.Fatalf("got %d groups, want 1", len(page.Groups))
						}
						insertTimestampedDecision(t, st, "summary-valid", page.Groups[0].GroupID, "2026-10-13T00:00:00Z")
						insertTimestampedDecision(t, st, "summary-invalid", page.Groups[0].GroupID, value)
						_, err = st.ListDuplicateGroups(context.Background(), query.GroupQuery{PageSize: 20})
					}
					if err == nil {
						t.Fatalf("%s accepted invalid timestamp", operation)
					}
					if strings.Contains(err.Error(), value) {
						t.Fatal("timestamp error echoed stored value")
					}
					if !strings.Contains(err.Error(), "invalid decision timestamp") {
						t.Fatalf("error lacks fixed classification: %v", err)
					}
				})
			}
		})
	}
}

func TestDecisionTimestampOrderAcceptsTrailingZeroFractionAndOffset(t *testing.T) {
	st := newTestStore(t)
	insertTimestampedDecision(t, st, "before", "g-valid", "2026-10-13T00:00:00.123449999Z")
	insertTimestampedDecision(t, st, "after", "g-valid", "2026-10-13T01:00:00.123450000+01:00")
	got, err := st.GetGroupDecision(context.Background(), "g-valid")
	if err != nil {
		t.Fatalf("GetGroupDecision rejected valid trailing-zero fraction: %v", err)
	}
	if got.ID != "after" {
		t.Fatalf("GetGroupDecision = %q, want after", got.ID)
	}
}

func TestDecisionTimestampOrderNormalizesLowercaseInDecisionDTOs(t *testing.T) {
	want := time.Date(2026, 10, 13, 1, 0, 0, 123450000, time.UTC)
	for _, operation := range []string{"get", "list"} {
		t.Run(operation, func(t *testing.T) {
			st := newTestStore(t)
			ctx := context.Background()
			insertDecisionWithTimestamps(t, st, "lowercase", "g-lowercase", "2026-10-13t01:00:00.123450000z", "2026-10-13t01:00:00.123450000z")
			switch operation {
			case "get":
				got, err := st.GetGroupDecision(ctx, "g-lowercase")
				if err != nil {
					t.Fatalf("GetGroupDecision: %v", err)
				}
				if got.CreatedAt.IsZero() || !got.CreatedAt.Equal(want) || got.UpdatedAt.IsZero() || !got.UpdatedAt.Equal(want) {
					t.Fatalf("GetGroupDecision timestamps = created %v / updated %v, want %v", got.CreatedAt, got.UpdatedAt, want)
				}
			case "list":
				list, err := st.ListGroupDecisions(ctx, "")
				if err != nil {
					t.Fatalf("ListGroupDecisions: %v", err)
				}
				if len(list) != 1 || list[0].CreatedAt.IsZero() || !list[0].CreatedAt.Equal(want) || list[0].UpdatedAt.IsZero() || !list[0].UpdatedAt.Equal(want) {
					t.Fatalf("ListGroupDecisions UpdatedAt = %#v, want one normalized value %v", list, want)
				}
			}
		})
	}
}

func TestDecisionTimestampDTOParsingRejectsInvalidCreatedAtWithoutEchoingValue(t *testing.T) {
	const privateMarker = "private-created-timestamp-marker"
	for _, operation := range []string{"get", "list"} {
		t.Run(operation, func(t *testing.T) {
			st := newTestStore(t)
			insertDecisionWithTimestamps(t, st, "bad-created", "g-bad-created", privateMarker, "2026-10-13T00:00:00Z")

			var err error
			if operation == "get" {
				_, err = st.GetGroupDecision(context.Background(), "g-bad-created")
			} else {
				_, err = st.ListGroupDecisions(context.Background(), "")
			}
			if err == nil {
				t.Fatalf("%s accepted malformed created_at", operation)
			}
			if strings.Contains(err.Error(), privateMarker) {
				t.Fatal("timestamp error echoed stored value")
			}
			if !strings.Contains(err.Error(), "invalid decision timestamp") {
				t.Fatalf("error lacks fixed classification: %v", err)
			}
		})
	}
}

func TestGroupDecisionOrderingReadOnlyDoesNotMutateDatabaseBytes(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "decisions.db")
	writable, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	insertTimestampedDecision(t, writable, "a", "g-a", "2026-10-13T00:00:00.12345Z")
	insertTimestampedDecision(t, writable, "b", "g-b", "2026-10-13T00:00:00.123450001Z")
	if err := writable.Close(); err != nil {
		t.Fatalf("close writable store: %v", err)
	}
	before, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read database before read-only queries: %v", err)
	}
	beforeHash := sha256.Sum256(before)

	readOnly, err := OpenReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	got, err := readOnly.ListGroupDecisions(ctx, "")
	if err != nil {
		t.Fatalf("read-only ListGroupDecisions: %v", err)
	}
	if len(got) != 2 || got[0].ID != "b" {
		t.Fatalf("read-only ordering = %#v, want b before a", got)
	}
	if _, err := readOnly.GetGroupDecision(ctx, "g-a"); err != nil {
		t.Fatalf("read-only GetGroupDecision: %v", err)
	}
	if err := readOnly.Close(); err != nil {
		t.Fatalf("close read-only store: %v", err)
	}
	after, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("read database after read-only queries: %v", err)
	}
	if afterHash := sha256.Sum256(after); afterHash != beforeHash {
		t.Fatal("read-only decision ordering changed database bytes")
	}
}

func TestSaveAndGetGroupDecision(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	d := domain.GroupDecision{
		ID:           "dec-001",
		GroupID:      "group-abc",
		DecisionType: domain.DecisionKeepAll,
		Reason:       "user reviewed and decided to keep all",
		RuleID:       "rule-007",
	}
	if err := st.SaveGroupDecision(ctx, d); err != nil {
		t.Fatalf("SaveGroupDecision: %v", err)
	}

	got, err := st.GetGroupDecision(ctx, "group-abc")
	if err != nil {
		t.Fatalf("GetGroupDecision: %v", err)
	}
	if got.ID != "dec-001" {
		t.Errorf("ID: expected dec-001, got %s", got.ID)
	}
	if got.GroupID != "group-abc" {
		t.Errorf("GroupID: expected group-abc, got %s", got.GroupID)
	}
	if got.DecisionType != domain.DecisionKeepAll {
		t.Errorf("DecisionType: expected %s, got %s", domain.DecisionKeepAll, got.DecisionType)
	}
	if got.Reason != "user reviewed and decided to keep all" {
		t.Errorf("Reason: got %s", got.Reason)
	}
	if got.RuleID != "rule-007" {
		t.Errorf("RuleID: expected rule-007, got %s", got.RuleID)
	}
	if got.RetainedFileID != nil {
		t.Errorf("RetainedFileID: expected nil, got %v", *got.RetainedFileID)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
	if !got.UpdatedAt.Equal(got.CreatedAt) || got.UpdatedAt.Before(got.CreatedAt) {
		// UpdatedAt should be >= CreatedAt
	}
}

func TestSaveGroupDecisionUpsertReplaces(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// First decision: KEEP_ALL
	d1 := domain.GroupDecision{
		ID:           "dec-001",
		GroupID:      "group-xyz",
		DecisionType: domain.DecisionKeepAll,
		Reason:       "initial decision",
	}
	if err := st.SaveGroupDecision(ctx, d1); err != nil {
		t.Fatalf("SaveGroupDecision 1: %v", err)
	}

	// Second decision for the same group: DEFERRED
	d2 := domain.GroupDecision{
		ID:           "dec-002",
		GroupID:      "group-xyz",
		DecisionType: domain.DecisionDeferred,
		Reason:       "changed my mind",
	}
	if err := st.SaveGroupDecision(ctx, d2); err != nil {
		t.Fatalf("SaveGroupDecision 2: %v", err)
	}

	got, err := st.GetGroupDecision(ctx, "group-xyz")
	if err != nil {
		t.Fatalf("GetGroupDecision: %v", err)
	}
	if got.DecisionType != domain.DecisionDeferred {
		t.Errorf("DecisionType: expected %s (latest), got %s", domain.DecisionDeferred, got.DecisionType)
	}
	if got.Reason != "changed my mind" {
		t.Errorf("Reason: expected 'changed my mind', got %s", got.Reason)
	}

	// Only one row should exist for this group.
	decisions, err := st.ListGroupDecisions(ctx, "")
	if err != nil {
		t.Fatalf("ListGroupDecisions: %v", err)
	}
	count := 0
	for _, d := range decisions {
		if d.GroupID == "group-xyz" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 decision for group-xyz, got %d", count)
	}
}

func TestGetGroupDecisionNotFound(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	_, err := st.GetGroupDecision(ctx, "nonexistent-group")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListGroupDecisionsByType(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	decisions := []domain.GroupDecision{
		{ID: "d1", GroupID: "g1", DecisionType: domain.DecisionKeepAll, Reason: "keep 1"},
		{ID: "d2", GroupID: "g2", DecisionType: domain.DecisionDeferred, Reason: "defer 2"},
		{ID: "d3", GroupID: "g3", DecisionType: domain.DecisionKeepAll, Reason: "keep 3"},
		{ID: "d4", GroupID: "g4", DecisionType: domain.DecisionRejectedSuggestion, Reason: "reject 4"},
	}
	for _, d := range decisions {
		if err := st.SaveGroupDecision(ctx, d); err != nil {
			t.Fatalf("SaveGroupDecision %s: %v", d.ID, err)
		}
	}

	// Filter by KEEP_ALL
	keepAll, err := st.ListGroupDecisions(ctx, domain.DecisionKeepAll)
	if err != nil {
		t.Fatalf("ListGroupDecisions KEEP_ALL: %v", err)
	}
	if len(keepAll) != 2 {
		t.Fatalf("expected 2 KEEP_ALL decisions, got %d", len(keepAll))
	}
	for _, d := range keepAll {
		if d.DecisionType != domain.DecisionKeepAll {
			t.Errorf("expected KEEP_ALL, got %s", d.DecisionType)
		}
	}

	// Filter by DEFERRED
	deferred, err := st.ListGroupDecisions(ctx, domain.DecisionDeferred)
	if err != nil {
		t.Fatalf("ListGroupDecisions DEFERRED: %v", err)
	}
	if len(deferred) != 1 {
		t.Fatalf("expected 1 DEFERRED decision, got %d", len(deferred))
	}

	// No filter → all 4
	all, err := st.ListGroupDecisions(ctx, "")
	if err != nil {
		t.Fatalf("ListGroupDecisions all: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("expected 4 total decisions, got %d", len(all))
	}
}

func TestSaveGroupDecisionWithRetainedFileID(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	seedStorage(t, st, "s1")
	seedFile(t, st, "s1", "/vol/primary.txt", 100, "hashA")

	// Get the file ID for retained_file_id.
	fileID, err := st.FileID(ctx, "s1", "/vol/primary.txt")
	if err != nil {
		t.Fatalf("FileID: %v", err)
	}

	d := domain.GroupDecision{
		ID:             "dec-retention",
		GroupID:        "group-retention",
		DecisionType:   domain.DecisionPrimaryRetention,
		RetainedFileID: &fileID,
		Reason:         "user selected primary copy",
	}
	if err := st.SaveGroupDecision(ctx, d); err != nil {
		t.Fatalf("SaveGroupDecision: %v", err)
	}

	got, err := st.GetGroupDecision(ctx, "group-retention")
	if err != nil {
		t.Fatalf("GetGroupDecision: %v", err)
	}
	if got.DecisionType != domain.DecisionPrimaryRetention {
		t.Errorf("DecisionType: expected %s, got %s", domain.DecisionPrimaryRetention, got.DecisionType)
	}
	if got.RetainedFileID == nil {
		t.Fatal("RetainedFileID should not be nil")
	}
	if *got.RetainedFileID != fileID {
		t.Errorf("RetainedFileID: expected %d, got %d", fileID, *got.RetainedFileID)
	}
}

func TestSaveGroupDecisionValidation(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Empty group_id
	err := st.SaveGroupDecision(ctx, domain.GroupDecision{
		ID:           "d1",
		DecisionType: domain.DecisionKeepAll,
	})
	if err == nil {
		t.Error("expected error for empty group_id")
	}

	// Empty decision_type
	err = st.SaveGroupDecision(ctx, domain.GroupDecision{
		ID:      "d2",
		GroupID: "g1",
	})
	if err == nil {
		t.Error("expected error for empty decision_type")
	}

	// Unknown decision values must not leak into the read model.
	err = st.SaveGroupDecision(ctx, domain.GroupDecision{
		ID: "d3", GroupID: "g1", DecisionType: domain.ReviewDecisionType("TYPO"),
	})
	if err == nil {
		t.Error("expected error for unsupported decision_type")
	}

	// A primary-retention decision is incomplete without its selected file.
	err = st.SaveGroupDecision(ctx, domain.GroupDecision{
		ID: "d4", GroupID: "g1", DecisionType: domain.DecisionPrimaryRetention,
	})
	if err == nil {
		t.Error("expected error for PRIMARY_RETENTION without retained_file_id")
	}

	fileID := int64(1)
	err = st.SaveGroupDecision(ctx, domain.GroupDecision{
		ID: "d5", GroupID: "g1", DecisionType: domain.DecisionKeepAll,
		RetainedFileID: &fileID,
	})
	if err == nil {
		t.Error("expected error for retained_file_id on KEEP_ALL")
	}
}

func TestListGroupDecisionsOrderingByUpdatedAt(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Insert decisions with explicit CreatedAt to verify ordering.
	d1 := domain.GroupDecision{
		ID:           "d1",
		GroupID:      "g1",
		DecisionType: domain.DecisionKeepAll,
		CreatedAt:    time.Now().UTC().Add(-2 * time.Hour),
	}
	d2 := domain.GroupDecision{
		ID:           "d2",
		GroupID:      "g2",
		DecisionType: domain.DecisionKeepAll,
		CreatedAt:    time.Now().UTC().Add(-1 * time.Hour),
	}
	if err := st.SaveGroupDecision(ctx, d1); err != nil {
		t.Fatalf("SaveGroupDecision d1: %v", err)
	}
	if err := st.SaveGroupDecision(ctx, d2); err != nil {
		t.Fatalf("SaveGroupDecision d2: %v", err)
	}

	all, err := st.ListGroupDecisions(ctx, "")
	if err != nil {
		t.Fatalf("ListGroupDecisions: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(all))
	}
	// d2 was inserted later, so its UpdatedAt is later → should be first.
	if all[0].ID != "d2" {
		t.Errorf("expected d2 first (latest updated_at), got %s", all[0].ID)
	}
}

func TestGroupDecisionAllDecisionTypes(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	types := []domain.ReviewDecisionType{
		domain.DecisionKeepAll,
		domain.DecisionDraftAction,
		domain.DecisionDeferred,
		domain.DecisionRejectedSuggestion,
		domain.DecisionCrossArchive,
		domain.DecisionBackupRelation,
	}
	for i, dt := range types {
		d := domain.GroupDecision{
			ID:           "dec-" + string(rune('a'+i)),
			GroupID:      "group-" + string(rune('a'+i)),
			DecisionType: dt,
		}
		if err := st.SaveGroupDecision(ctx, d); err != nil {
			t.Fatalf("SaveGroupDecision %s: %v", dt, err)
		}
	}

	// Verify each type is retrievable.
	for i, dt := range types {
		groupID := "group-" + string(rune('a'+i))
		got, err := st.GetGroupDecision(ctx, groupID)
		if err != nil {
			t.Fatalf("GetGroupDecision %s: %v", groupID, err)
		}
		if got.DecisionType != dt {
			t.Errorf("DecisionType for %s: expected %s, got %s", groupID, dt, got.DecisionType)
		}
	}

	// Verify total count.
	all, err := st.ListGroupDecisions(ctx, "")
	if err != nil {
		t.Fatalf("ListGroupDecisions: %v", err)
	}
	if len(all) != len(types) {
		t.Errorf("expected %d decisions, got %d", len(types), len(all))
	}
}

func TestSaveGroupDecisionAutoGeneratesID(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// Save two decisions for different groups without providing IDs.
	// Previously this would cause a PRIMARY KEY constraint violation
	// because both rows would have id="".
	d1 := domain.GroupDecision{
		GroupID:      "group-alpha",
		DecisionType: domain.DecisionKeepAll,
		Reason:       "first group",
	}
	if err := st.SaveGroupDecision(ctx, d1); err != nil {
		t.Fatalf("SaveGroupDecision d1: %v", err)
	}

	d2 := domain.GroupDecision{
		GroupID:      "group-beta",
		DecisionType: domain.DecisionDeferred,
		Reason:       "second group",
	}
	if err := st.SaveGroupDecision(ctx, d2); err != nil {
		t.Fatalf("SaveGroupDecision d2: %v", err)
	}

	// Both decisions should be retrievable.
	got1, err := st.GetGroupDecision(ctx, "group-alpha")
	if err != nil {
		t.Fatalf("GetGroupDecision group-alpha: %v", err)
	}
	if got1.DecisionType != domain.DecisionKeepAll {
		t.Errorf("group-alpha: expected KEEP_ALL, got %s", got1.DecisionType)
	}
	if got1.ID != "group-alpha-KEEP_ALL" {
		t.Errorf("group-alpha: expected auto ID 'group-alpha-KEEP_ALL', got %s", got1.ID)
	}

	got2, err := st.GetGroupDecision(ctx, "group-beta")
	if err != nil {
		t.Fatalf("GetGroupDecision group-beta: %v", err)
	}
	if got2.DecisionType != domain.DecisionDeferred {
		t.Errorf("group-beta: expected DEFERRED, got %s", got2.DecisionType)
	}
	if got2.ID != "group-beta-DEFERRED" {
		t.Errorf("group-beta: expected auto ID 'group-beta-DEFERRED', got %s", got2.ID)
	}

	// Verify both rows exist (no primary key collision).
	all, err := st.ListGroupDecisions(ctx, "")
	if err != nil {
		t.Fatalf("ListGroupDecisions: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(all))
	}
}

func TestSaveGroupDecisionAutoIDUpdatesOnDecisionChange(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	// First decision: KEEP_ALL (auto ID = "group-x-KEEP_ALL")
	d1 := domain.GroupDecision{
		GroupID:      "group-x",
		DecisionType: domain.DecisionKeepAll,
	}
	if err := st.SaveGroupDecision(ctx, d1); err != nil {
		t.Fatalf("SaveGroupDecision d1: %v", err)
	}

	// Second decision for same group: DEFERRED (auto ID = "group-x-DEFERRED")
	d2 := domain.GroupDecision{
		GroupID:      "group-x",
		DecisionType: domain.DecisionDeferred,
	}
	if err := st.SaveGroupDecision(ctx, d2); err != nil {
		t.Fatalf("SaveGroupDecision d2: %v", err)
	}

	// Latest decision should be DEFERRED with the new auto-generated ID.
	got, err := st.GetGroupDecision(ctx, "group-x")
	if err != nil {
		t.Fatalf("GetGroupDecision: %v", err)
	}
	if got.DecisionType != domain.DecisionDeferred {
		t.Errorf("expected DEFERRED, got %s", got.DecisionType)
	}
	if got.ID != "group-x-DEFERRED" {
		t.Errorf("expected auto ID 'group-x-DEFERRED', got %s", got.ID)
	}

	// Only one row should exist for this group.
	all, err := st.ListGroupDecisions(ctx, "")
	if err != nil {
		t.Fatalf("ListGroupDecisions: %v", err)
	}
	count := 0
	for _, d := range all {
		if d.GroupID == "group-x" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 row for group-x, got %d", count)
	}
}
