package assets

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"nas-data-governance/internal/domain"
)

func mustTime(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestGroupByBusinessAnchor(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/data/PRJ-2024-001/docs/spec.pdf", StorageID: "s1", ModifiedAt: mustTime("2024-03-01")},
		{Path: "/data/PRJ-2024-001/raw/photo.jpg", StorageID: "s1", ModifiedAt: mustTime("2024-03-02")},
		{Path: "/data/PRJ-2024-002/docs/spec.pdf", StorageID: "s1", ModifiedAt: mustTime("2024-03-03")},
		{Path: "/data/PRJ-2024-002/raw/draft.jpg", StorageID: "s1", ModifiedAt: mustTime("2024-03-04")},
	}
	groups := Group(files)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	for _, g := range groups {
		if len(g.Members) != 2 {
			t.Errorf("anchor %s: expected 2 members, got %d", g.Anchor, len(g.Members))
		}
		if g.ID == "" {
			t.Error("expected non-empty group ID")
		}
		if g.RootPath == "" {
			t.Error("expected non-empty root path")
		}
	}
}

func TestGroupByPathPrefixWhenNoAnchor(t *testing.T) {
	// Paths without year folders or project codes: no anchor detected, so
	// clustering falls back to the first 3 path segments.
	files := []domain.FileInstance{
		{Path: "/photos/trip/beach.jpg", StorageID: "s1", ModifiedAt: mustTime("2023-06-01")},
		{Path: "/photos/trip/sunset.jpg", StorageID: "s1", ModifiedAt: mustTime("2023-06-02")},
		{Path: "/photos/work/report.pdf", StorageID: "s1", ModifiedAt: mustTime("2023-07-10")},
	}
	groups := Group(files)
	// /photos/trip clusters 2 files; /photos/work has 1 (filtered).
	found := false
	for _, g := range groups {
		if len(g.Members) == 2 {
			found = true
			if g.Anchor != "" {
				t.Errorf("expected empty anchor for path-prefix group, got %q", g.Anchor)
			}
		}
	}
	if !found {
		t.Fatalf("expected a 2-member path-prefix group, got %d groups", len(groups))
	}
}

func TestGroupIgnoresSystemPathAboveScanRoot(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/synthetic/mount/archive/client-a/a.txt"},
		{Path: "/synthetic/mount/archive/client-a/b.txt"},
		{Path: "/synthetic/mount/archive/client-b/a.txt"},
		{Path: "/synthetic/mount/archive/client-b/b.txt"},
	}
	groups := Group(files)
	if len(groups) != 2 {
		t.Fatalf("expected groups below common scan root, got %d", len(groups))
	}
}

func TestSingleFilesAreNotGroups(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/a/PRJ-2024-001/x.pdf", StorageID: "s1"},
		{Path: "/b/PRJ-2024-002/y.pdf", StorageID: "s1"},
	}
	groups := Group(files)
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups (all singletons), got %d", len(groups))
	}
}

func TestCommonDirPrefix(t *testing.T) {
	cases := []struct {
		name  string
		paths []string
		want  string
	}{
		{"shared_parent", []string{"/data/proj/a.txt", "/data/proj/b.txt"}, "/data/proj"},
		{"partial_overlap", []string{"/data/proj/a/x.txt", "/data/proj/b/y.txt"}, "/data/proj"},
		{"no_overlap", []string{"/data/a/x.txt", "/other/b/y.txt"}, "/"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := make([]domain.FileInstance, len(c.paths))
			for i, p := range c.paths {
				files[i] = domain.FileInstance{Path: p}
			}
			got := commonDirPrefix(files)
			if got != c.want {
				t.Errorf("commonDirPrefix(%v) = %q, want %q", c.paths, got, c.want)
			}
		})
	}
}

func TestGroupDeterministicOrder(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/data/PRJ-2024-001/a.txt", StorageID: "s1", ModifiedAt: mustTime("2024-01-01")},
		{Path: "/data/PRJ-2024-001/b.txt", StorageID: "s1", ModifiedAt: mustTime("2024-01-02")},
		{Path: "/data/PRJ-2024-003/c.txt", StorageID: "s1", ModifiedAt: mustTime("2024-01-03")},
		{Path: "/data/PRJ-2024-003/d.txt", StorageID: "s1", ModifiedAt: mustTime("2024-01-04")},
	}
	g1 := Group(files)
	g2 := Group(files)
	if len(g1) != len(g2) {
		t.Fatalf("non-deterministic: group counts differ %d vs %d", len(g1), len(g2))
	}
	for i := range g1 {
		if g1[i].ID != g2[i].ID {
			t.Errorf("non-deterministic order: run1[%d].ID=%s, run2[%d].ID=%s", i, g1[i].ID, i, g2[i].ID)
		}
	}
}

func TestMembersSortedByModifiedAt(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/data/PRJ-2024-001/b.txt", StorageID: "s1", ModifiedAt: mustTime("2024-03-05")},
		{Path: "/data/PRJ-2024-001/a.txt", StorageID: "s1", ModifiedAt: mustTime("2024-03-01")},
	}
	groups := Group(files)
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	m := groups[0].Members
	if m[0].Path != "/data/PRJ-2024-001/a.txt" {
		t.Errorf("expected older file first, got %s", m[0].Path)
	}
}

func TestEmptyInput(t *testing.T) {
	groups := Group(nil)
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups for nil input, got %d", len(groups))
	}
}

func TestOversizedGroupIsSplitAndRequiresReview(t *testing.T) {
	files := make([]domain.FileInstance, maxGroupMembers+25)
	for i := range files {
		files[i] = domain.FileInstance{
			Path:      fmt.Sprintf("/data/2024/project/same-dir/file-%05d.wav", i),
			StorageID: "s1", ModifiedAt: time.Unix(int64(i), 0),
		}
	}
	groups := Group(files)
	if len(groups) < 2 {
		t.Fatalf("oversized bucket must split, got %d group(s)", len(groups))
	}
	total := 0
	for _, group := range groups {
		total += len(group.Members)
		if len(group.Members) > maxGroupMembers {
			t.Fatalf("group %s has %d members, cap=%d", group.ID, len(group.Members), maxGroupMembers)
		}
		if !group.ReviewRequired || group.ReviewReason == "" {
			t.Fatalf("split group must require review: %#v", group)
		}
	}
	if total != len(files) {
		t.Fatalf("split lost members: got %d want %d", total, len(files))
	}
}

func TestGroupEvidenceDoesNotDuplicateSensitiveMemberPaths(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/private/customer/project/a.pdf"},
		{Path: "/private/customer/project/b.pdf"},
	}
	groups := Group(files)
	if len(groups) != 1 {
		t.Fatalf("expected one group, got %d", len(groups))
	}
	for _, evidence := range groups[0].Evidence {
		if strings.Contains(evidence, "/private/customer/project") {
			t.Fatalf("evidence duplicated a sensitive member path: %q", evidence)
		}
	}
}
