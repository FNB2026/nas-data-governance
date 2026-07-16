package governancediag

import (
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestBuildKeepsAllPlansDraftAndZeroFilesReviewOnly(t *testing.T) {
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	hash := strings.Repeat("a", 64)
	files := []domain.FileInstance{
		{StorageID: "s", Path: "/data/cache/a.wav", Name: "a.wav", Size: 100, ContentSHA256: hash, ModifiedAt: now, Format: domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio, Duration: 1, Codec: "pcm"}},
		{StorageID: "s", Path: "/data/cache/b.wav", Name: "b.wav", Size: 100, ContentSHA256: hash, ModifiedAt: now, Format: domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio, Duration: 1, Codec: "pcm"}},
		{StorageID: "s", Path: "/data/temp/output.tmp", Name: "output.tmp", Size: 0, ModifiedAt: now},
	}
	report := Build(files, 50, now)
	if report.Summary.FormatRows != 2 || report.Summary.MissingFormatRows != 1 {
		t.Fatalf("format coverage: %#v", report.Summary)
	}
	if report.ExecutionAuthorized || report.Summary.NonDraftPlans != 0 || report.Summary.DraftPlans != 1 {
		t.Fatalf("unsafe report state: %#v", report.Summary)
	}
	if report.Summary.QuarantineCandidateActions != 1 {
		t.Fatalf("expected one low-duty candidate, got %#v", report.Summary)
	}
	if len(report.ZeroByteReviews) != 1 || report.ZeroByteReviews[0].Recommendation != RecommendationKeepReview {
		t.Fatalf("zero byte review: %#v", report.ZeroByteReviews)
	}
	if report.ZeroByteReviews[0].Classification != "incomplete_or_failed_output" {
		t.Fatalf("classification: %#v", report.ZeroByteReviews[0])
	}
}

func TestBuildProtectsSidecarDuplicateAndCapturesMediaRelations(t *testing.T) {
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	hash := strings.Repeat("b", 64)
	files := []domain.FileInstance{
		{Path: "/project/2024/audio.wav", Name: "audio.wav", Size: 200, ModifiedAt: now, Format: domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio, Duration: 2}},
		{Path: "/project/2024/audio.wav.peak", Name: "audio.wav.peak", Size: 10, ContentSHA256: hash, ModifiedAt: now},
		{Path: "/project/2024/copy.peak", Name: "copy.peak", Size: 10, ContentSHA256: hash, ModifiedAt: now},
	}
	report := Build(files, 100, now)
	if report.Summary.CriticalPlans != 1 || report.Summary.QuarantineCandidateActions != 0 {
		t.Fatalf("sidecar duplicate was not protected: %#v", report.Summary)
	}
	if report.Summary.LargeMediaFiles != 1 || report.Summary.MediaRelations != 1 {
		t.Fatalf("media analysis missing: %#v", report.Summary)
	}
	if report.Summary.LargeMediaWithRelations != 1 || report.Summary.LargeMediaWithAnchor != 1 {
		t.Fatalf("media relation/anchor summary missing: %#v", report.Summary)
	}
	if report.Summary.LargeMediaProjectWork != 1 {
		t.Fatalf("project-work summary missing: %#v", report.Summary)
	}
}
