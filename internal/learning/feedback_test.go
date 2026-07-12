package learning

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/store"
)

// seedTaskWithPlans creates a task and saves the given plans under it.
func seedTaskWithPlans(t *testing.T, st *store.SQLiteStore, taskID string, plans []domain.OperationPlan) {
	t.Helper()
	ctx := context.Background()
	if err := st.CreateTask(ctx, domain.OperationTask{
		ID: taskID, RootPath: "/vol", State: "executing", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := st.SavePlans(ctx, taskID, plans); err != nil {
		t.Fatalf("save plans: %v", err)
	}
}

// makePlan builds a plan with two actions: one KEEP (retained) and one
// REVIEW/QUARANTINE. The retained copy's AuthorityLevel is set via
// retainAuth; the other copy's AuthorityLevel is set via otherAuth.
func makePlan(id string, state domain.PlanState, retainAuth, otherAuth int, evidence []string) domain.OperationPlan {
	retainScore := domain.RetentionScore{
		Authority:  retainAuth,
		Stability:  10,
		PathDepth:  3,
		RoleBonus:  0,
		Total:      retainAuth + 10 + 3,
	}
	return domain.OperationPlan{
		ID:           id,
		State:        state,
		ContentSHA256: strings.Repeat("a", 64),
		RetainScore:  retainScore,
		Evidence:     evidence,
		Actions: []domain.PlannedAction{
			{
				Action:  domain.OperationKeep,
				Context: domain.DirectoryContext{AuthorityLevel: retainAuth},
			},
			{
				Action:  domain.OperationReview,
				Context: domain.DirectoryContext{AuthorityLevel: otherAuth},
			},
		},
	}
}

// TestLearnFromFeedback_TooFewPlansReturnsEmpty verifies that with fewer
// than MinSamples plans, no adjustments are proposed.
func TestLearnFromFeedback_TooFewPlansReturnsEmpty(t *testing.T) {
	st := newLearnStore(t)
	// 2 plans, below default MinSamples=5.
	seedTaskWithPlans(t, st, "task-1", []domain.OperationPlan{
		makePlan("p1", domain.PlanApproved, 80, 90, nil),
		makePlan("p2", domain.PlanApproved, 70, 95, nil),
	})
	stats, err := LearnFromFeedback(context.Background(), st, FeedbackOptions{MinSamples: 5})
	if err != nil {
		t.Fatalf("LearnFromFeedback: %v", err)
	}
	if stats.PlansAnalyzed != 2 {
		t.Errorf("PlansAnalyzed = %d, want 2", stats.PlansAnalyzed)
	}
	if len(stats.WeightAdjustments) != 0 {
		t.Errorf("expected 0 adjustments for too-few plans, got %d", len(stats.WeightAdjustments))
	}
}

// TestLearnFromFeedback_AuthorityUnderweighted verifies that when the user
// retains lower-authority copies while higher-authority copies exist, the
// authority weight is adjusted upward (+delta), capped at +3.
func TestLearnFromFeedback_AuthorityUnderweighted(t *testing.T) {
	st := newLearnStore(t)
	plans := make([]domain.OperationPlan, 0, 10)
	// 10 plans where retained copy has Authority=50, other copy has Authority=90.
	// User consistently kept the lower-authority copy → authority underweighted.
	for i := 0; i < 10; i++ {
		plans = append(plans, makePlan(
			planID("p", i),
			domain.PlanApproved,
			50, 90, // retained=50, other=90 → underweight
			nil,
		))
	}
	seedTaskWithPlans(t, st, "task-1", plans)

	stats, err := LearnFromFeedback(context.Background(), st, FeedbackOptions{})
	if err != nil {
		t.Fatalf("LearnFromFeedback: %v", err)
	}
	if len(stats.WeightAdjustments) == 0 {
		t.Fatalf("expected at least one weight adjustment, got 0")
	}
	var authAdj *WeightAdjustment
	for i := range stats.WeightAdjustments {
		if stats.WeightAdjustments[i].Component == "authority" {
			authAdj = &stats.WeightAdjustments[i]
		}
	}
	if authAdj == nil {
		t.Fatalf("expected authority adjustment, got: %+v", stats.WeightAdjustments)
	}
	if authAdj.Delta <= 0 {
		t.Errorf("authority delta = %d, want > 0 (underweighted)", authAdj.Delta)
	}
	if authAdj.Delta > 3 {
		t.Errorf("authority delta = %d, exceeds +3 cap (K-008)", authAdj.Delta)
	}
}

// TestLearnFromFeedback_DeltaCappedAt3 verifies the ±3 cap is enforced
// even with extreme discrepancy.
func TestLearnFromFeedback_DeltaCappedAt3(t *testing.T) {
	st := newLearnStore(t)
	plans := make([]domain.OperationPlan, 0, 20)
	for i := 0; i < 20; i++ {
		plans = append(plans, makePlan(
			planID("p", i),
			domain.PlanApproved,
			10, 100, // extreme: retained=10, other=100
			nil,
		))
	}
	seedTaskWithPlans(t, st, "task-1", plans)
	stats, err := LearnFromFeedback(context.Background(), st, FeedbackOptions{})
	if err != nil {
		t.Fatalf("LearnFromFeedback: %v", err)
	}
	for _, adj := range stats.WeightAdjustments {
		if adj.Delta > 3 || adj.Delta < -3 {
			t.Errorf("%s delta = %d, must be within [-3, +3]", adj.Component, adj.Delta)
		}
	}
}

// TestLearnFromFeedback_ConfidenceDowngradeForRejectedRules verifies that
// a learned rule whose term appears in rejected plans' evidence gets a
// confidence downgrade suggestion.
func TestLearnFromFeedback_ConfidenceDowngradeForRejectedRules(t *testing.T) {
	st := newLearnStore(t)
	ctx := context.Background()

	// Seed a learned rule with a known term "竣工图".
	rule := domain.Rule{
		ID:         "learned-dir-竣工图",
		Version:    1,
		Priority:   60,
		Enabled:    true,
		Source:     domain.RuleSourceLearned,
		Confidence: 0.8,
		Status:     domain.RuleDraft,
		Definition: "match:\n  segment_contains: \"竣工图\"\neffect:\n  role: formal_archive\n  authority: 60",
	}
	if err := st.SaveRule(ctx, rule); err != nil {
		t.Fatal(err)
	}

	// 10 plans, 6 rejected (60% rejection rate), all mentioning "竣工图" in evidence.
	plans := make([]domain.OperationPlan, 0, 10)
	for i := 0; i < 10; i++ {
		state := domain.PlanApproved
		if i < 6 {
			state = domain.PlanRolledBack
		}
		plans = append(plans, makePlan(
			planID("p", i),
			state,
			50, 50,
			[]string{"竣工图 相关证据"},
		))
	}
	seedTaskWithPlans(t, st, "task-1", plans)

	stats, err := LearnFromFeedback(ctx, st, FeedbackOptions{MinSamples: 5, RejectionThreshold: 0.5})
	if err != nil {
		t.Fatalf("LearnFromFeedback: %v", err)
	}
	if stats.PlansRejected != 6 {
		t.Errorf("PlansRejected = %d, want 6", stats.PlansRejected)
	}
	var dg *ConfidenceDowngrade
	for i := range stats.ConfidenceDowngrades {
		if stats.ConfidenceDowngrades[i].RuleID == rule.ID {
			dg = &stats.ConfidenceDowngrades[i]
		}
	}
	if dg == nil {
		t.Fatalf("expected confidence downgrade for rule %s, got: %+v", rule.ID, stats.ConfidenceDowngrades)
	}
	if dg.ObservedRejection != 0.6 {
		t.Errorf("ObservedRejection = %f, want 0.6", dg.ObservedRejection)
	}
	if dg.SuggestedDelta > 0 {
		t.Errorf("SuggestedDelta = %f, want <= 0", dg.SuggestedDelta)
	}
	if dg.SuggestedDelta < -0.2 {
		t.Errorf("SuggestedDelta = %f, exceeds -0.2 cap", dg.SuggestedDelta)
	}
}

// TestGenerateFeedbackDrafts_PersistsWeightRules verifies weight adjustments
// are persisted as draft rules with the correct ID prefix.
func TestGenerateFeedbackDrafts_PersistsWeightRules(t *testing.T) {
	st := newLearnStore(t)
	stats := &FeedbackStats{
		PlansAnalyzed: 10,
		WeightAdjustments: []WeightAdjustment{
			{Component: "authority", Delta: 2, Reason: "test +2", Evidence: "e1"},
			{Component: "stability", Delta: -1, Reason: "test -1", Evidence: "e2"},
		},
	}
	rules, err := GenerateFeedbackDrafts(context.Background(), st, stats, "feedback-batch-1")
	if err != nil {
		t.Fatalf("GenerateFeedbackDrafts: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 draft rules, got %d", len(rules))
	}
	// Verify IDs and priority.
	seenAuth, seenStab := false, false
	for _, r := range rules {
		if r.Priority > 60 {
			t.Errorf("rule %s priority %d > 60 (K-008)", r.ID, r.Priority)
		}
		if r.Source != domain.RuleSourceLearned {
			t.Errorf("rule %s source = %s, want learned", r.ID, r.Source)
		}
		if r.Status != domain.RuleDraft {
			t.Errorf("rule %s status = %s, want draft", r.ID, r.Status)
		}
		if r.ID == "learned-weight-authority" {
			seenAuth = true
		}
		if r.ID == "learned-weight-stability" {
			seenStab = true
		}
	}
	if !seenAuth || !seenStab {
		t.Errorf("missing weight rule IDs: auth=%v stab=%v", seenAuth, seenStab)
	}
}

// TestGenerateFeedbackDrafts_AppliesConfidenceDowngrade verifies confidence
// downgrades are applied to draft rules but not approved ones.
func TestGenerateFeedbackDrafts_AppliesConfidenceDowngrade(t *testing.T) {
	st := newLearnStore(t)
	ctx := context.Background()

	draftRule := domain.Rule{
		ID:         "learned-dir-竣工图",
		Version:    1, Priority: 60, Enabled: true,
		Source:     domain.RuleSourceLearned,
		Confidence: 0.8,
		Status:     domain.RuleDraft,
		Definition: "match:\n  segment_contains: \"竣工图\"\neffect:\n  role: formal_archive\n  authority: 60",
	}
	approvedRule := domain.Rule{
		ID:         "learned-dir-deliverables",
		Version:    1, Priority: 60, Enabled: true,
		Source:     domain.RuleSourceLearned,
		Confidence: 0.9,
		Status:     domain.RuleApproved,
		Definition: "match:\n  segment_contains: \"deliverables\"\neffect:\n  role: formal_archive\n  authority: 60",
	}
	if err := st.SaveRule(ctx, draftRule); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveRule(ctx, approvedRule); err != nil {
		t.Fatal(err)
	}

	stats := &FeedbackStats{
		PlansAnalyzed: 10,
		ConfidenceDowngrades: []ConfidenceDowngrade{
			{RuleID: "learned-dir-竣工图", ObservedRejection: 0.6, Samples: 10, SuggestedDelta: -0.12},
			{RuleID: "learned-dir-deliverables", ObservedRejection: 0.7, Samples: 10, SuggestedDelta: -0.14},
		},
	}
	if _, err := GenerateFeedbackDrafts(ctx, st, stats, "fb-batch"); err != nil {
		t.Fatalf("GenerateFeedbackDrafts: %v", err)
	}

	// Draft rule: confidence lowered.
	persisted, _ := st.ListRules(ctx, domain.RuleSourceLearned, "")
	for _, r := range persisted {
		if r.ID == "learned-dir-竣工图" {
			if r.Confidence >= 0.8 {
				t.Errorf("draft rule confidence = %f, want < 0.8 (downgraded)", r.Confidence)
			}
		}
		if r.ID == "learned-dir-deliverables" {
			if r.Confidence != 0.9 {
				t.Errorf("approved rule confidence = %f, want 0.9 (untouched)", r.Confidence)
			}
		}
	}
}

// TestGenerateFeedbackDrafts_PreservesApprovedWeightRules verifies that
// an approved weight-adjustment rule is not overwritten by re-running feedback.
func TestGenerateFeedbackDrafts_PreservesApprovedWeightRules(t *testing.T) {
	st := newLearnStore(t)
	ctx := context.Background()
	approved := domain.Rule{
		ID:         "learned-weight-authority",
		Version:    1, Priority: 40, Enabled: true,
		Source:     domain.RuleSourceLearned,
		Confidence: 0.9,
		Status:     domain.RuleApproved,
		Definition: "match:\n  segment_contains: \"__weight_authority__\"\neffect:\n  role: unorganized\n  authority: 0",
	}
	if err := st.SaveRule(ctx, approved); err != nil {
		t.Fatal(err)
	}
	stats := &FeedbackStats{
		WeightAdjustments: []WeightAdjustment{
			{Component: "authority", Delta: 2, Reason: "r", Evidence: "e"},
		},
	}
	rules, err := GenerateFeedbackDrafts(ctx, st, stats, "new")
	if err != nil {
		t.Fatalf("GenerateFeedbackDrafts: %v", err)
	}
	for _, r := range rules {
		if r.ID == approved.ID {
			t.Fatalf("approved weight rule %s was overwritten", r.ID)
		}
	}
}

// TestLearnFromFeedback_NoPathsInOutput verifies K-009: FeedbackStats
// never contains file paths.
func TestLearnFromFeedback_NoPathsInOutput(t *testing.T) {
	st := newLearnStore(t)
	// Plans with paths in actions (simulating real data).
	plans := []domain.OperationPlan{
		{
			ID:           "p1",
			State:        domain.PlanApproved,
			ContentSHA256: strings.Repeat("a", 64),
			RetainPath:    "/vol/secret/retain.txt",
			RetainScore:   domain.RetentionScore{Authority: 50, Total: 60},
			Actions: []domain.PlannedAction{
				{Action: domain.OperationKeep, Path: "/vol/secret/retain.txt", Context: domain.DirectoryContext{AuthorityLevel: 50}},
				{Action: domain.OperationReview, Path: "/vol/other/copy.txt", Context: domain.DirectoryContext{AuthorityLevel: 90}},
			},
		},
	}
	for i := 1; i < 10; i++ {
		plans = append(plans, plans[0])
		plans[len(plans)-1].ID = planID("p", i)
	}
	seedTaskWithPlans(t, st, "task-1", plans)
	stats, err := LearnFromFeedback(context.Background(), st, FeedbackOptions{})
	if err != nil {
		t.Fatalf("LearnFromFeedback: %v", err)
	}
	for _, adj := range stats.WeightAdjustments {
		if strings.Contains(adj.Reason, "/vol/") || strings.Contains(adj.Evidence, "/vol/") {
			t.Errorf("weight adjustment leaked path: reason=%q evidence=%q", adj.Reason, adj.Evidence)
		}
	}
	for _, dg := range stats.ConfidenceDowngrades {
		if strings.Contains(dg.RuleID, "/vol/") {
			t.Errorf("confidence downgrade leaked path: rule_id=%q", dg.RuleID)
		}
	}
}

func planID(prefix string, i int) string {
	return prefix + "-" + string(rune('0'+i))
}

// ensure filepath import is used (for potential future path-based tests).
var _ = filepath.Join
