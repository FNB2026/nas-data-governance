package formatdiag

import (
	"testing"
	"time"

	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/store"
)

func TestBuildProducesPrivateReviewEvidenceWithoutReadingSources(t *testing.T) {
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	files := []domain.FileInstance{
		{StorageID: "s", Path: "/offline/large.bin", Size: 200 << 20, ModifiedAt: now},
		{StorageID: "s", Path: "/offline/audio.wav", Size: 10},
		{StorageID: "s", Path: "/offline/video.mp4", Size: 20},
	}
	formats := []store.FormatRecord{
		{StorageID: "s", Path: "/offline/large.bin", Info: domain.FormatInfo{Format: "unknown", Category: domain.CategoryUnknown}},
		{StorageID: "s", Path: "/offline/audio.wav", Info: domain.FormatInfo{Format: "aiff", Category: domain.CategoryAudio, Duration: 2}},
		{StorageID: "s", Path: "/offline/video.mp4", Info: domain.FormatInfo{Format: "mp4", Category: domain.CategoryVideo}},
	}
	report := Build(files, formats, 100<<20, now)
	if report.Summary.LargeUnknown != 1 || len(report.LargeUnknown) != 1 {
		t.Fatalf("large unknown: %#v", report)
	}
	if report.Summary.ExtensionMismatches != 1 || report.ExtensionMismatches[0].Detected != "aiff" {
		t.Fatalf("mismatch: %#v", report.ExtensionMismatches)
	}
	if len(report.MetadataGaps) != 1 || report.MetadataGaps[0].MissingDimensions != 1 || report.MetadataGaps[0].MissingDuration != 1 {
		t.Fatalf("metadata gaps: %#v", report.MetadataGaps)
	}
}
