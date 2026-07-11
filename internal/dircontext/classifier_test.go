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
