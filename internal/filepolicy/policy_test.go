package filepolicy

import (
	"testing"

	"nas-data-governance/internal/domain"
)

func TestApplyClassifiesProtectedFileRoles(t *testing.T) {
	tests := []struct {
		path        string
		role        domain.FormatRole
		regenerable bool
	}{
		{"photo.xmp", domain.FormatRoleMetadataSidecar, false},
		{"session.cpr", domain.FormatRoleProjectSource, false},
		{"session.sesx", domain.FormatRoleProjectSource, false},
		{"audio.wav.peak", domain.FormatRoleRegenerableCache, true},
		{"audio.pkf", domain.FormatRoleRegenerableCache, true},
	}
	for _, test := range tests {
		info := Apply(test.path, domain.FormatInfo{Format: "unknown", Category: domain.CategoryUnknown})
		if info.Role != test.role || !info.Protected || info.Regenerable != test.regenerable || info.Format == "unknown" {
			t.Fatalf("%s: %#v", test.path, info)
		}
	}
}

func TestApplyRefinesOLEByExtension(t *testing.T) {
	for path, want := range map[string]string{"old.doc": "doc", "old.xls": "xls", "old.ppt": "ppt"} {
		got := Apply(path, domain.FormatInfo{Format: "ole", Category: domain.CategoryDocument})
		if got.Format != want || got.Role != domain.FormatRolePrimary {
			t.Fatalf("%s: %#v", path, got)
		}
	}
}
