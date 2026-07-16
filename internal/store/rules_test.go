package store

import (
	"context"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestSaveAndListRules(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	now := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	r := domain.Rule{
		ID:         "learned-signal-1",
		Version:    1,
		Priority:   60,
		Enabled:    true,
		Source:     domain.RuleSourceLearned,
		BatchID:    "learn-2026-07-11-001",
		Confidence: 0.82,
		Status:     domain.RuleDraft,
		ApprovedAt: &now,
		Definition: `match:
  segment_contains: "竣工图"
effect:
  role: formal_archive
  authority: 90`,
	}
	if err := s.SaveRule(ctx, r); err != nil {
		t.Fatalf("save rule: %v", err)
	}

	// List by source=learned, status=draft.
	rules, err := s.ListRules(ctx, domain.RuleSourceLearned, domain.RuleDraft)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	got := rules[0]
	if got.ID != r.ID {
		t.Errorf("ID = %s, want %s", got.ID, r.ID)
	}
	if got.Source != domain.RuleSourceLearned {
		t.Errorf("Source = %s, want learned", got.Source)
	}
	if got.Status != domain.RuleDraft {
		t.Errorf("Status = %s, want draft", got.Status)
	}
	if got.Confidence != 0.82 {
		t.Errorf("Confidence = %f, want 0.82", got.Confidence)
	}
	if got.BatchID != "learn-2026-07-11-001" {
		t.Errorf("BatchID = %s", got.BatchID)
	}
	if got.ApprovedAt == nil || !got.ApprovedAt.Equal(now) {
		t.Errorf("ApprovedAt mismatch: %v", got.ApprovedAt)
	}
}

func TestUpdateRuleStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := domain.Rule{
		ID: "rule-2", Version: 1, Priority: 60, Enabled: true,
		Source: domain.RuleSourceLearned, Status: domain.RuleDraft,
		Confidence: 0.7, Definition: "match:\n  segment_contains: \"test\"\neffect:\n  role: temporary\n  authority: 20",
	}
	if err := s.SaveRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := s.UpdateRuleStatus(ctx, r.ID, domain.RuleProbation, &now); err != nil {
		t.Fatalf("update status: %v", err)
	}
	rules, _ := s.ListRules(ctx, "", domain.RuleProbation)
	if len(rules) != 1 {
		t.Fatalf("expected 1 probation rule, got %d", len(rules))
	}
	if rules[0].Status != domain.RuleProbation {
		t.Errorf("status = %s, want probation", rules[0].Status)
	}
}

func TestUpdateRuleStatusRejectsMissingAndIllegalTransition(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if err := s.UpdateRuleStatus(ctx, "missing", domain.RuleProbation, nil); err != ErrNotFound {
		t.Fatalf("missing rule: got %v, want ErrNotFound", err)
	}
	r := domain.Rule{
		ID: "rejected-rule", Version: 1, Priority: 50, Enabled: true,
		Source: domain.RuleSourceLearned, Status: domain.RuleRejected,
		Definition: "match:\n  segment_contains: \"x\"\neffect:\n  role: temporary\n  authority: 20",
	}
	if err := s.SaveRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRuleStatus(ctx, r.ID, domain.RuleProbation, nil); err == nil {
		t.Fatal("rejected rule must not be silently re-approved")
	}
}

func TestDisableBatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	batch := "learn-2026-07-11-002"
	for i := 0; i < 3; i++ {
		r := domain.Rule{
			ID: "rule-b-" + string(rune('a'+i)), Version: 1, Priority: 50, Enabled: true,
			Source: domain.RuleSourceLearned, BatchID: batch, Status: domain.RuleApproved,
			Confidence: 0.9, Definition: "match:\n  segment_contains: \"x\"\neffect:\n  role: cache_derived\n  authority: 10",
		}
		if err := s.SaveRule(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	// Add one rule from a different batch — must not be disabled.
	other := domain.Rule{
		ID: "rule-other", Version: 1, Priority: 50, Enabled: true,
		Source: domain.RuleSourceLearned, BatchID: "other-batch", Status: domain.RuleApproved,
		Confidence: 0.9, Definition: "match:\n  segment_contains: \"y\"\neffect:\n  role: cache_derived\n  authority: 10",
	}
	s.SaveRule(ctx, other)

	if err := s.DisableBatch(ctx, batch); err != nil {
		t.Fatalf("disable batch: %v", err)
	}
	disabled, _ := s.ListRules(ctx, "", domain.RuleDisabled)
	if len(disabled) != 3 {
		t.Fatalf("expected 3 disabled rules, got %d", len(disabled))
	}
	// Other batch's rule should still be approved.
	approved, _ := s.ListRules(ctx, "", domain.RuleApproved)
	if len(approved) != 1 {
		t.Fatalf("expected 1 approved (other batch), got %d", len(approved))
	}
}

func TestListRulesNoFilter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		r := domain.Rule{
			ID: "rule-n-" + string(rune('a'+i)), Version: 1, Priority: 60 - i,
			Enabled: true, Source: domain.RuleSourceLearned, Status: domain.RuleApproved,
			Confidence: 1.0, Definition: "match:\n  segment_contains: \"n\"\neffect:\n  role: temporary\n  authority: 20",
		}
		if err := s.SaveRule(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	rules, err := s.ListRules(ctx, "", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rules) != 5 {
		t.Fatalf("expected 5 rules, got %d", len(rules))
	}
	// Should be ordered by priority DESC.
	if rules[0].Priority < rules[4].Priority {
		t.Errorf("expected priority DESC order, got %d before %d", rules[0].Priority, rules[4].Priority)
	}
}

func TestSaveLearningBatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	started := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	b := LearningBatch{
		ID: "learn-2026-07-11-003", Source: "stats",
		StartedAt: started, RuleCount: 0, Status: "running",
	}
	if err := s.SaveLearningBatch(ctx, b); err != nil {
		t.Fatalf("save batch: %v", err)
	}
	// Update to completed.
	completed := time.Date(2026, 7, 11, 10, 5, 0, 0, time.UTC)
	b.RuleCount = 12
	b.Status = "completed"
	b.CompletedAt = &completed
	if err := s.SaveLearningBatch(ctx, b); err != nil {
		t.Fatalf("update batch: %v", err)
	}
}
