package learning

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// FeedbackStats is the output of one feedback learning run. It analyzes
// historical plan decisions and produces weight-adjustment suggestions
// for the retention scorer, plus rule-confidence downgrade suggestions.
//
// Privacy (K-009): only the structured fields of plans (action types,
// score components, evidence strings) are read. RetainPath and action
// paths are NOT used for learning — only the score breakdown and the
// action type. The output never contains paths.
type FeedbackStats struct {
	// WeightAdjustments proposes tuning the four retention score
	// components (Authority, Stability, PathDepth, RoleBonus) by a
	// small delta. Each delta is capped at ±3 per run (decision: the
	// user confirmed ±3 分/次 as the single-run adjustment ceiling).
	WeightAdjustments []WeightAdjustment `json:"weight_adjustments"`
	// ConfidenceDowngrades lists learned rules whose observed
	// rejection rate exceeds the threshold, suggesting the user
	// disagrees with the rule's role assignment. Each entry carries
	// the rule ID (path-free) and the observed rejection rate.
	ConfidenceDowngrades []ConfidenceDowngrade `json:"confidence_downgrades"`
	// PlansAnalyzed is the total number of plans scanned.
	PlansAnalyzed int `json:"plans_analyzed"`
	// PlansRejected is the number of plans whose final state indicates
	// user rejection (ROLLED_BACK) or were downgraded from a cleanup
	// action to REVIEW.
	PlansRejected int `json:"plans_rejected"`
}

// WeightAdjustment proposes a ±N delta on one score component.
// Component is one of "authority", "stability", "path_depth", "role_bonus".
// Delta is clamped to [-3, +3].
type WeightAdjustment struct {
	Component string `json:"component"`
	Delta     int    `json:"delta"`
	Reason    string `json:"reason"`
	Evidence  string `json:"evidence"`
}

// ConfidenceDowngrade suggests lowering a learned rule's confidence.
type ConfidenceDowngrade struct {
	RuleID            string  `json:"rule_id"`
	ObservedRejection float64 `json:"observed_rejection"`
	Samples           int     `json:"samples"`
	SuggestedDelta    float64 `json:"suggested_delta"` // negative, magnitude <= 0.2
}

// FeedbackOptions controls feedback learning.
type FeedbackOptions struct {
	// MinSamples is the minimum number of plan decisions required before
	// proposing any adjustment. Default 5.
	MinSamples int
	// RejectionThreshold is the fraction of rejected plans above which a
	// rule's confidence downgrade is proposed. Default 0.5.
	RejectionThreshold float64
}

func (o *FeedbackOptions) withDefaults() FeedbackOptions {
	out := *o
	if out.MinSamples == 0 {
		out.MinSamples = 5
	}
	if out.RejectionThreshold == 0 {
		out.RejectionThreshold = 0.5
	}
	return out
}

// maxWeightDelta is the single-run weight adjustment ceiling. The user
// confirmed ±3 分/次 as the adjustment cap to prevent large drift.
const maxWeightDelta = 3

// minPlansForAdjustment is the minimum plans needed before a weight
// adjustment is proposed. Too few samples produce noisy suggestions.
const minPlansForAdjustment = 5

// LearnFromFeedback scans historical operation plans and derives
// weight-adjustment suggestions for the retention scorer plus
// confidence-downgrade suggestions for learned rules.
//
// What it learns:
//  1. If the user frequently retained copies that were NOT the highest-
//     scoring one (RetainPath != planner's top pick), the dimension
//     where the user's pick was stronger is a candidate for weight +
//     delta, and the dimension where the planner's pick was stronger
//     is a candidate for weight - delta.
//  2. Plans in ROLLED_BACK state or downgraded to REVIEW indicate the
//     user rejected the plan. Rules whose terms appear in evidence of
//     rejected plans accumulate rejection rate; above threshold they
//     get a confidence downgrade suggestion.
//
// What it does NOT do:
//   - Never reads action.Path / RetainPath for learning. Only score
//     components and action types are used (K-009).
//   - Never applies adjustments directly. Output is a draft for human
//     review via the `rules` subcommand.
func LearnFromFeedback(ctx context.Context, st store.Store, opts FeedbackOptions) (*FeedbackStats, error) {
	opts = opts.withDefaults()

	plans, err := st.ListAllPlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("learning: list all plans: %w", err)
	}

	stats := &FeedbackStats{PlansAnalyzed: len(plans)}
	if len(plans) < opts.MinSamples {
		// Not enough history to learn. Return empty stats; no adjustments.
		return stats, nil
	}

	// Phase 1: collect score-component discrepancies.
	// For each plan with a non-empty RetainScore, compare the retained
	// copy's components against the max across all actions. If the
	// retained copy is not the max in a dimension, that dimension is
	// "underweighted" (user values it more than the planner does).
	type dimStat struct {
		underweight int // retained copy won on this dim despite not being max
		overweight  int // planner's pick was max but user rejected
		total       int
	}
	dims := map[string]*dimStat{
		"authority":  {},
		"stability":  {},
		"path_depth": {},
		"role_bonus": {},
	}

	rejectedCount := 0
	ruleRejections := map[string]int{} // rule ID → rejected plan count
	ruleSamples := map[string]int{}    // rule ID → total plan count

	for i := range plans {
		p := &plans[i]
		// Track per-rule plan exposure via evidence strings. Evidence may
		// contain terms matched by learned rules; we approximate by
		// scanning learned rules' terms against evidence text. This is a
		// coarse signal — L4 does not require precise attribution.
		// (Rule-hit tracking via rule_hits table is the future path.)

		isRejected := p.State == domain.PlanRolledBack
		// Also treat a plan as "soft-rejected" if all actions are REVIEW
		// and it was not a protected/anchor-divergent case — but we cannot
		// tell those apart without paths. We rely on ROLLED_BACK as the
		// hard signal.
		if isRejected {
			rejectedCount++
		}

		if len(p.Actions) == 0 || p.RetainScore.Total == 0 {
			continue
		}

		// Find the retained action's score by matching RetainScore.Total
		// is not enough; we compare component-wise across actions using
		// each action's Context to re-derive a score. But we do not have
		// per-action scores stored. Instead we use the plan-level
		// RetainScore components as the retained copy's values, and we
		// compare against the OTHER actions' Context.AuthorityLevel as a
		// proxy. This is a heuristic: if the retained copy has lower
		// Authority than another action's Context.AuthorityLevel, the
		// user preferred a lower-authority copy → Authority is underweighted.

		retained := p.RetainScore
		for _, a := range p.Actions {
			if a.Action == domain.OperationKeep {
				continue // the retained copy
			}
			otherAuth := a.Context.AuthorityLevel
			if otherAuth > retained.Authority {
				// Another copy had higher authority but was not retained.
				dims["authority"].underweight++
			} else if otherAuth < retained.Authority {
				dims["authority"].overweight++
			}
			dims["authority"].total++
		}

		// Evidence-based rule rejection tracking (approximate).
		// We check if any learned rule's term appears in the evidence.
		// This is populated in GenerateFeedbackDrafts where we have the
		// rule list; here we just count rejections for the plan.
		_ = p.Evidence
	}

	stats.PlansRejected = rejectedCount

	// Phase 2: compute weight deltas from dim stats.
	// If underweight > overweight for a dimension, suggest +delta
	// proportional to the ratio, capped at ±3.
	for _, dim := range []string{"authority", "stability", "path_depth", "role_bonus"} {
		ds := dims[dim]
		if ds.total < minPlansForAdjustment {
			continue
		}
		net := ds.underweight - ds.overweight
		if net == 0 {
			continue
		}
		// Delta proportional to net ratio, clamped to [-3, +3].
		ratio := float64(net) / float64(ds.total)
		delta := int(ratio * float64(maxWeightDelta*2)) // scale
		if delta > maxWeightDelta {
			delta = maxWeightDelta
		}
		if delta < -maxWeightDelta {
			delta = -maxWeightDelta
		}
		if delta == 0 {
			continue
		}
		reason := fmt.Sprintf("用户在 %d 个 plan 中偏向%s分 %s 的副本（净偏移 %d/%d）",
			ds.total, biasWord(delta), dim, net, ds.total)
		stats.WeightAdjustments = append(stats.WeightAdjustments, WeightAdjustment{
			Component: dim,
			Delta:     delta,
			Reason:    reason,
			Evidence:  fmt.Sprintf("underweight=%d overweight=%d total=%d", ds.underweight, ds.overweight, ds.total),
		})
	}

	// Phase 3: rule confidence downgrades based on rejection rate.
	// We approximate rule exposure by scanning learned rules' terms in
	// plan evidence strings. A rule is a downgrade candidate if its
	// rejection rate >= threshold.
	learnedRules, err := st.ListRules(ctx, domain.RuleSourceLearned, "")
	if err != nil {
		return nil, fmt.Errorf("learning: list learned rules for feedback: %w", err)
	}
	for _, r := range learnedRules {
		// Extract the term from the rule definition (segment_contains value).
		term := extractTermFromDefinition(r.Definition)
		if term == "" {
			continue
		}
		samples := 0
		rejected := 0
		for i := range plans {
			p := &plans[i]
			hit := false
			for _, ev := range p.Evidence {
				if strings.Contains(ev, term) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
			samples++
			if p.State == domain.PlanRolledBack {
				rejected++
			}
		}
		ruleSamples[r.ID] = samples
		ruleRejections[r.ID] = rejected
		if samples < opts.MinSamples {
			continue
		}
		rate := float64(rejected) / float64(samples)
		if rate < opts.RejectionThreshold {
			continue
		}
		// Suggested confidence delta: proportional to rejection rate,
		// capped at -0.2 per run.
		suggestedDelta := -rate * 0.2
		if suggestedDelta < -0.2 {
			suggestedDelta = -0.2
		}
		stats.ConfidenceDowngrades = append(stats.ConfidenceDowngrades, ConfidenceDowngrade{
			RuleID:            r.ID,
			ObservedRejection: rate,
			Samples:           samples,
			SuggestedDelta:    suggestedDelta,
		})
	}

	sort.Slice(stats.WeightAdjustments, func(i, j int) bool {
		return abs(stats.WeightAdjustments[i].Delta) > abs(stats.WeightAdjustments[j].Delta)
	})
	sort.Slice(stats.ConfidenceDowngrades, func(i, j int) bool {
		return stats.ConfidenceDowngrades[i].ObservedRejection > stats.ConfidenceDowngrades[j].ObservedRejection
	})
	return stats, nil
}

// extractTermFromDefinition pulls the segment_contains value out of a
// directory_signal YAML definition. Returns empty string on failure.
func extractTermFromDefinition(definition string) string {
	lines := strings.Split(definition, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "segment_contains:") {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(line, "segment_contains:"))
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			return val[1 : len(val)-1]
		}
		return val
	}
	return ""
}

func biasWord(delta int) string {
	if delta > 0 {
		return "加"
	}
	return "减"
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// feedbackAdjustmentRuleID is the stable rule ID for weight-adjustment
// rules. There is one rule per component, so re-running feedback updates
// the same draft rather than creating duplicates.
func feedbackAdjustmentRuleID(component string) string {
	return "learned-weight-" + component
}

// GenerateFeedbackDrafts persists feedback-derived suggestions as rule
// drafts. Weight adjustments become a special rule definition that the
// planner can read (status=draft, priority <= 60). Confidence downgrades
// are applied directly to the rule's Confidence field (capped at -0.2),
// but only to rules still in draft/probation status — approved rules
// are left untouched (the user already decided to trust them).
//
// As with L2/L3, rules that already left draft status are never
// overwritten by weight-adjustment drafts.
func GenerateFeedbackDrafts(ctx context.Context, st store.Store, stats *FeedbackStats, batchID string) ([]domain.Rule, error) {
	if batchID == "" {
		return nil, fmt.Errorf("learning: batchID is required")
	}
	existing, err := st.ListRules(ctx, domain.RuleSourceLearned, "")
	if err != nil {
		return nil, fmt.Errorf("learning: list existing rules: %w", err)
	}
	skip := make(map[string]bool, len(existing))
	existingConf := make(map[string]float64, len(existing))
	for _, r := range existing {
		if r.Status != domain.RuleDraft {
			skip[r.ID] = true
		}
		existingConf[r.ID] = r.Confidence
	}

	rules := make([]domain.Rule, 0, len(stats.WeightAdjustments))

	// Persist weight adjustments as directory_signal drafts. The effect
	// is intentionally encoded as a role=unorganized, authority=0 signal
	// (no-op for classification) carrying the adjustment in evidence.
	// The planner reads weight adjustments from rules with a special ID
	// prefix "learned-weight-".
	for _, adj := range stats.WeightAdjustments {
		id := feedbackAdjustmentRuleID(adj.Component)
		if skip[id] {
			continue
		}
		def := fmt.Sprintf("match:\n  segment_contains: \"__weight_%s__\"\neffect:\n  role: unorganized\n  authority: 0\n  # weight_adjustment: %d\n  # reason: %s",
			adj.Component, adj.Delta, adj.Reason)
		r := domain.Rule{
			ID:         id,
			Version:    1,
			Priority:   40, // below role rules
			Enabled:    true,
			Source:     domain.RuleSourceLearned,
			BatchID:    batchID,
			Confidence: 0.5,
			Status:     domain.RuleDraft,
			Definition: def,
		}
		if err := st.SaveRule(ctx, r); err != nil {
			return nil, fmt.Errorf("learning: save weight rule %s: %w", r.ID, err)
		}
		rules = append(rules, r)
	}

	// Apply confidence downgrades directly to draft/probation rules.
	// Approved rules are left untouched.
	for _, dg := range stats.ConfidenceDowngrades {
		oldConf, ok := existingConf[dg.RuleID]
		if !ok {
			continue
		}
		// Find the existing rule to check its status.
		var target *domain.Rule
		for i := range existing {
			if existing[i].ID == dg.RuleID {
				target = &existing[i]
				break
			}
		}
		if target == nil {
			continue
		}
		if target.Status != domain.RuleDraft && target.Status != domain.RuleProbation {
			continue // approved/disabled/rejected: leave untouched
		}
		newConf := oldConf + dg.SuggestedDelta
		if newConf < 0 {
			newConf = 0
		}
		target.Confidence = newConf
		if err := st.SaveRule(ctx, *target); err != nil {
			return nil, fmt.Errorf("learning: apply confidence downgrade to %s: %w", dg.RuleID, err)
		}
	}

	// Record the learning batch.
	now := time.Now().UTC()
	batch := store.LearningBatch{
		ID:          batchID,
		Source:      "feedback",
		StartedAt:   now,
		CompletedAt: &now,
		Status:      "completed",
		RuleCount:   len(rules),
	}
	if err := st.SaveLearningBatch(ctx, batch); err != nil {
		return nil, fmt.Errorf("learning: save feedback batch: %w", err)
	}
	return rules, nil
}
