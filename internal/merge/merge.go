// Package merge proposes directory consolidation suggestions. It is
// read-only: it only reads the file index and emits suggestions, never
// touches the filesystem (K-008). Every suggestion requires human review
// before any plan is created.
package merge

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"nas-data-governance/internal/domain"
)

// nameOverlapThreshold is the minimum Jaccard overlap of file-name sets
// for two sibling directories to be considered merge candidates. 0.5 means
// half the file names must be shared — conservative to avoid merging
// unrelated directories that merely share a parent.
const nameOverlapThreshold = 0.5

// dirSuffix marks a directory as a backup/copy variant of another: project
// vs project_backup, photos vs photos_copy, etc.
var dirSuffix = regexp.MustCompile(`(?i)([_-](backup|bak|copy|old|new|副本|temp|tmp)|\(\d+\)$)`)

// Suggest scans for sibling directories that look like duplicates of each
// other (same base name after stripping backup/copy suffixes, plus
// significant file-name overlap). It returns merge suggestions only — no
// filesystem action is taken.
func Suggest(files []domain.FileInstance) []domain.MergeSuggestion {
	// Group file names by directory.
	dirNames := map[string]map[string]struct{}{}
	for _, f := range files {
		dir := filepath.ToSlash(filepath.Dir(f.Path))
		name := f.Name
		if name == "" {
			name = filepath.Base(f.Path)
		}
		if dirNames[dir] == nil {
			dirNames[dir] = map[string]struct{}{}
		}
		dirNames[dir][strings.ToLower(name)] = struct{}{}
	}

	// Group sibling directories by parent.
	siblings := map[string][]string{}
	for dir := range dirNames {
		parent := filepath.ToSlash(filepath.Dir(dir))
		if parent == dir {
			continue // root
		}
		siblings[parent] = append(siblings[parent], dir)
	}

	suggestions := make([]domain.MergeSuggestion, 0)
	for _, dirs := range siblings {
		sort.Strings(dirs)
		for i := 0; i < len(dirs); i++ {
			for j := i + 1; j < len(dirs); j++ {
				a, b := dirs[i], dirs[j]
				if !namesSimilar(a, b) {
					continue
				}
				overlap := jaccard(dirNames[a], dirNames[b])
				if overlap < nameOverlapThreshold {
					continue
				}
				target, source := pickTarget(a, b, dirNames)
				suggestions = append(suggestions, domain.MergeSuggestion{
					ID:         suggestID(target, source),
					TargetDir:  target,
					SourceDirs: []string{source},
					Reason:     "兄弟目录名称相似（去除 backup/copy 后缀后相同）且文件名重叠度高",
					Confidence: overlap,
					Evidence: []string{
						"目录名相似：" + filepath.Base(a) + " ≈ " + filepath.Base(b),
						"文件名重叠度：" + formatPct(overlap),
						"合并建议仅为参考，须人工复核后生成计划（K-008）",
					},
				})
			}
		}
	}
	sort.SliceStable(suggestions, func(i, j int) bool {
		return suggestions[i].ID < suggestions[j].ID
	})
	return suggestions
}

// namesSimilar returns true when two directory basenames match after
// stripping backup/copy/numbering suffixes.
func namesSimilar(a, b string) bool {
	ba := dirSuffix.ReplaceAllString(filepath.Base(a), "")
	bb := dirSuffix.ReplaceAllString(filepath.Base(b), "")
	ba = strings.TrimRight(ba, "-_ ")
	bb = strings.TrimRight(bb, "-_ ")
	return ba != "" && strings.EqualFold(ba, bb)
}

// jaccard computes the Jaccard similarity of two file-name sets:
// |intersection| / |union|. Returns 0 when both sets are empty.
func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	intersection := 0
	for name := range a {
		if _, ok := b[name]; ok {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// pickTarget chooses the "canonical" directory as the merge target: the
// one whose base name has no backup/copy suffix (the original), or the one
// with more files if both/neither have suffixes.
func pickTarget(a, b string, names map[string]map[string]struct{}) (target, source string) {
	aHas := dirSuffix.MatchString(filepath.Base(a))
	bHas := dirSuffix.MatchString(filepath.Base(b))
	if aHas && !bHas {
		return b, a
	}
	if bHas && !aHas {
		return a, b
	}
	// Neither or both have suffixes: prefer the one with more files.
	if len(names[a]) >= len(names[b]) {
		return a, b
	}
	return b, a
}

func suggestID(target, source string) string {
	return "merge-" + shortHash(target) + "-" + shortHash(source)
}

func shortHash(s string) string {
	// Simple FNV-1a for a stable short id.
	var h uint32 = 2166136261
	for _, c := range s {
		h ^= uint32(c)
		h *= 16777619
	}
	const hex = "0123456789abcdef"
	return string([]byte{hex[(h>>12)&0xf], hex[(h>>8)&0xf], hex[(h>>4)&0xf], hex[h&0xf]})
}

func formatPct(f float64) string {
	return fmt.Sprintf("%.0f%%", f*100)
}
