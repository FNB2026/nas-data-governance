package report

import (
	"nas-data-governance/internal/domain"
	"testing"
)

func TestDuplicateGroupsOnlyIncludesRepeatedFullHashes(t *testing.T) {
	files := []domain.FileInstance{{Path: "a", Size: 3, ContentSHA256: "x"}, {Path: "b", Size: 3, ContentSHA256: "x"}, {Path: "c", Size: 4, ContentSHA256: "y"}, {Path: "d"}}
	groups := DuplicateGroups(files)
	if len(groups) != 1 || len(groups[0].Files) != 2 {
		t.Fatalf("unexpected groups: %#v", groups)
	}
}
