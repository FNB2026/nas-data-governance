package relations

import (
	"testing"
	"time"

	"nas-data-governance/internal/domain"
)

func dt(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestDerivativesImagePair(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/d/photo.jpg", ModifiedAt: dt("2024-01-01")},
		{Path: "/d/photo.png", ModifiedAt: dt("2024-01-02")},
	}
	rels := Derivatives(files)
	if len(rels) != 1 {
		t.Fatalf("expected 1 derivative relation, got %d", len(rels))
	}
	if rels[0].Type != domain.RelationDerivative {
		t.Errorf("type = %s, want derivative", rels[0].Type)
	}
}

func TestDerivativesVideoPair(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/d/clip.mp4", ModifiedAt: dt("2024-01-01")},
		{Path: "/d/clip.mov", ModifiedAt: dt("2024-01-02")},
	}
	rels := Derivatives(files)
	if len(rels) != 1 {
		t.Fatalf("expected 1 derivative relation, got %d", len(rels))
	}
}

func TestDerivativesDocumentPair(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/d/report.docx", ModifiedAt: dt("2024-01-01")},
		{Path: "/d/report.pdf", ModifiedAt: dt("2024-01-02")},
	}
	rels := Derivatives(files)
	if len(rels) != 1 {
		t.Fatalf("expected 1 derivative relation, got %d", len(rels))
	}
}

func TestDerivativesIgnoresSameExtension(t *testing.T) {
	// Same extension = version case, not derivative.
	files := []domain.FileInstance{
		{Path: "/d/photo.jpg", ModifiedAt: dt("2024-01-01")},
		{Path: "/d/photo_v2.jpg", ModifiedAt: dt("2024-01-02")},
	}
	rels := Derivatives(files)
	if len(rels) != 0 {
		t.Fatalf("expected 0 derivative relations (same ext), got %d", len(rels))
	}
}

func TestDerivativesIgnoresDifferentCategory(t *testing.T) {
	// photo.jpg (image) vs photo.zip (archive) — different category, not derivative.
	files := []domain.FileInstance{
		{Path: "/d/photo.jpg", ModifiedAt: dt("2024-01-01")},
		{Path: "/d/photo.zip", ModifiedAt: dt("2024-01-02")},
	}
	rels := Derivatives(files)
	if len(rels) != 0 {
		t.Fatalf("expected 0 relations (different category), got %d", len(rels))
	}
}

func TestDerivativesIgnoresDifferentDirectory(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/a/photo.jpg", ModifiedAt: dt("2024-01-01")},
		{Path: "/b/photo.png", ModifiedAt: dt("2024-01-02")},
	}
	rels := Derivatives(files)
	if len(rels) != 0 {
		t.Fatalf("expected 0 relations (different dir), got %d", len(rels))
	}
}

func TestDerivativesUsesFormatInfoCategory(t *testing.T) {
	// When FormatInfo is present, it overrides the extension map.
	files := []domain.FileInstance{
		{Path: "/d/data.bin", Format: domain.FormatInfo{Category: domain.CategoryImage}},
		{Path: "/d/data.png", Format: domain.FormatInfo{Category: domain.CategoryImage}},
	}
	rels := Derivatives(files)
	if len(rels) != 1 {
		t.Fatalf("expected 1 relation via FormatInfo category, got %d", len(rels))
	}
}

func TestDerivativesThreeWayPairwise(t *testing.T) {
	// Three derivatives of the same stem: all pairs linked.
	files := []domain.FileInstance{
		{Path: "/d/img.jpg", ModifiedAt: dt("2024-01-01")},
		{Path: "/d/img.png", ModifiedAt: dt("2024-01-02")},
		{Path: "/d/img.tiff", ModifiedAt: dt("2024-01-03")},
	}
	rels := Derivatives(files)
	// 3 choose 2 = 3 pairs
	if len(rels) != 3 {
		t.Fatalf("expected 3 pairwise relations, got %d", len(rels))
	}
}

func TestRelationsCombinesVersionAndDerivative(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/d/photo_v1.jpg", ModifiedAt: dt("2024-01-01")},
		{Path: "/d/photo_v2.jpg", ModifiedAt: dt("2024-01-02")},
		{Path: "/d/report.pdf", ModifiedAt: dt("2024-01-03")},
		{Path: "/d/report.docx", ModifiedAt: dt("2024-01-04")},
	}
	rels := Relations(files)
	// 1 version (photo_v1->photo_v2) + 1 derivative (report.pdf vs report.docx) = 2
	if len(rels) != 2 {
		t.Fatalf("expected 2 relations combined, got %d", len(rels))
	}
}

// ---- P1-6: 补齐缺失分支测试 ----

func TestDerivativesAudioPair(t *testing.T) {
	// 音频派生关系：mp3 vs wav
	files := []domain.FileInstance{
		{Path: "/d/song.mp3", ModifiedAt: dt("2024-01-01")},
		{Path: "/d/song.wav", ModifiedAt: dt("2024-01-02")},
	}
	rels := Derivatives(files)
	if len(rels) != 1 {
		t.Fatalf("expected 1 audio derivative relation, got %d", len(rels))
	}
	if rels[0].Type != domain.RelationDerivative {
		t.Errorf("type = %s, want derivative", rels[0].Type)
	}
}

func TestDerivativesAudioFlacPair(t *testing.T) {
	// flac vs aac
	files := []domain.FileInstance{
		{Path: "/d/track.flac", ModifiedAt: dt("2024-01-01")},
		{Path: "/d/track.aac", ModifiedAt: dt("2024-01-02")},
	}
	rels := Derivatives(files)
	if len(rels) != 1 {
		t.Fatalf("expected 1 flac/aac derivative relation, got %d", len(rels))
	}
}
