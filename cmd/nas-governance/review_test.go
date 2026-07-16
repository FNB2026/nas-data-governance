package main

import (
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// TestExtractKeywords 验证关键词提取逻辑。
func TestExtractKeywords(t *testing.T) {
	cases := []struct {
		id, def string
		want    []string
	}{
		{
			id:   "protect-deliverables",
			def:  "matches deliverables directory",
			want: []string{"protect-deliverables", "matches", "deliverables", "directory"},
		},
		{
			id:   "cleanup-temp",
			def:  "matches temp files",
			want: []string{"cleanup-temp", "matches", "temp", "files"},
		},
		{
			id:   "a",
			def:  "",
			want: nil, // "a" too short (< 3 chars)
		},
	}
	for _, c := range cases {
		got := extractKeywords(c.id, c.def)
		if c.want == nil {
			if len(got) != 0 {
				t.Errorf("extractKeywords(%q, %q) = %v, want empty", c.id, c.def, got)
			}
			continue
		}
		// Check that all expected keywords are present (order may vary due to splitting)
		gotSet := make(map[string]bool)
		for _, k := range got {
			gotSet[k] = true
		}
		for _, w := range c.want {
			if !gotSet[w] {
				t.Errorf("extractKeywords(%q, %q) missing %q, got %v", c.id, c.def, w, got)
			}
		}
	}
}

// TestKeywordOverlap 验证关键词交集计算。
func TestKeywordOverlap(t *testing.T) {
	a := []string{"deliverables", "project", "archive"}
	b := []string{"deliverables", "temp", "cache"}
	overlap := keywordOverlap(a, b)
	if len(overlap) != 1 || overlap[0] != "deliverables" {
		t.Fatalf("expected [deliverables], got %v", overlap)
	}
}

// TestKeywordOverlapEmpty 验证无交集时返回空。
func TestKeywordOverlapEmpty(t *testing.T) {
	a := []string{"alpha", "beta"}
	b := []string{"gamma", "delta"}
	overlap := keywordOverlap(a, b)
	if len(overlap) != 0 {
		t.Fatalf("expected empty overlap, got %v", overlap)
	}
}

// TestDetectRuleConflicts 验证冲突检测能发现共享关键词的规则对。
func TestDetectRuleConflicts(t *testing.T) {
	protect := []domain.Rule{
		{ID: "protect-deliverables", Priority: 95, Enabled: true, Definition: "protect deliverables dir"},
		{ID: "protect-archive", Priority: 90, Enabled: true, Definition: "protect archive dir"},
	}
	clean := []domain.Rule{
		{ID: "cleanup-deliverables", Priority: 50, Enabled: true, Definition: "clean deliverables duplicates"},
		{ID: "cleanup-temp", Priority: 40, Enabled: true, Definition: "clean temp files"},
	}
	conflicts := detectRuleConflicts(protect, clean)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict (shared 'deliverables'), got %d", len(conflicts))
	}
	c := conflicts[0]
	if c.ProtectID != "protect-deliverables" || c.CleanID != "cleanup-deliverables" {
		t.Fatalf("expected protect-deliverables vs cleanup-deliverables, got %s vs %s",
			c.ProtectID, c.CleanID)
	}
}

// TestDetectRuleConflictsNoOverlap 验证无共享关键词时不报冲突。
func TestDetectRuleConflictsNoOverlap(t *testing.T) {
	protect := []domain.Rule{
		{ID: "protect-deliverables", Priority: 95, Enabled: true, Definition: "protect deliverables"},
	}
	clean := []domain.Rule{
		{ID: "cleanup-temp", Priority: 40, Enabled: true, Definition: "clean temp files"},
	}
	conflicts := detectRuleConflicts(protect, clean)
	if len(conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d: %+v", len(conflicts), conflicts)
	}
}

// TestMergeSuggestionToPlan 验证合并建议转换为 plan 的逻辑。
func TestMergeSuggestionToPlan(t *testing.T) {
	s := domain.MergeSuggestion{
		ID:         "merge-001",
		TargetDir:  "/data/deliverables",
		SourceDirs: []string{"/data/deliverables-copy", "/data/deliverables-backup"},
		Reason:     "similar content",
		Confidence: 0.85,
		Evidence:   []string{"jaccard=0.7"},
	}
	plan := mergeSuggestionToPlan(s)
	if plan.ID != "merge-merge-001" {
		t.Fatalf("expected plan ID merge-merge-001, got %s", plan.ID)
	}
	if plan.State != domain.PlanDraft {
		t.Fatalf("expected DRAFT, got %s", plan.State)
	}
	if plan.Risk != domain.RiskMedium {
		t.Fatalf("expected medium risk, got %s", plan.Risk)
	}
	if len(plan.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(plan.Actions))
	}
	for _, a := range plan.Actions {
		if a.Action != domain.OperationReview {
			t.Fatalf("expected REVIEW action, got %s", a.Action)
		}
		if a.TargetPath != "/data/deliverables" {
			t.Fatalf("expected target /data/deliverables, got %s", a.TargetPath)
		}
	}
	if len(plan.Evidence) != 2 {
		t.Fatalf("expected 2 evidence entries, got %d", len(plan.Evidence))
	}
}
