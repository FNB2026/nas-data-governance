package merge

import (
	"testing"
	"time"

	"nas-data-governance/internal/domain"
)

func mt(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestSuggestBackupDir(t *testing.T) {
	// project and project_backup share all file names → high overlap.
	files := []domain.FileInstance{
		{Path: "/data/project/a.txt", ModifiedAt: mt("2024-01-01")},
		{Path: "/data/project/b.txt", ModifiedAt: mt("2024-01-02")},
		{Path: "/data/project_backup/a.txt", ModifiedAt: mt("2024-01-03")},
		{Path: "/data/project_backup/b.txt", ModifiedAt: mt("2024-01-04")},
	}
	suggestions := Suggest(files)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
	s := suggestions[0]
	// Target should be the non-backup dir.
	if s.TargetDir != "/data/project" {
		t.Errorf("target = %s, want /data/project", s.TargetDir)
	}
	if len(s.SourceDirs) != 1 || s.SourceDirs[0] != "/data/project_backup" {
		t.Errorf("source = %v, want [/data/project_backup]", s.SourceDirs)
	}
	if s.Confidence < nameOverlapThreshold {
		t.Errorf("confidence = %f, want >= %f", s.Confidence, nameOverlapThreshold)
	}
}

func TestSuggestCopyDir(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/photos/trip/x.jpg", ModifiedAt: mt("2024-01-01")},
		{Path: "/photos/trip/y.jpg", ModifiedAt: mt("2024-01-02")},
		{Path: "/photos/trip_copy/x.jpg", ModifiedAt: mt("2024-01-03")},
		{Path: "/photos/trip_copy/y.jpg", ModifiedAt: mt("2024-01-04")},
	}
	suggestions := Suggest(files)
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}
}

func TestNoSuggestionWhenLowOverlap(t *testing.T) {
	// Same parent, similar names, but completely different file contents.
	files := []domain.FileInstance{
		{Path: "/data/work/a.txt", ModifiedAt: mt("2024-01-01")},
		{Path: "/data/work_backup/b.txt", ModifiedAt: mt("2024-01-02")},
		{Path: "/data/work_backup/c.txt", ModifiedAt: mt("2024-01-03")},
	}
	suggestions := Suggest(files)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions (no file overlap), got %d", len(suggestions))
	}
}

func TestNoSuggestionForUnrelatedDirs(t *testing.T) {
	// Different base names — not merge candidates even if overlapping names.
	files := []domain.FileInstance{
		{Path: "/data/alpha/a.txt", ModifiedAt: mt("2024-01-01")},
		{Path: "/data/beta/a.txt", ModifiedAt: mt("2024-01-02")},
	}
	suggestions := Suggest(files)
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions (unrelated names), got %d", len(suggestions))
	}
}

func TestPickTargetPrefersNonBackup(t *testing.T) {
	names := map[string]map[string]struct{}{
		"/d/project":        {"a": {}},
		"/d/project_backup": {"a": {}},
	}
	target, source := pickTarget("/d/project_backup", "/d/project", names)
	if target != "/d/project" {
		t.Errorf("target = %s, want /d/project", target)
	}
	if source != "/d/project_backup" {
		t.Errorf("source = %s, want /d/project_backup", source)
	}
}

func TestPickTargetPrefersMoreFiles(t *testing.T) {
	// Neither has a backup suffix: pick the one with more files.
	names := map[string]map[string]struct{}{
		"/d/work":  {"a": {}, "b": {}},
		"/d/work2": {"a": {}},
	}
	target, _ := pickTarget("/d/work", "/d/work2", names)
	if target != "/d/work" {
		t.Errorf("target = %s, want /d/work (more files)", target)
	}
}

func TestJaccard(t *testing.T) {
	cases := []struct {
		name string
		a, b map[string]struct{}
		want float64
	}{
		{"identical", map[string]struct{}{"a": {}, "b": {}}, map[string]struct{}{"a": {}, "b": {}}, 1.0},
		{"half", map[string]struct{}{"a": {}, "b": {}}, map[string]struct{}{"a": {}, "c": {}}, 1.0 / 3.0},
		{"empty", map[string]struct{}{}, map[string]struct{}{}, 0},
		{"disjoint", map[string]struct{}{"a": {}}, map[string]struct{}{"b": {}}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := jaccard(c.a, c.b)
			if abs(got-c.want) > 1e-9 {
				t.Errorf("jaccard = %f, want %f", got, c.want)
			}
		})
	}
}

func TestNamesSimilar(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"/d/project", "/d/project_backup", true},
		{"/d/photos", "/d/photos_copy", true},
		{"/d/work (1)", "/d/work", true},
		{"/d/alpha", "/d/beta", false},
		{"/d/project", "/d/project", true},
	}
	for _, c := range cases {
		got := namesSimilar(c.a, c.b)
		if got != c.want {
			t.Errorf("namesSimilar(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestSuggestDeterministic(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/data/proj/a.txt", ModifiedAt: mt("2024-01-01")},
		{Path: "/data/proj_backup/a.txt", ModifiedAt: mt("2024-01-02")},
	}
	s1 := Suggest(files)
	s2 := Suggest(files)
	if len(s1) != len(s2) {
		t.Fatalf("non-deterministic: %d vs %d", len(s1), len(s2))
	}
	for i := range s1 {
		if s1[i].ID != s2[i].ID {
			t.Errorf("non-deterministic ID: %s vs %s", s1[i].ID, s2[i].ID)
		}
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
