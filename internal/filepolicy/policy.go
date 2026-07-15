// Package filepolicy classifies project sources and sidecars conservatively.
// A regenerable cache is still protected until its primary/project dependency
// has been verified; classification never grants deletion authority.
package filepolicy

import (
	"path/filepath"
	"strings"

	"nas-data-governance/internal/domain"
)

type Policy struct {
	Format      string
	MIME        string
	Category    domain.FormatCategory
	Role        domain.FormatRole
	Protected   bool
	Regenerable bool
	Sidecar     bool
}

var extensionPolicies = map[string]Policy{
	".xmp":      {Format: "xmp", MIME: "application/rdf+xml", Category: domain.CategoryOther, Role: domain.FormatRoleMetadataSidecar, Protected: true, Sidecar: true},
	".cpr":      {Format: "cubase-project", MIME: "application/x-cubase-project", Category: domain.CategoryOther, Role: domain.FormatRoleProjectSource, Protected: true},
	".sesx":     {Format: "audition-session", MIME: "application/xml", Category: domain.CategoryOther, Role: domain.FormatRoleProjectSource, Protected: true},
	".psd":      {Format: "psd", MIME: "image/vnd.adobe.photoshop", Category: domain.CategoryImage, Role: domain.FormatRoleProjectSource, Protected: true},
	".peak":     {Format: "peak-cache", Category: domain.CategoryOther, Role: domain.FormatRoleRegenerableCache, Protected: true, Regenerable: true, Sidecar: true},
	".pkf":      {Format: "pkf-cache", Category: domain.CategoryOther, Role: domain.FormatRoleRegenerableCache, Protected: true, Regenerable: true, Sidecar: true},
	".pek":      {Format: "pek-cache", Category: domain.CategoryOther, Role: domain.FormatRoleRegenerableCache, Protected: true, Regenerable: true, Sidecar: true},
	".cfa":      {Format: "cfa-cache", Category: domain.CategoryOther, Role: domain.FormatRoleRegenerableCache, Protected: true, Regenerable: true, Sidecar: true},
	".mpgindex": {Format: "mpeg-index-cache", Category: domain.CategoryOther, Role: domain.FormatRoleRegenerableCache, Protected: true, Regenerable: true, Sidecar: true},
}

func ForPath(path string) (Policy, bool) {
	policy, ok := extensionPolicies[strings.ToLower(filepath.Ext(path))]
	return policy, ok
}

func Apply(path string, info domain.FormatInfo) domain.FormatInfo {
	policy, ok := ForPath(path)
	if ok {
		if info.Format == "" || info.Format == "unknown" || policy.Role == domain.FormatRoleProjectSource {
			info.Format = policy.Format
			info.Category = policy.Category
			if policy.MIME != "" {
				info.MIME = policy.MIME
			}
		}
		info.Role = policy.Role
		info.Protected = policy.Protected
		info.Regenerable = policy.Regenerable
		return info
	}
	if info.Format != "" && info.Format != "unknown" && info.Role == "" {
		info.Role = domain.FormatRolePrimary
	}
	return refineOLE(path, info)
}

func refineOLE(path string, info domain.FormatInfo) domain.FormatInfo {
	if info.Format != "ole" {
		return info
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".doc":
		info.Format, info.MIME = "doc", "application/msword"
	case ".xls":
		info.Format, info.MIME = "xls", "application/vnd.ms-excel"
	case ".ppt":
		info.Format, info.MIME = "ppt", "application/vnd.ms-powerpoint"
	}
	return info
}

func IsDependencyProtected(path string) bool {
	policy, ok := ForPath(path)
	return ok && policy.Protected
}

func IsSidecar(path string) bool {
	policy, ok := ForPath(path)
	return ok && policy.Sidecar
}

// ExtensionOnly reports policies whose role is defined by the project file
// extension itself. PSD is excluded because its stable magic bytes should be
// verified before treating it as an image project source.
func ExtensionOnly(path string) bool {
	policy, ok := ForPath(path)
	return ok && policy.Format != "psd"
}
