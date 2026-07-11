package planner

import (
	"nas-data-governance/internal/domain"
	"testing"
	"time"
)

func group(paths ...string) []domain.DuplicateGroup {
	files := make([]domain.FileInstance, len(paths))
	for i, path := range paths {
		files[i] = domain.FileInstance{Path: path, Size: 1, ModifiedAt: time.Unix(int64(i), 0)}
	}
	return []domain.DuplicateGroup{{SHA256: "123456789012abcdef", Size: 1, Files: files}}
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
