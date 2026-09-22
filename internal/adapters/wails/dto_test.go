package wails

import (
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/query"
)

func TestDTOTimeMappingKeepsUnknownTimesEmpty(t *testing.T) {
	storages := mapStorages([]domain.Storage{{ID: "s1"}})
	if len(storages) != 1 || storages[0].CreatedAt != "" {
		t.Fatalf("zero storage time should map to empty string: %#v", storages)
	}

	file := mapFileItem(domain.FileInstance{Path: "/synthetic/a"})
	if file.ModifiedAt != "" {
		t.Fatalf("zero modified time should map to empty string, got %q", file.ModifiedAt)
	}
}

func TestMapGroupDetailComputesDirContextAndRetention(t *testing.T) {
	d := query.GroupDetail{
		DuplicateGroupSummary: query.DuplicateGroupSummary{
			GroupID:   "g1",
			SHA256:    strings.Repeat("a", 64),
			Size:      1,
			StorageID: "s1",
			PathCount: 2,
		},
		Files: []domain.FileInstance{
			{StorageID: "s1", Path: "/download/temp/a.iso", Name: "a.iso", Size: 1, ModifiedAt: time.Unix(0, 0)},
			{StorageID: "s1", Path: "/download/temp/b.iso", Name: "b.iso", Size: 1, ModifiedAt: time.Unix(1, 0)},
		},
	}

	resp := mapGroupDetail(d)
	if len(resp.Files) != 2 {
		t.Fatalf("expected 2 file items, got %d", len(resp.Files))
	}
	item := resp.Files[0]
	if item.DirContext == nil || item.RetainScore == nil {
		t.Fatalf("expected dir context and retain score populated: %#v", item)
	}
	if item.RetainScore.Total == 0 {
		t.Fatalf("expected non-zero retain score: %#v", item)
	}
	if !item.IsRetainSelected {
		t.Fatalf("older temp copy should be the retain candidate: %#v", item)
	}
	if item.RetainReason == "" {
		t.Fatalf("expected a retain reason: %#v", item)
	}
}
