// Package relations identifies non-identical file relationships: version
// chains and derivative pairs (same content, different encoding). It is
// read-only — it only interprets file names and (for derivatives) reads
// headers, never modifies the filesystem.
package relations

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"nas-data-governance/internal/domain"
)

// versionSuffix captures the trailing token that marks a file as a version
// of some base name: _v1, -final, (2), 副本, _draft2, etc.
var versionSuffix = regexp.MustCompile(`(?i)([_-]v?\d+$|[_-]final\d*$|[_-]draft\d*$|[_-]copy\d*$|[_-]副本$|[_-]old$|[_-]new$|[_-]backup$|\(\d+\)$)`)

// Versions scans a file list for version relationships within the same
// directory. Two files are versions when their name stems share a common
// base after stripping version markers, and they carry the same extension.
//
// Version detection is conservative (K-002): a false positive just yields a
// "review" relation; a false negative leaves files unlinked. Either way no
// automatic deletion occurs.
func Versions(files []domain.FileInstance) []domain.FileRelation {
	// Group by (dir, baseName, ext) so versions must share all three.
	type key struct{ dir, base, ext string }
	buckets := map[key][]domain.FileInstance{}

	for _, f := range files {
		dir := filepath.ToSlash(filepath.Dir(f.Path))
		name := f.Name
		if name == "" {
			name = filepath.Base(f.Path)
		}
		ext := filepath.Ext(name)
		stem := strings.TrimSuffix(name, ext)
		base := versionSuffix.ReplaceAllString(stem, "")
		base = strings.TrimRight(base, "-_ ")
		if base == "" {
			// Empty base name: cannot form a version key.
			continue
		}
		k := key{dir, strings.ToLower(base), strings.ToLower(ext)}
		buckets[k] = append(buckets[k], f)
	}

	relations := make([]domain.FileRelation, 0)
	for k, members := range buckets {
		if len(members) < 2 {
			continue
		}
		// Sort for stable pair ordering (older first).
		sort.SliceStable(members, func(i, j int) bool {
			if !members[i].ModifiedAt.Equal(members[j].ModifiedAt) {
				return members[i].ModifiedAt.Before(members[j].ModifiedAt)
			}
			return strings.Compare(members[i].Path, members[j].Path) < 0
		})
		// Link consecutive versions into a chain: a→b, b→c, ...
		for i := 0; i+1 < len(members); i++ {
			relations = append(relations, domain.FileRelation{
				Type: domain.RelationVersion,
				A:    members[i].Path,
				B:    members[i+1].Path,
				Evidence: []string{
					"同目录内基础名相同、扩展名相同、仅版本标记不同",
					"基础名：" + k.base + k.ext,
					"版本关系默认不自动删除（K-002）",
				},
			})
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
