// Package filepolicy classifies project sources and sidecars conservatively.
// A regenerable cache is still protected until its primary/project dependency
// has been verified; classification never grants deletion authority.
package filepolicy

import (
	"path/filepath"
	"strings"

	"github.com/FNB2026/nas-data-governance/internal/domain"
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
	".xmp":          {Format: "xmp", MIME: "application/rdf+xml", Category: domain.CategoryOther, Role: domain.FormatRoleMetadataSidecar, Protected: true, Sidecar: true},
	".cpr":          {Format: "cubase-project", MIME: "application/x-cubase-project", Category: domain.CategoryOther, Role: domain.FormatRoleProjectSource, Protected: true},
	".sesx":         {Format: "audition-session", MIME: "application/xml", Category: domain.CategoryOther, Role: domain.FormatRoleProjectSource, Protected: true},
	".psd":          {Format: "psd", MIME: "image/vnd.adobe.photoshop", Category: domain.CategoryImage, Role: domain.FormatRoleProjectSource, Protected: true},
	".peak":         {Format: "peak-cache", Category: domain.CategoryOther, Role: domain.FormatRoleRegenerableCache, Protected: true, Regenerable: true, Sidecar: true},
	".pkf":          {Format: "pkf-cache", Category: domain.CategoryOther, Role: domain.FormatRoleRegenerableCache, Protected: true, Regenerable: true, Sidecar: true},
	".pek":          {Format: "pek-cache", Category: domain.CategoryOther, Role: domain.FormatRoleRegenerableCache, Protected: true, Regenerable: true, Sidecar: true},
	".cfa":          {Format: "cfa-cache", Category: domain.CategoryOther, Role: domain.FormatRoleRegenerableCache, Protected: true, Regenerable: true, Sidecar: true},
	".mpgindex":     {Format: "mpeg-index-cache", Category: domain.CategoryOther, Role: domain.FormatRoleRegenerableCache, Protected: true, Regenerable: true, Sidecar: true},
	".aep":          {Format: "after-effects-project", MIME: "application/x-after-effects-project", Category: domain.CategoryOther, Role: domain.FormatRoleProjectSource, Protected: true},
	".prproj":       {Format: "premiere-project", MIME: "application/xml", Category: domain.CategoryOther, Role: domain.FormatRoleProjectSource, Protected: true},
	".lrcat":        {Format: "lightroom-catalog", MIME: "application/vnd.sqlite3", Category: domain.CategoryOther, Role: domain.FormatRoleProjectSource, Protected: true},
	".step":         {Format: "step", MIME: "model/step", Category: domain.CategoryOther, Role: domain.FormatRoleProjectSource, Protected: true},
	".stp":          {Format: "step", MIME: "model/step", Category: domain.CategoryOther, Role: domain.FormatRoleProjectSource, Protected: true},
	".cdr":          {Format: "corel-draw", MIME: "application/vnd.corel-draw", Category: domain.CategoryImage, Role: domain.FormatRoleProjectSource, Protected: true},
	".cube":         {Format: "lut-cube", MIME: "application/x-cube-lut", Category: domain.CategoryOther, Role: domain.FormatRoleProjectSource, Protected: true},
	".srt":          {Format: "subrip", MIME: "application/x-subrip", Category: domain.CategoryDocument, Role: domain.FormatRoleMetadataSidecar, Protected: true, Sidecar: true},
	".plist":        {Format: "plist", MIME: "application/x-apple-plist", Category: domain.CategoryCode, Role: domain.FormatRoleMetadataSidecar, Protected: true, Sidecar: true},
	".lrprev":       {Format: "lightroom-preview", Category: domain.CategoryOther, Role: domain.FormatRoleRegenerableCache, Protected: true, Regenerable: true, Sidecar: true},
	".mcaudioindex": {Format: "mc-audio-index", Category: domain.CategoryOther, Role: domain.FormatRoleRegenerableCache, Protected: true, Regenerable: true, Sidecar: true},
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
	return refineContainer(path, info)
}

func refineContainer(path string, info domain.FormatInfo) domain.FormatInfo {
	ext := strings.ToLower(filepath.Ext(path))
	switch info.Format {
	case "ole":
		switch ext {
		case ".doc":
			info.Format, info.MIME = "doc", "application/msword"
		case ".xls":
			info.Format, info.MIME = "xls", "application/vnd.ms-excel"
		case ".ppt":
			info.Format, info.MIME = "ppt", "application/vnd.ms-powerpoint"
		}
	case "asf":
		switch ext {
		case ".wma":
			info.Format, info.Category, info.MIME = "wma", domain.CategoryAudio, "audio/x-ms-wma"
		case ".wmv":
			info.Format, info.Category, info.MIME = "wmv", domain.CategoryVideo, "video/x-ms-wmv"
		}
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
