package planner

import (
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"strings"
	"testing"
	"time"
)

func group(paths ...string) []domain.DuplicateGroup {
	files := make([]domain.FileInstance, len(paths))
	for i, path := range paths {
		files[i] = domain.FileInstance{Path: path, Size: 1, ModifiedAt: time.Unix(int64(i), 0)}
	}
	return []domain.DuplicateGroup{{SHA256: strings.Repeat("a", 64), Size: 1, Files: files}}
}

func TestBuildProtectsCrossArchive(t *testing.T) {
	p := Build(group("/家庭/医疗/报告.pdf", "/家庭/临时/报告.pdf"))[0]
	if p.Risk != domain.RiskCritical || p.Actions[0].Action != domain.OperationReview {
		t.Fatalf("got %#v", p)
	}
}

func TestBuildRecommendsQuarantineOnlyForTemporaryDuplicates(t *testing.T) {
	p := Build(group("/download/temp/a.iso", "/download/temp/b.iso"))[0]
	if p.Risk != domain.RiskMedium || p.Actions[0].Action != domain.OperationKeep || p.Actions[1].Action != domain.OperationQuarantine {
		t.Fatalf("got %#v", p)
	}
}

func TestBuildReviewsDifferentRoles(t *testing.T) {
	p := Build(group("/data/归档/a.pdf", "/data/项目/a.pdf"))[0]
	if p.Risk != domain.RiskHigh || p.Actions[0].Action != domain.OperationReview {
		t.Fatalf("got %#v", p)
	}
}

func TestBuildRejectsMalformedFullHashWithoutPanic(t *testing.T) {
	files := []domain.FileInstance{{Path: "/data/temp/a"}, {Path: "/data/temp/b"}}
	plans := Build([]domain.DuplicateGroup{{SHA256: "short", Size: 1, Files: files}})
	if len(plans) != 1 || plans[0].Risk != domain.RiskCritical {
		t.Fatalf("malformed hash must produce critical review plan: %#v", plans)
	}
	for _, action := range plans[0].Actions {
		if action.Action != domain.OperationReview {
			t.Fatalf("malformed hash must never produce filesystem action: %#v", action)
		}
	}
}

func TestBuildReviewsDifferentBusinessAnchors(t *testing.T) {
	// Both paths are RoleProjectWork, but their year anchors differ → review.
	p := Build(group("/data/项目/2023/report.pdf", "/data/项目/2024/report.pdf"))[0]
	if p.Actions[0].Action != domain.OperationReview {
		t.Fatalf("expected review for divergent anchors, got %#v", p.Actions[0])
	}
	if len(p.Evidence) == 0 || p.Evidence[0] != "副本业务锚点不同" {
		t.Fatalf("expected anchor-divergence evidence, got %#v", p.Evidence)
	}
}

func TestBuildAttachesRetentionScoreForKeptCopy(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	plans := BuildAt(group("/download/temp/a.iso", "/download/temp/b.iso"), now)
	p := plans[0]
	if p.RetainScore.Total == 0 {
		t.Fatalf("expected non-zero retain score, got %#v", p.RetainScore)
	}
	if len(p.RetainScore.Reasons) == 0 {
		t.Fatalf("expected explainable reasons, got %#v", p.RetainScore)
	}
	// The first file is older (mtime=Unix(0)) and should win the stability tiebreak.
	if p.RetainPath != "/download/temp/a.iso" {
		t.Fatalf("expected older file retained, got %q", p.RetainPath)
	}
}

func TestScoreRetentionAuthorityDominates(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	high := domain.FileInstance{Path: "/data/归档/2024/a.pdf", ModifiedAt: now.Add(-24 * time.Hour)}
	low := domain.FileInstance{Path: "/download/temp/a.pdf", ModifiedAt: now.Add(-24 * time.Hour)}
	highCtx := domain.DirectoryContext{Role: domain.RoleFormalArchive, AuthorityLevel: 90}
	lowCtx := domain.DirectoryContext{Role: domain.RoleTemporary, AuthorityLevel: 20}
	hs := ScoreRetention(high, highCtx, now)
	ls := ScoreRetention(low, lowCtx, now)
	if hs.Total <= ls.Total {
		t.Fatalf("formal archive must outrank temporary: high=%d low=%d", hs.Total, ls.Total)
	}
	if hs.RoleBonus <= 0 || ls.RoleBonus >= 0 {
		t.Fatalf("role bonus sign wrong: high=%d low=%d", hs.RoleBonus, ls.RoleBonus)
	}
}

func TestScoreRetentionStabilityRewardsAge(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	old := domain.FileInstance{Path: "/a/b.pdf", ModifiedAt: now.Add(-365 * 24 * time.Hour)}
	new := domain.FileInstance{Path: "/a/b.pdf", ModifiedAt: now.Add(-1 * 24 * time.Hour)}
	ctx := domain.DirectoryContext{AuthorityLevel: 50}
	os := ScoreRetention(old, ctx, now)
	ns := ScoreRetention(new, ctx, now)
	if os.Stability <= ns.Stability {
		t.Fatalf("older file must have higher stability: old=%d new=%d", os.Stability, ns.Stability)
	}
	if os.Stability > 30 {
		t.Fatalf("stability must cap at 30, got %d", os.Stability)
	}
}

func TestScoreRetentionRoleBonusAppliesForArchiveRoles(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	f := domain.FileInstance{Path: "/data/归档/a.pdf", ModifiedAt: now}
	for _, tc := range []struct {
		role  domain.DirectoryRole
		bonus int
	}{
		{domain.RoleRaw, 20},
		{domain.RoleFormalArchive, 20},
		{domain.RoleTemporary, -20},
		{domain.RoleCache, -20},
		{domain.RoleUnorganized, 0},
	} {
		ctx := domain.DirectoryContext{Role: tc.role, AuthorityLevel: 50}
		s := ScoreRetention(f, ctx, now)
		if s.RoleBonus != tc.bonus {
			t.Fatalf("role %s: expected bonus %d, got %d", tc.role, tc.bonus, s.RoleBonus)
		}
	}
}

func TestAnchorsDivergeIgnoresEmptyAnchors(t *testing.T) {
	// Two empty anchors → no divergence.
	if anchorsDiverge([]domain.DirectoryContext{{}, {}}) {
		t.Fatal("expected no divergence with all empty anchors")
	}
	// One anchor, one empty → still no divergence (single anchor not enough).
	if anchorsDiverge([]domain.DirectoryContext{{BusinessAnchor: "2024"}, {}}) {
		t.Fatal("expected no divergence with single anchor")
	}
}

func TestAnchorsDivergeDetectsDifferentAnchors(t *testing.T) {
	c1 := domain.DirectoryContext{BusinessAnchor: "2024"}
	c2 := domain.DirectoryContext{BusinessAnchor: "2025"}
	if !anchorsDiverge([]domain.DirectoryContext{c1, c2}) {
		t.Fatal("expected divergence between 2024 and 2025")
	}
}

func TestAnchorsDivergeAllowsSameAnchor(t *testing.T) {
	c1 := domain.DirectoryContext{BusinessAnchor: "PRJ-2024-001"}
	c2 := domain.DirectoryContext{BusinessAnchor: "PRJ-2024-001"}
	if anchorsDiverge([]domain.DirectoryContext{c1, c2}) {
		t.Fatal("same anchor must not diverge")
	}
}

// TestBuildDefaultReviewForSameRoleNonTemporary 验证 buildGroup 的默认分支6：
// 所有副本同角色、同锚点、非临时非缓存时，生成 REVIEW action。
// 此前没有任何测试走到这个分支。
func TestBuildDefaultReviewForSameRoleNonTemporary(t *testing.T) {
	// 两个 RoleProjectWork 路径（"项目"触发），同锚点，非临时非缓存非保护
	p := Build(group("/data/项目/report.pdf", "/data/项目/report-2.pdf"))[0]
	if p.Actions[0].Action != domain.OperationReview {
		t.Fatalf("expected REVIEW for same-role non-temporary duplicates, got %s", p.Actions[0].Action)
	}
	if p.Risk != domain.RiskHigh {
		t.Fatalf("expected high risk for default review, got %s", p.Risk)
	}
	if len(p.Evidence) == 0 || p.Evidence[0] != "所有副本均位于同一目录角色：project_work" {
		t.Fatalf("expected same-role evidence, got %#v", p.Evidence)
	}
}

// TestBuildQuarantineForCacheRole 验证 Cache 角色也触发 QUARANTINE。
// 此前只测了 Temporary 角色，Cache 角色未覆盖。
func TestBuildQuarantineForCacheRole(t *testing.T) {
	// "缓存" 触发 RoleCache (priority 10)
	p := Build(group("/data/缓存/a.txt", "/data/缓存/b.txt"))[0]
	if p.Actions[0].Action != domain.OperationKeep && p.Actions[1].Action != domain.OperationQuarantine {
		// 至少一个应该是 Quarantine
	}
	hasQuarantine := false
	for _, a := range p.Actions {
		if a.Action == domain.OperationQuarantine {
			hasQuarantine = true
		}
	}
	if !hasQuarantine {
		t.Fatalf("expected at least one QUARANTINE action for cache role, got %#v", p.Actions)
	}
	if p.Risk != domain.RiskMedium {
		t.Fatalf("expected medium risk for cache quarantine, got %s", p.Risk)
	}
}

func TestBuildProtectsSidecarEvenInsideCacheDirectory(t *testing.T) {
	p := Build(group("/data/缓存/audio.wav.peak", "/data/缓存/copy.wav.peak"))[0]
	if p.Risk != domain.RiskCritical {
		t.Fatalf("sidecar must be critical review, got %s", p.Risk)
	}
	for _, action := range p.Actions {
		if action.Action != domain.OperationReview {
			t.Fatalf("sidecar must never be ordinary quarantine: %#v", action)
		}
	}
}

func TestBuildProtectsProjectSource(t *testing.T) {
	p := Build(group("/data/temp/session.cpr", "/data/temp/session-copy.cpr"))[0]
	if p.Risk != domain.RiskCritical || p.Actions[0].Action != domain.OperationReview {
		t.Fatalf("project source must be protected: %#v", p)
	}
}
