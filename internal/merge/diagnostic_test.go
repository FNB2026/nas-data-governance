package merge

import (
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestDiagnoseExplainsThresholdGates(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/data/project/a.txt", Name: "a.txt"},
		{Path: "/data/project/b.txt", Name: "b.txt"},
		{Path: "/data/project_backup/a.txt", Name: "a.txt"},
		{Path: "/data/project_backup/c.txt", Name: "c.txt"},
		{Path: "/data/photo/x.jpg", Name: "x.jpg"},
		{Path: "/data/photo_copy/x.jpg", Name: "x.jpg"},
	}
	report := Diagnose(files, time.Now())
	if report.ExecutionAuthorized || report.Summary.NameSimilarPairs != 2 {
		t.Fatalf("unexpected diagnostic: %#v", report.Summary)
	}
	if report.Summary.OverlapAtLeast25 != 2 || report.Summary.OverlapAtLeast50 != 1 || report.Summary.Suggestions != 1 {
		t.Fatalf("unexpected gates: %#v", report.Summary)
	}
	if got := len(Suggest(files)); got != report.Summary.Suggestions {
		t.Fatalf("diagnostic suggestions=%d production=%d", report.Summary.Suggestions, got)
	}
}
