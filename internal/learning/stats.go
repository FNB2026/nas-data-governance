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

	"github.com/FNB2026/nas-data-governance/internal/dircontext"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// DirNameStat records how frequently a directory name appears across the
// indexed file tree. It is the raw material for generating learned rules.
type DirNameStat struct {
	Name          string               `json:"name"`
	DirCount      int                  `json:"dir_count"`     // unique parent paths containing this name
	FileCount     int                  `json:"file_count"`    // total files in directories with this name
	CoOccurrence  map[string]int       `json:"co_occurrence"` // builtin term → co-occurrence count
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
	DirStats         []DirNameStat     `json:"dir_stats"`
	ProjectCodes     []ProjectCodeStat `json:"project_codes"`
	TotalFiles       int               `json:"total_files"`
	SensitiveSkipped int               `json:"sensitive_skipped"`
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
		// Normalize root: only segments BELOW the storage root are business
		// directory names. Segments at or above root (system paths like
		// /private/var/folders/...) must never become rule candidates or
		// leak into rule IDs (K-009). Files whose path is not under root
		// are skipped defensively.
		rootClean := normalizeRoot(storage.RootPath)

		files, err := st.ListFiles(ctx, storage.ID)
		if err != nil {
			return nil, fmt.Errorf("learning: list files for %s: %w", storage.ID, err)
		}
		for _, file := range files {
			totalFiles++
			segments := extractSegmentsBelow(file.Path, rootClean)
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

				// Count unique parent paths. Use the relative path so the
				// same business layout across two storages counts twice.
				parentPath := filepath.ToSlash(filepath.Dir(file.Path))
				if rootClean != "" {
					parentPath = strings.TrimPrefix(parentPath, rootClean+"/")
				}
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
			originalSegs := extractOriginalSegmentsBelow(file.Path, rootClean)
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
		TotalFiles:       totalFiles,
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

// normalizeRoot cleans and slash-normalizes a storage root path. Returns
// empty string when rootPath is empty (in which case the caller skips the
// storage defensively — no segments are extracted, so no rules are learned
// from a storage whose root is unknown).
func normalizeRoot(rootPath string) string {
	if rootPath == "" {
		return ""
	}
	clean := filepath.Clean(filepath.ToSlash(rootPath))
	if clean == "." || clean == "/" {
		return ""
	}
	return clean
}

// extractSegmentsBelow returns lowercase directory-name segments that
// are strictly below rootClean. If rootClean is empty or the path is not
// under root, returns nil (the file is skipped). The file name itself
// is excluded — only directory segments are returned.
//
// This is the K-009 boundary: segments at or above the storage root
// (system paths like /private/var/folders/...) never become rule
// candidates and never leak into rule IDs.
func extractSegmentsBelow(path, rootClean string) []string {
	rel := relBelowRoot(path, rootClean)
	if rel == "" {
		return nil
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		// File is directly under root: no directory segments to learn from.
		return nil
	}
	return strings.FieldsFunc(strings.ToLower(dir), func(r rune) bool {
		return r == '/' || r == '\\'
	})
}

// extractOriginalSegmentsBelow is the original-case variant used for
// project code matching (project codes are uppercase by convention).
func extractOriginalSegmentsBelow(path, rootClean string) []string {
	rel := relBelowRoot(path, rootClean)
	if rel == "" {
		return nil
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		return nil
	}
	return strings.FieldsFunc(dir, func(r rune) bool {
		return r == '/' || r == '\\'
	})
}

// relBelowRoot returns the portion of path below rootClean as a
// slash-normalized relative path. Returns empty string when path is not
// under root (e.g., absolute path from a different storage) or when root
// is empty.
func relBelowRoot(path, rootClean string) string {
	if rootClean == "" {
		return ""
	}
	p := filepath.ToSlash(path)
	// Case-insensitive prefix match is not needed on POSIX, but we use
	// exact prefix match for simplicity and predictability. If the path
	// does not start with rootClean + "/", it is outside this storage.
	prefix := rootClean + "/"
	if !strings.HasPrefix(p, prefix) {
		return ""
	}
	return strings.TrimPrefix(p, prefix)
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
