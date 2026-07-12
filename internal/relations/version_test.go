package relations

import (
	"testing"
	"time"

	"nas-data-governance/internal/domain"
)

func vt(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestVersionsDetectsVersionMarkers(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/proj/report.pdf", ModifiedAt: vt("2024-01-01")},
		{Path: "/proj/report_v1.pdf", ModifiedAt: vt("2024-01-02")},
		{Path: "/proj/report_v2.pdf", ModifiedAt: vt("2024-01-03")},
		{Path: "/proj/notes.txt", ModifiedAt: vt("2024-01-04")},
	}
	rels := Versions(files)
	if len(rels) != 2 {
		t.Fatalf("expected 2 version relations (chain), got %d", len(rels))
	}
	// Chain: report.pdf -> report_v1.pdf -> report_v2.pdf
	wantA, wantB := "/proj/report.pdf", "/proj/report_v1.pdf"
	if rels[0].A != wantA || rels[0].B != wantB {
		t.Errorf("first link = %s -> %s, want %s -> %s", rels[0].A, rels[0].B, wantA, wantB)
	}
	if rels[0].Type != domain.RelationVersion {
		t.Errorf("type = %s, want version", rels[0].Type)
	}
}

func TestVersionsDetectsFinalDraftMarkers(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/p/spec_draft.pdf", ModifiedAt: vt("2024-01-01")},
		{Path: "/p/spec_final.pdf", ModifiedAt: vt("2024-01-02")},
	}
	rels := Versions(files)
	if len(rels) != 1 {
		t.Fatalf("expected 1 version relation, got %d", len(rels))
	}
}

func TestVersionsDetectsParenNumber(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/d/photo.jpg", ModifiedAt: vt("2024-01-01")},
		{Path: "/d/photo (1).jpg", ModifiedAt: vt("2024-01-02")},
		{Path: "/d/photo (2).jpg", ModifiedAt: vt("2024-01-03")},
	}
	rels := Versions(files)
	if len(rels) != 2 {
		t.Fatalf("expected 2 version relations, got %d", len(rels))
	}
}

func TestVersionsIgnoresDifferentExtension(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/d/report_v1.pdf", ModifiedAt: vt("2024-01-01")},
		{Path: "/d/report_v2.docx", ModifiedAt: vt("2024-01-02")},
	}
	rels := Versions(files)
	if len(rels) != 0 {
		t.Fatalf("expected 0 relations (different ext), got %d", len(rels))
	}
}

func TestVersionsIgnoresDifferentDirectory(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/a/report_v1.pdf", ModifiedAt: vt("2024-01-01")},
		{Path: "/b/report_v2.pdf", ModifiedAt: vt("2024-01-02")},
	}
	rels := Versions(files)
	if len(rels) != 0 {
		t.Fatalf("expected 0 relations (different dir), got %d", len(rels))
	}
}

func TestVersionsIgnoresNoMarker(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/d/alpha.pdf", ModifiedAt: vt("2024-01-01")},
		{Path: "/d/beta.pdf", ModifiedAt: vt("2024-01-02")},
	}
	rels := Versions(files)
	if len(rels) != 0 {
		t.Fatalf("expected 0 relations (no version markers), got %d", len(rels))
	}
}

func TestVersionsDetectsCopyMarker(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/d/data.xlsx", ModifiedAt: vt("2024-01-01")},
		{Path: "/d/data_copy.xlsx", ModifiedAt: vt("2024-01-02")},
	}
	rels := Versions(files)
	if len(rels) != 1 {
		t.Fatalf("expected 1 version relation, got %d", len(rels))
	}
}

// ---- P1-6: 补齐缺失版本标记测试 ----

func TestVersionsDetectsOldNewMarkers(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/d/report.pdf", ModifiedAt: vt("2024-01-01")},
		{Path: "/d/report_old.pdf", ModifiedAt: vt("2024-01-02")},
	}
	rels := Versions(files)
	if len(rels) != 1 {
		t.Fatalf("expected 1 version relation (_old), got %d", len(rels))
	}
}

func TestVersionsDetectsNewMarker(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/d/config.yaml", ModifiedAt: vt("2024-01-01")},
		{Path: "/d/config_new.yaml", ModifiedAt: vt("2024-01-02")},
	}
	rels := Versions(files)
	if len(rels) != 1 {
		t.Fatalf("expected 1 version relation (_new), got %d", len(rels))
	}
}

func TestVersionsDetectsBackupMarker(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/d/notes.txt", ModifiedAt: vt("2024-01-01")},
		{Path: "/d/notes_backup.txt", ModifiedAt: vt("2024-01-02")},
	}
	rels := Versions(files)
	if len(rels) != 1 {
		t.Fatalf("expected 1 version relation (_backup), got %d", len(rels))
	}
}

func TestVersionsDetectsChineseCopyMarker(t *testing.T) {
	// 中文"_副本"标记（正则要求 _ 或 - 前缀）
	files := []domain.FileInstance{
		{Path: "/d/文档.pdf", ModifiedAt: vt("2024-01-01")},
		{Path: "/d/文档_副本.pdf", ModifiedAt: vt("2024-01-02")},
	}
	rels := Versions(files)
	if len(rels) != 1 {
		t.Fatalf("expected 1 version relation (_副本), got %d", len(rels))
	}
}
