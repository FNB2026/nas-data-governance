package dircontext

import (
	"nas-data-governance/internal/domain"
	"testing"
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
