package dircontext

import (
	"testing"

	"nas-data-governance/internal/domain"
)

func TestBuiltinRulesUnchanged(t *testing.T) {
	// With no learned rules merged, classification must match the original
	// builtin behavior exactly.
	ctx := Classify("/data/backup/file.txt")
	if ctx.Role != domain.RoleBackup {
		t.Errorf("expected backup role, got %s", ctx.Role)
	}
	if !ctx.Protected {
		t.Error("backup should be protected")
	}
}

func TestMergeLearnedAddsNewSignal(t *testing.T) {
	ClearLearned()
	t.Cleanup(ClearLearned)
	r := domain.Rule{
		ID: "learned-1", Enabled: true, Source: domain.RuleSourceLearned,
		Status: domain.RuleApproved,
		Definition: "match:\n  segment_contains: \"竣工图\"\neffect:\n  role: formal_archive\n  authority: 60",
	}
	MergeLearned([]domain.Rule{r})
	ctx := Classify("/data/竣工图/2024/drawing.pdf")
	if ctx.Role != domain.RoleFormalArchive {
		t.Errorf("expected formal_archive from learned rule, got %s", ctx.Role)
	}
}

func TestLearnedRuleNeverOverridesProtection(t *testing.T) {
	ClearLearned()
	t.Cleanup(ClearLearned)
	// Learned rule claims "backup" → but builtin already handles backup with
	// priority 95. The learned rule must not change the matched term set.
	r := domain.Rule{
		ID: "learned-2", Enabled: true, Source: domain.RuleSourceLearned,
		Status: domain.RuleApproved,
		Definition: "match:\n  segment_contains: \"backup\"\neffect:\n  role: temporary\n  authority: 60",
	}
	MergeLearned([]domain.Rule{r})
	ctx := Classify("/data/backup/file.txt")
	// Builtin backup (priority 95) must win over learned (priority 60).
	if ctx.Role != domain.RoleBackup {
		t.Errorf("builtin backup must win, got %s", ctx.Role)
	}
}

func TestLearnedPriorityCapped(t *testing.T) {
	ClearLearned()
	t.Cleanup(ClearLearned)
	// Rule claims authority 200, must be capped at maxLearnedPriority (60).
	r := domain.Rule{
		ID: "learned-3", Enabled: true, Source: domain.RuleSourceLearned,
		Status: domain.RuleApproved,
		Definition: "match:\n  segment_contains: \"customdir\"\neffect:\n  role: project_work\n  authority: 200",
	}
	MergeLearned([]domain.Rule{r})
	ctx := Classify("/data/customdir/file.txt")
	if ctx.Role != domain.RoleProjectWork {
		t.Errorf("expected project_work, got %s", ctx.Role)
	}
	if ctx.AuthorityLevel > maxLearnedPriority {
		t.Errorf("authority %d must be capped at %d", ctx.AuthorityLevel, maxLearnedPriority)
	}
}

func TestDisabledLearnedRuleIgnored(t *testing.T) {
	ClearLearned()
	t.Cleanup(ClearLearned)
	r := domain.Rule{
		ID: "learned-4", Enabled: false, Source: domain.RuleSourceLearned,
		Status: domain.RuleApproved,
		Definition: "match:\n  segment_contains: \"disabledterm\"\neffect:\n  role: temporary\n  authority: 20",
	}
	MergeLearned([]domain.Rule{r})
	ctx := Classify("/data/disabledterm/file.txt")
	if ctx.Role == domain.RoleTemporary {
		t.Errorf("disabled rule should not match, got %s", ctx.Role)
	}
}

func TestDraftLearnedRuleIgnored(t *testing.T) {
	ClearLearned()
	t.Cleanup(ClearLearned)
	r := domain.Rule{
		ID: "learned-5", Enabled: true, Source: domain.RuleSourceLearned,
		Status: domain.RuleDraft,
		Definition: "match:\n  segment_contains: \"draftterm\"\neffect:\n  role: temporary\n  authority: 20",
	}
	MergeLearned([]domain.Rule{r})
	ctx := Classify("/data/draftterm/file.txt")
	if ctx.Role == domain.RoleTemporary {
		t.Errorf("draft rule should not match, got %s", ctx.Role)
	}
}

func TestProbationRuleApplies(t *testing.T) {
	ClearLearned()
	t.Cleanup(ClearLearned)
	r := domain.Rule{
		ID: "learned-6", Enabled: true, Source: domain.RuleSourceLearned,
		Status: domain.RuleProbation,
		Definition: "match:\n  segment_contains: \"probationterm\"\neffect:\n  role: project_work\n  authority: 60",
	}
	MergeLearned([]domain.Rule{r})
	ctx := Classify("/data/probationterm/file.txt")
	if ctx.Role != domain.RoleProjectWork {
		t.Errorf("probation rule should match, got %s", ctx.Role)
	}
}

func TestRuleVersionChanges(t *testing.T) {
	ClearLearned()
	t.Cleanup(ClearLearned)
	v1 := RuleVersion()
	if v1 != "builtin-v1" {
		t.Errorf("no learned rules: version = %s, want builtin-v1", v1)
	}
	r := domain.Rule{
		ID: "learned-7", Enabled: true, Source: domain.RuleSourceLearned,
		Status: domain.RuleApproved,
		Definition: "match:\n  segment_contains: \"versiontest\"\neffect:\n  role: project_work\n  authority: 60",
	}
	MergeLearned([]domain.Rule{r})
	v2 := RuleVersion()
	if v2 == v1 {
		t.Errorf("version should change after merging learned rules")
	}
	ClearLearned()
	v3 := RuleVersion()
	if v3 != v1 {
		t.Errorf("version should revert after clear: %s", v3)
	}
}
