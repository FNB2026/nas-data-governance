package dircontext

import (
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestClassifyProtectsSensitiveBeforeTemporary(t *testing.T) {
	c := Classify("/家庭/医疗/临时/报告.pdf")
	if c.Role != domain.RoleSensitive || !c.Protected {
		t.Fatalf("got %#v", c)
	}
}

func TestClassifyRecognizesBackup(t *testing.T) {
	c := Classify("/Volumes/NAS/家庭资料/冷备/2024/photo.jpg")
	if c.Role != domain.RoleBackup || !c.Protected {
		t.Fatalf("got %#v", c)
	}
}

func TestClassifyDoesNotTreatTemporarySystemPathAsDirectoryRole(t *testing.T) {
	c := Classify("/var/folders/a/tmp.123/project/file.txt")
	if c.Role != domain.RoleProjectWork {
		t.Fatalf("got %#v", c)
	}
}

func TestParentChainCapturesUpToSixAncestors(t *testing.T) {
	c := Classify("/a/b/c/d/e/f/g/file.txt")
	if len(c.ParentChain) != 6 {
		t.Fatalf("expected chain capped at 6, got %d: %#v", len(c.ParentChain), c.ParentChain)
	}
	// Nearest first: the first chain node is the immediate parent "g".
	if c.ParentChain[0].Name != "g" {
		t.Fatalf("expected nearest first, got %#v", c.ParentChain)
	}
}

func TestParentChainRolesReflectEachSegment(t *testing.T) {
	c := Classify("/家庭/医疗/报告.pdf")
	if len(c.ParentChain) != 2 {
		t.Fatalf("expected 2 ancestors, got %d", len(c.ParentChain))
	}
	if c.ParentChain[0].Role != domain.RoleSensitive || c.ParentChain[0].Authority != 100 {
		t.Fatalf("medical parent should be sensitive: %#v", c.ParentChain[0])
	}
}

func TestBranchPointFlagsCrossRoleAncestor(t *testing.T) {
	// ProjectWork file with a FormalArchive ancestor: the archive dir is
	// the branch point — same content there might be the canonical copy.
	c := Classify("/data/归档/项目/file.pdf")
	if c.BranchPoint != "/data/归档" {
		t.Fatalf("expected branch point at /data/归档, got %q", c.BranchPoint)
	}
}

func TestBranchPointEmptyWhenUniform(t *testing.T) {
	// All ancestors unknown → no internal branching detected.
	c := Classify("/a/b/c/file.txt")
	if c.BranchPoint != "" {
		t.Fatalf("expected empty branch point, got %q", c.BranchPoint)
	}
}

func TestBusinessAnchorDetectsProjectCode(t *testing.T) {
	c := Classify("/data/PRJ-2024-001/交付/file.pdf")
	if c.BusinessAnchor != "PRJ-2024-001" {
		t.Fatalf("expected project code anchor, got %q", c.BusinessAnchor)
	}
}

func TestBusinessAnchorDetectsYearFolder(t *testing.T) {
	c := Classify("/归档/2024/报告.pdf")
	if c.BusinessAnchor != "2024" {
		t.Fatalf("expected 2024 anchor, got %q", c.BusinessAnchor)
	}
}

func TestBusinessAnchorPicksNearest(t *testing.T) {
	// Two year anchors in the path; the nearest wins.
	c := Classify("/2023/归档/2024/file.pdf")
	if c.BusinessAnchor != "2024" {
		t.Fatalf("expected nearest anchor 2024, got %q", c.BusinessAnchor)
	}
}

func TestBusinessAnchorEmptyForUnstructured(t *testing.T) {
	c := Classify("/杂项/file.txt")
	if c.BusinessAnchor != "" {
		t.Fatalf("expected empty anchor, got %q", c.BusinessAnchor)
	}
}

// ---- P1-6: 补齐缺失的角色正面测试 ----

func TestClassifyRecognizesSystem(t *testing.T) {
	c := Classify("/data/.git/config")
	if c.Role != domain.RoleSystem {
		t.Fatalf("expected RoleSystem, got %s", c.Role)
	}
	if !c.Protected {
		t.Fatal("system role should be protected")
	}
	if c.AuthorityLevel != 100 {
		t.Fatalf("expected authority 100, got %d", c.AuthorityLevel)
	}
}

func TestClassifyRecognizesCache(t *testing.T) {
	c := Classify("/data/cache/thumbnail.jpg")
	if c.Role != domain.RoleCache {
		t.Fatalf("expected RoleCache, got %s", c.Role)
	}
	if c.Protected {
		t.Fatal("cache role should not be protected")
	}
}

func TestClassifyRecognizesRaw(t *testing.T) {
	c := Classify("/data/raw/相机导出/IMG_001.dng")
	if c.Role != domain.RoleRaw {
		t.Fatalf("expected RoleRaw, got %s", c.Role)
	}
	if !c.Protected {
		t.Fatal("raw role should be protected")
	}
}

func TestClassifyRecognizesFormalArchive(t *testing.T) {
	c := Classify("/data/归档/2024/最终交付/report.pdf")
	if c.Role != domain.RoleFormalArchive {
		t.Fatalf("expected RoleFormalArchive, got %s", c.Role)
	}
	// FormalArchive is NOT in the protected list (only System/Backup/Raw are)
	if c.Protected {
		t.Fatal("formal archive should not be protected (only System/Backup/Raw are)")
	}
}

func TestClassifyRecognizesUnorganized(t *testing.T) {
	c := Classify("/data/未整理/photos.zip")
	if c.Role != domain.RoleUnorganized {
		t.Fatalf("expected RoleUnorganized, got %s", c.Role)
	}
	if c.Protected {
		t.Fatal("unorganized role should not be protected")
	}
}

func TestClassifyRecognizesDatabase(t *testing.T) {
	c := Classify("/data/数据库/backup.sql")
	if c.Role != domain.RoleSystem {
		t.Fatalf("expected RoleSystem for 数据库, got %s", c.Role)
	}
	if !c.Protected {
		t.Fatal("database path should be protected as system role")
	}
}

func TestClassifyUnknownForUnrelatedPath(t *testing.T) {
	c := Classify("/a/b/c/file.txt")
	if c.Role != domain.RoleUnknown {
		t.Fatalf("expected RoleUnknown for generic path, got %s", c.Role)
	}
	if c.AuthorityLevel != 50 {
		t.Fatalf("expected authority 50 for unknown, got %d", c.AuthorityLevel)
	}
}

func TestP1MediaProductionRolesAreConservativeAndExplainable(t *testing.T) {
	tests := []struct {
		path      string
		role      domain.DirectoryRole
		protected bool
	}{
		{"/data/现场录音/take.wav", domain.RoleRaw, true},
		{"/data/视频素材/clip.mov", domain.RoleRaw, true},
		{"/data/节目后期/timeline.prproj", domain.RoleProjectWork, false},
		{"/data/播出版/final.mp4", domain.RoleFormalArchive, false},
	}
	for _, test := range tests {
		ctx := Classify(test.path)
		if ctx.Role != test.role || ctx.Protected != test.protected {
			t.Fatalf("%s: %#v", test.path, ctx)
		}
	}
	// A generic music library can be source, reference, or deliverable. Keep
	// it unknown instead of inventing storage duty from one ambiguous word.
	if ctx := Classify("/data/音乐/song.flac"); ctx.Role != domain.RoleUnknown {
		t.Fatalf("ambiguous music directory must remain unknown: %#v", ctx)
	}
}

func TestRuleVersionChangesWithP1Builtins(t *testing.T) {
	if got := RuleVersion(); got != "builtin-v2" {
		t.Fatalf("rule version=%q", got)
	}
}
