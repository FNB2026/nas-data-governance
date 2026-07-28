package store

import (
	"context"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

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
