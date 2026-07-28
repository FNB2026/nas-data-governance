package wails

import (
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
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
