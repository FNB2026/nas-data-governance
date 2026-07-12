package dircontext

import (
	"sync"

	"nas-data-governance/internal/domain"
)

// maxLearnedPriority caps the priority of learned rules. Per K-008, learned
// rules never override protection rules (priority 90-100). They may sit at
// the directory-role level (60) or below.
const maxLearnedPriority = 60

// ruleSet holds builtin + learned signals. The zero value is ready to use
// with only builtin rules; call MergeLearned to add learned rules.
type ruleSet struct {
	mu      sync.RWMutex
	builtin []signal
	learned []signal
}

var defaultRules = &ruleSet{builtin: roleSignals}

// RuleVersion returns the current classifier rule version. It changes when
// learned rules are merged, so stored directory_contexts can be invalidated
// and recomputed.
func RuleVersion() string {
	defaultRules.mu.RLock()
	defer defaultRules.mu.RUnlock()
	if len(defaultRules.learned) == 0 {
		return "builtin-v1"
	}
	return "builtin-v1+learned-" + learnedHash()
}

// MergeLearned replaces the learned signal set with the given rules. Rules
// with priority > maxLearnedPriority are clamped down (K-008 enforcement).
// Rules are sorted by priority descending so the first match wins, matching
// the builtin classifier's behavior.
func MergeLearned(rules []domain.Rule) {
	signals := make([]signal, 0, len(rules))
	for _, r := range rules {
		if !r.Enabled || r.Status != domain.RuleApproved && r.Status != domain.RuleProbation {
			continue
		}
		// Parse the YAML definition into a signal. For L1 we only support
		// directory_signal rules with segment_contains match and role effect.
		sig, ok := parseDirectorySignal(r.Definition)
		if !ok {
			continue
		}
		if sig.authority > maxLearnedPriority {
			sig.authority = maxLearnedPriority
		}
		signals = append(signals, sig)
	}
	// Sort by authority descending so higher-priority signals match first.
	selectionSort(signals)
	defaultRules.mu.Lock()
	defaultRules.learned = signals
	defaultRules.mu.Unlock()
}

// ClearLearned removes all learned rules, restoring builtin-only behavior.
func ClearLearned() {
	defaultRules.mu.Lock()
	defaultRules.learned = nil
	defaultRules.mu.Unlock()
}

// activeSignals returns builtin and learned signals in one priority order.
// Learned signals are capped at 60, so builtin protection roles at 90-100
// always win while a learned directory-role rule can still refine low-priority
// temporary/cache/unorganized matches.
func activeSignals() []signal {
	defaultRules.mu.RLock()
	defer defaultRules.mu.RUnlock()
	out := make([]signal, 0, len(defaultRules.builtin)+len(defaultRules.learned))
	out = append(out, defaultRules.builtin...)
	out = append(out, defaultRules.learned...)
	selectionSort(out)
	return out
}

// BuiltinTermRoles returns the mapping of builtin directory name terms
// to their assigned roles. The learning module uses this to skip terms
// already covered by builtin rules and to suggest roles based on
// co-occurrence analysis.
func BuiltinTermRoles() map[string]domain.DirectoryRole {
	out := map[string]domain.DirectoryRole{}
	defaultRules.mu.RLock()
	defer defaultRules.mu.RUnlock()
	for _, sig := range defaultRules.builtin {
		for _, t := range sig.terms {
			out[t] = sig.role
		}
	}
	for _, t := range sensitiveTerms {
		out[t] = domain.RoleSensitive
	}
	return out
}

// ContainsSensitiveTerm returns true if any segment matches a sensitive
// term. Exported for the learning module to skip sensitive directories
// per K-009 (privacy: sensitive directories must not enter learning samples).
func ContainsSensitiveTerm(segments []string) bool {
	for _, term := range sensitiveTerms {
		if matches(segments, term) {
			return true
		}
	}
	return false
}

// learnedHash returns a short stable hash of the current learned signals.
func learnedHash() string {
	if len(defaultRules.learned) == 0 {
		return "none"
	}
	// Simple sum of role+authority+first-term for a cheap stable hash.
	var sum uint32 = 2166136261
	for _, s := range defaultRules.learned {
		sum ^= uint32(s.authority)
		sum *= 16777619
		for _, t := range s.terms {
			for _, c := range t {
				sum ^= uint32(c)
				sum *= 16777619
			}
		}
	}
	return string([]byte{
		hexDigit((sum >> 12) & 0xf),
		hexDigit((sum >> 8) & 0xf),
		hexDigit((sum >> 4) & 0xf),
		hexDigit(sum & 0xf),
	})
}

func hexDigit(n uint32) byte {
	const hex = "0123456789abcdef"
	return hex[n&0xf]
}

func selectionSort(s []signal) {
	for i := 0; i < len(s); i++ {
		max := i
		for j := i + 1; j < len(s); j++ {
			if s[j].authority > s[max].authority {
				max = j
			}
		}
		s[i], s[max] = s[max], s[i]
	}
}
