// Package learning implements local-only statistical rule induction.
// It reads the governance store's indexed file metadata and produces
// draft rules for human review. It never sends data externally and
// never writes to user file systems (K-009, AGENTS rule 1/6).
package learning

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"nas-data-governance/internal/dircontext"
	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/store"
)

// DirNameStat records how frequently a directory name appears across the
// indexed file tree. It is the raw material for generating learned rules.
type DirNameStat struct {
	Name         string            `json:"name"`
	DirCount     int               `json:"dir_count"`     // unique parent paths containing this name
	FileCount    int               `json:"file_count"`     // total files in directories with this name
	CoOccurrence map[string]int    `json:"co_occurrence"`  // builtin term → co-occurrence count
	SuggestedRole domain.DirectoryRole `json:"suggested_role"`
}

// ProjectCodeStat records a project code pattern and its frequency.
type ProjectCodeStat struct {
	Pattern string `json:"pattern"`
	Count   int    `json:"count"`
}

// Stats is the output of one learning run. It contains no raw paths or
// file names — only anonymized directory name frequencies and pattern counts.
type Stats struct {
	DirStats     []DirNameStat     `json:"dir_stats"`
	ProjectCodes []ProjectCodeStat `json:"project_codes"`
	TotalFiles   int               `json:"total_files"`
	SensitiveSkipped int           `json:"sensitive_skipped"`
}

// minDirCount is the minimum number of unique parent paths for a directory
// name to be considered a naming convention worth learning.
const minDirCount = 2

// minFileCount is the minimum total file count for a directory name to
// be considered significant (high-volume single directory).
const minFileCount = 5

// minProjectCodeCount is the minimum hits for a project code pattern.
const minProjectCodeCount = 2

// projectCodePattern matches stable identifiers like PRJ-2024-001,
// P2024001, CLIENT-A. Mirrors dircontext.projectCodePattern.
var projectCodePattern = regexp.MustCompile(`^[A-Z]{2,}[-_]\d{2,}[A-Z0-9-]*$`)

// Learn traverses the store's indexed files, collects directory name
// statistics, and returns the result. It is read-only: it never writes
// to the store. Rule drafts are generated separately by GenerateDrafts.
//
// Privacy (K-009): sensitive directories are skipped entirely; only
// directory names (not full paths) are retained in the output.
func Learn(ctx context.Context, st store.Store) (*Stats, error) {
	storages, err := st.ListStorages(ctx)
	if err != nil {
		return nil, fmt.Errorf("learning: list storages: %w", err)
	}

	builtinRoles := dircontext.BuiltinTermRoles()

	dirFreq := map[string]*DirNameStat{}
	dirParents := map[string]map[string]bool{} // name → set of parent paths (for unique counting)
	projectCodes := map[string]int{}
	totalFiles := 0
	sensitiveSkipped := 0

	for _, storage := range storages {
		files, err := st.ListFiles(ctx, storage.ID)
		if err != nil {
			return nil, fmt.Errorf("learning: list files for %s: %w", storage.ID, err)
		}
		for _, file := range files {
			totalFiles++
			segments := extractSegments(file.Path)
			if len(segments) == 0 {
				continue
			}

			// Skip sensitive directories entirely (K-009).
			if dircontext.ContainsSensitiveTerm(segments) {
				sensitiveSkipped++
				continue
			}

			// Track directory name frequency (lowercase for matching).
			for _, seg := range segments {
				name := strings.ToLower(seg)
				// Skip if this exact term is already covered by builtin rules.
				if _, exists := builtinRoles[name]; exists {
					continue
				}

				if dirFreq[name] == nil {
					dirFreq[name] = &DirNameStat{
						Name:         name,
						CoOccurrence: map[string]int{},
					}
					dirParents[name] = map[string]bool{}
				}

				// Count unique parent paths.
				parentPath := filepath.Dir(file.Path)
				if !dirParents[name][parentPath] {
					dirParents[name][parentPath] = true
					dirFreq[name].DirCount++
				}
				dirFreq[name].FileCount++

				// Track co-occurrence with builtin terms in the same path.
				for _, other := range segments {
					otherLower := strings.ToLower(other)
					if otherLower == name {
						continue
					}
					if role, ok := builtinRoles[otherLower]; ok && role != domain.RoleUnknown {
						dirFreq[name].CoOccurrence[otherLower]++
					}
				}
			}

			// Track project code patterns (case-sensitive, original segments).
			originalSegs := extractOriginalSegments(file.Path)
			for _, seg := range originalSegs {
				if projectCodePattern.MatchString(seg) {
					pattern := generalizeProjectCode(seg)
					projectCodes[pattern]++
				}
			}
		}
	}

	// Convert maps to sorted slices, applying thresholds.
	stats := &Stats{
		TotalFiles:      totalFiles,
		SensitiveSkipped: sensitiveSkipped,
	}
	for _, s := range dirFreq {
		if s.DirCount < minDirCount && s.FileCount < minFileCount {
			continue
		}
		s.SuggestedRole = suggestRole(s, builtinRoles)
		stats.DirStats = append(stats.DirStats, *s)
	}
	sort.Slice(stats.DirStats, func(i, j int) bool {
		if stats.DirStats[i].FileCount != stats.DirStats[j].FileCount {
			return stats.DirStats[i].FileCount > stats.DirStats[j].FileCount
		}
		return stats.DirStats[i].Name < stats.DirStats[j].Name
	})

	for pattern, count := range projectCodes {
		if count < minProjectCodeCount {
			continue
		}
		stats.ProjectCodes = append(stats.ProjectCodes, ProjectCodeStat{
			Pattern: pattern,
			Count:   count,
		})
	}
	sort.Slice(stats.ProjectCodes, func(i, j int) bool {
		return stats.ProjectCodes[i].Count > stats.ProjectCodes[j].Count
	})

	return stats, nil
}

// extractSegments returns lowercase path segments (excluding the file name).
func extractSegments(path string) []string {
	dir := filepath.Dir(filepath.ToSlash(path))
	segments := strings.FieldsFunc(strings.ToLower(dir), func(r rune) bool {
		return r == '/' || r == '\\'
	})
	return segments
}

// extractOriginalSegments returns original-case path segments (for project code matching).
func extractOriginalSegments(path string) []string {
	dir := filepath.Dir(filepath.ToSlash(path))
	segments := strings.FieldsFunc(dir, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	return segments
}

// suggestRole picks a role based on co-occurrence with builtin terms.
// If the directory name frequently appears alongside "归档" or "archive",
// it suggests RoleFormalArchive; alongside "项目"/"project", RoleProjectWork;
// etc. Falls back to RoleUnorganized for names with no clear signal.
//
// Ties (multiple terms with the same co-occurrence count) are broken
// deterministically: higher builtin authority wins (protection roles beat
// working/temporary), then lexicographic term order. This makes rule
// generation reproducible regardless of Go map iteration order.
func suggestRole(stat *DirNameStat, builtinRoles map[string]domain.DirectoryRole) domain.DirectoryRole {
	maxCo := 0
	bestAuthority := -1
	var bestTerm string
	for term, count := range stat.CoOccurrence {
		role, ok := builtinRoles[term]
		if !ok {
			continue
		}
		auth := builtinAuthorityFor(role)
		switch {
		case count > maxCo:
			maxCo, bestTerm, bestAuthority = count, term, auth
		case count == maxCo && count > 0:
			// Tie-break: higher builtin authority, then lexicographic term.
			if auth > bestAuthority || (auth == bestAuthority && (bestTerm == "" || term < bestTerm)) {
				bestTerm, bestAuthority = term, auth
			}
		}
	}
	if bestTerm != "" {
		if role, ok := builtinRoles[bestTerm]; ok {
			return role
		}
	}
	return domain.RoleUnorganized
}

// builtinAuthorityFor returns the builtin authority level for a role. It is
// used only to break co-occurrence ties deterministically in suggestRole
// (protection roles outrank working/temporary). Values mirror dircontext's
// builtin roleSignals so builtin rules still win on the real classifier.
func builtinAuthorityFor(role domain.DirectoryRole) int {
	switch role {
	case domain.RoleSystem:
		return 100
	case domain.RoleBackup:
		return 95
	case domain.RoleRaw, domain.RoleFormalArchive:
		return 90
	case domain.RoleProjectWork:
		return 65
	case domain.RoleUnorganized:
		return 45
	case domain.RoleTemporary:
		return 20
	case domain.RoleCache:
		return 10
	case domain.RoleSensitive:
		return 100
	default:
		return 0
	}
}

// generalizeProjectCode converts a specific code like "PRJ-2024-001" into
// a generalized pattern string like "[A-Z]+-\\d+-\\d+". Only the pattern
// is retained, never the actual code value (K-009).
func generalizeProjectCode(code string) string {
	var pattern strings.Builder
	i := 0
	for i < len(code) {
		switch {
		case code[i] >= 'A' && code[i] <= 'Z':
			pattern.WriteString("[A-Z]+")
			for i < len(code) && code[i] >= 'A' && code[i] <= 'Z' {
				i++
			}
		case code[i] >= '0' && code[i] <= '9':
			pattern.WriteString("\\d+")
			for i < len(code) && code[i] >= '0' && code[i] <= '9' {
				i++
			}
		default:
			pattern.WriteByte(code[i])
			i++
		}
	}
	return "^" + pattern.String() + "$"
}
