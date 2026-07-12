package learning

import (
	"context"
	"fmt"
	"math"
	"time"

	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/store"
)

// GenerateDrafts converts learned Stats into persisted rule drafts.
//
// For each directory name stat with a non-unknown SuggestedRole it produces
// a directory_signal rule definition and saves it to the store with
// Source=learned, Status=draft. The rule ID is derived from the directory
// name so re-running learning on the same data refreshes the same draft
// rather than creating duplicates.
//
// Rules that already left draft status (approved/probation/disabled/rejected)
// are left untouched — human decisions are never silently overwritten.
//
// Project code patterns are recorded in Stats (visible to the CLI) but do
// not yet become rules: the current YAML parser only supports
// segment_contains (exact term) matching, not regex. Regex-based rules are
// a future extension.
//
// Privacy (K-009): rule definitions carry only the directory name term,
// never full paths or file names. The batch record stores no raw content.
// Priority safety (K-008): all generated rules have priority <= 60.
func GenerateDrafts(ctx context.Context, st store.Store, stats *Stats, batchID string) ([]domain.Rule, error) {
	if batchID == "" {
		return nil, fmt.Errorf("learning: batchID is required")
	}

	// Build a skip-set of learned rules that already left draft status, so
	// re-running learning never clobbers a human-approved or rejected rule.
	existing, err := st.ListRules(ctx, domain.RuleSourceLearned, "")
	if err != nil {
		return nil, fmt.Errorf("learning: list existing rules: %w", err)
	}
	skip := make(map[string]bool, len(existing))
	for _, r := range existing {
		if r.Status != domain.RuleDraft {
			skip[r.ID] = true
		}
	}

	now := time.Now().UTC()
	rules := make([]domain.Rule, 0, len(stats.DirStats))

	for i := range stats.DirStats {
		ds := &stats.DirStats[i]
		if ds.SuggestedRole == domain.RoleUnknown {
			continue
		}
		// Never emit learned rules targeting the sensitive role; sensitive
		// directories are already filtered by Learn() and builtin rules.
		if ds.SuggestedRole == domain.RoleSensitive {
			continue
		}
		role := ds.SuggestedRole
		authority := authorityForLearnedRole(role)
		rule := domain.Rule{
			ID:         ruleIDForName(ds.Name),
			Version:    1,
			Priority:   authority,
			Enabled:    true,
			Source:     domain.RuleSourceLearned,
			BatchID:    batchID,
			Confidence: confidence(ds),
			Status:     domain.RuleDraft,
			Definition: buildDefinition(ds.Name, role, authority),
		}
		if skip[rule.ID] {
			continue
		}
		if err := st.SaveRule(ctx, rule); err != nil {
			return nil, fmt.Errorf("learning: save rule %s: %w", rule.ID, err)
		}
		rules = append(rules, rule)
	}

	// Record the learning batch.
	completed := time.Now().UTC()
	batch := store.LearningBatch{
		ID:          batchID,
		Source:      "stats",
		StartedAt:   now,
		CompletedAt: &completed,
		RuleCount:   len(rules),
		Status:      "completed",
	}
	if err := st.SaveLearningBatch(ctx, batch); err != nil {
		return nil, fmt.Errorf("learning: save batch: %w", err)
	}
	return rules, nil
}

// NewBatchID builds a human-readable batch identifier from the source and
// timestamp. Example: learn-stats-20260712-143051.
func NewBatchID(source string, now time.Time) string {
	return fmt.Sprintf("learn-%s-%s", source, now.UTC().Format("20060102-150405"))
}

// ruleIDForName builds a stable rule ID from the directory name. ASCII
// alphanumeric names (already lowercased by Learn) become a readable slug;
// non-ASCII names use an FNV-1a hash so the ID stays ASCII-safe and stable
// across filesystems. The YAML definition carries the actual term.
func ruleIDForName(name string) string {
	if isASCIISlugSafe(name) {
		return "learned-dir-" + name
	}
	return fmt.Sprintf("learned-dir-h%x", hashName(name))
}

func isASCIISlugSafe(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}

// hashName is a 64-bit FNV-1a hash over the string's runes.
func hashName(s string) uint64 {
	var h uint64 = 14695981039346656037
	for _, c := range s {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return h
}

// buildDefinition renders the minimal YAML understood by
// dircontext.parseDirectorySignal. The term is double-quoted; role uses the
// canonical domain constant string; authority is the learned priority (<=60).
func buildDefinition(name string, role domain.DirectoryRole, authority int) string {
	return fmt.Sprintf("match:\n  segment_contains: \"%s\"\neffect:\n  role: %s\n  authority: %d",
		name, string(role), authority)
}

// authorityForLearnedRole maps an inferred role to a learned-rule priority.
// Per K-008, learned rules never exceed 60, so protection roles (raw/archive/
// backup/system at builtin 90-100) are capped here; the builtin rule still
// wins on ties because it is sorted first.
func authorityForLearnedRole(role domain.DirectoryRole) int {
	switch role {
	case domain.RoleSystem, domain.RoleBackup, domain.RoleRaw, domain.RoleFormalArchive:
		return 60
	case domain.RoleProjectWork:
		return 60
	case domain.RoleUnorganized:
		return 45
	case domain.RoleTemporary:
		return 20
	case domain.RoleCache:
		return 10
	default:
		return 40
	}
}

// confidence estimates draft quality for human review. Inferred roles
// (co-occurrence backed) start at 0.5 and gain from evidence volume; the
// unorganized fallback stays low because it is a weak default.
func confidence(stat *DirNameStat) float64 {
	if stat.SuggestedRole == domain.RoleUnorganized {
		return clamp01(0.1 + float64(stat.FileCount)/200.0)
	}
	maxCo := 0
	for _, c := range stat.CoOccurrence {
		if c > maxCo {
			maxCo = c
		}
	}
	base := 0.5
	if stat.FileCount > 0 {
		base += math.Min(0.3, float64(maxCo)/float64(stat.FileCount)*0.5)
	}
	base += math.Min(0.2, float64(stat.DirCount)*0.05)
	return clamp01(base)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
