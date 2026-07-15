package relations

import (
	"testing"

	"nas-data-governance/internal/domain"
)

func TestSidecarsLinkExactPrimaryAndRemainReviewOnly(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/project/audio.wav", Name: "audio.wav"},
		{Path: "/project/audio.wav.peak", Name: "audio.wav.peak"},
		{Path: "/project/photo.jpg", Name: "photo.jpg"},
		{Path: "/project/photo.xmp", Name: "photo.xmp"},
	}
	rels := Sidecars(files)
	if len(rels) != 2 {
		t.Fatalf("got %d relations: %#v", len(rels), rels)
	}
	for _, rel := range rels {
		if rel.Type != domain.RelationSidecar || len(rel.Evidence) < 2 {
			t.Fatalf("unexpected relation: %#v", rel)
		}
	}
}

func TestSidecarsRejectAmbiguousStem(t *testing.T) {
	files := []domain.FileInstance{
		{Path: "/project/photo.jpg", Name: "photo.jpg"},
		{Path: "/project/photo.png", Name: "photo.png"},
		{Path: "/project/photo.xmp", Name: "photo.xmp"},
	}
	if got := Sidecars(files); len(got) != 0 {
		t.Fatalf("ambiguous sidecar must remain unlinked for review: %#v", got)
	}
}
