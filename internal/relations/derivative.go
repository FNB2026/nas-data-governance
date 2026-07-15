package relations

import (
	"path/filepath"
	"sort"
	"strings"

	"nas-data-governance/internal/domain"
)

// extCategory maps a file extension to a format category. Used when FormatInfo
// is absent (e.g., analyze hasn't run yet). Conservative: only the common
// derivative-prone extensions are mapped; everything else is "other".
var extCategory = map[string]domain.FormatCategory{
	".jpg":  domain.CategoryImage,
	".jpeg": domain.CategoryImage,
	".png":  domain.CategoryImage,
	".gif":  domain.CategoryImage,
	".bmp":  domain.CategoryImage,
	".webp": domain.CategoryImage,
	".tiff": domain.CategoryImage,
	".tif":  domain.CategoryImage,
	".heic": domain.CategoryImage,
	".mp4":  domain.CategoryVideo,
	".mov":  domain.CategoryVideo,
	".mkv":  domain.CategoryVideo,
	".avi":  domain.CategoryVideo,
	".flv":  domain.CategoryVideo,
	".m4v":  domain.CategoryVideo,
	".mp3":  domain.CategoryAudio,
	".wav":  domain.CategoryAudio,
	".flac": domain.CategoryAudio,
	".aac":  domain.CategoryAudio,
	".ogg":  domain.CategoryAudio,
	".m4a":  domain.CategoryAudio,
	".pdf":  domain.CategoryDocument,
	".doc":  domain.CategoryDocument,
	".docx": domain.CategoryDocument,
	".xls":  domain.CategoryDocument,
	".xlsx": domain.CategoryDocument,
	".ppt":  domain.CategoryDocument,
	".pptx": domain.CategoryDocument,
	".txt":  domain.CategoryDocument,
	".md":   domain.CategoryDocument,
}

// categoryOf resolves a file's format category: prefer the analyzed
// FormatInfo, fall back to the extension map, then "other".
func categoryOf(f domain.FileInstance) domain.FormatCategory {
	if f.Format.Category != "" && f.Format.Category != domain.CategoryUnknown {
		return f.Format.Category
	}
	if cat, ok := extCategory[strings.ToLower(filepath.Ext(f.Name))]; ok {
		return cat
	}
	if cat, ok := extCategory[strings.ToLower(filepath.Ext(f.Path))]; ok {
		return cat
	}
	return domain.CategoryOther
}

// Derivatives scans for candidate original/derivative pairs: files in the
// same directory whose name stems match but whose extensions differ, and
// which belong to the same format category (e.g., photo.jpg vs photo.png).
//
// This is header/name-only analysis — no pixels are decoded (K-006). A pair
// here is a *candidate*; confirming same-content-different-encoding would
// require perceptual hashing, which is intentionally out of scope for the
// default scan. Candidates default to review (K-002).
func Derivatives(files []domain.FileInstance) []domain.FileRelation {
	type key struct {
		dir, stem string
		category  domain.FormatCategory
	}
	buckets := map[key][]domain.FileInstance{}

	for _, f := range files {
		dir := filepath.ToSlash(filepath.Dir(f.Path))
		name := f.Name
		if name == "" {
			name = filepath.Base(f.Path)
		}
		ext := filepath.Ext(name)
		stem := strings.TrimSuffix(name, ext)
		// Strip version markers so "report_v1.pdf" shares a stem with
		// "report.docx". Same-extension collisions are filtered below.
		stem = versionSuffix.ReplaceAllString(stem, "")
		stem = strings.TrimRight(stem, "-_ ")
		if stem == "" {
			continue
		}
		cat := categoryOf(f)
		// Only image/video/document categories are derivative-prone.
		// Archives, code, etc. with same stem+different ext are usually
		// unrelated (e.g., report.zip vs report.tar.gz).
		if cat != domain.CategoryImage && cat != domain.CategoryVideo &&
			cat != domain.CategoryAudio && cat != domain.CategoryDocument {
			continue
		}
		k := key{dir, strings.ToLower(stem), cat}
		buckets[k] = append(buckets[k], f)
	}

	relations := make([]domain.FileRelation, 0)
	for k, members := range buckets {
		if len(members) < 2 {
			continue
		}
		// Verify the members actually have different extensions — a stem
		// collision with the same extension is a version case, not derivative.
		exts := map[string]bool{}
		for _, m := range members {
			exts[strings.ToLower(filepath.Ext(m.Name))] = true
			if m.Name == "" {
				exts[strings.ToLower(filepath.Ext(m.Path))] = true
			}
		}
		if len(exts) < 2 {
			continue
		}
		sort.SliceStable(members, func(i, j int) bool {
			if !members[i].ModifiedAt.Equal(members[j].ModifiedAt) {
				return members[i].ModifiedAt.Before(members[j].ModifiedAt)
			}
			return strings.Compare(members[i].Path, members[j].Path) < 0
		})
		// Link each pair where extensions differ.
		for i := 0; i < len(members); i++ {
			for j := i + 1; j < len(members); j++ {
				ei := strings.ToLower(filepath.Ext(members[i].Name))
				if members[i].Name == "" {
					ei = strings.ToLower(filepath.Ext(members[i].Path))
				}
				ej := strings.ToLower(filepath.Ext(members[j].Name))
				if members[j].Name == "" {
					ej = strings.ToLower(filepath.Ext(members[j].Path))
				}
				if ei == ej {
					continue
				}
				relations = append(relations, domain.FileRelation{
					Type: domain.RelationDerivative,
					A:    members[i].Path,
					B:    members[j].Path,
					Evidence: []string{
						"同目录、同基础名、同格式类别、不同扩展名",
						"类别：" + string(k.category),
						"候选派生关系：未经感知哈希确认，默认复核（K-002/K-006）",
					},
				})
			}
		}
	}
	sort.SliceStable(relations, func(i, j int) bool {
		if relations[i].A != relations[j].A {
			return relations[i].A < relations[j].A
		}
		return relations[i].B < relations[j].B
	})
	return relations
}

// Relations combines all relationship detectors (version + derivative).
// Identical (byte-level duplicate) relations are not produced here — those
// come from report.DuplicateGroups via content hash.
func Relations(files []domain.FileInstance) []domain.FileRelation {
	out := append(Versions(files), Derivatives(files)...)
	out = append(out, Sidecars(files)...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].A != out[j].A {
			return out[i].A < out[j].A
		}
		return out[i].B < out[j].B
	})
	return out
}
