package relations

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/filepolicy"
)

// Sidecars links conservatively identifiable sidecars to a primary file in
// the same directory. Ambiguous stem matches are deliberately left unlinked;
// the sidecar remains protected by filepolicy and must be manually reviewed.
func Sidecars(files []domain.FileInstance) []domain.FileRelation {
	type directoryFiles struct {
		byName map[string]domain.FileInstance
		byStem map[string][]domain.FileInstance
	}
	dirs := map[string]*directoryFiles{}
	for _, file := range files {
		if filepolicy.IsSidecar(file.Path) {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(file.Path))
		bucket := dirs[dir]
		if bucket == nil {
			bucket = &directoryFiles{byName: map[string]domain.FileInstance{}, byStem: map[string][]domain.FileInstance{}}
			dirs[dir] = bucket
		}
		name := file.Name
		if name == "" {
			name = filepath.Base(file.Path)
		}
		lowerName := strings.ToLower(name)
		stem := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
		bucket.byName[lowerName] = file
		bucket.byStem[stem] = append(bucket.byStem[stem], file)
	}

	var out []domain.FileRelation
	for _, sidecar := range files {
		if !filepolicy.IsSidecar(sidecar.Path) {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(sidecar.Path))
		bucket := dirs[dir]
		if bucket == nil {
			continue
		}
		name := sidecar.Name
		if name == "" {
			name = filepath.Base(sidecar.Path)
		}
		base := strings.TrimSuffix(name, filepath.Ext(name))
		primary, ok := bucket.byName[strings.ToLower(base)]
		if !ok {
			candidates := bucket.byStem[strings.ToLower(base)]
			if len(candidates) != 1 {
				continue
			}
			primary = candidates[0]
		}
		out = append(out, domain.FileRelation{
			Type: domain.RelationSidecar, A: primary.Path, B: sidecar.Path,
			Evidence: []string{
				"同目录且基础名唯一匹配",
				"侧车受保护；即使标记可再生，也必须验证主资产和项目后人工复核",
			},
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].A != out[j].A {
			return out[i].A < out[j].A
		}
		return out[i].B < out[j].B
	})
	return out
}
