package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	idx "github.com/FNB2026/nas-data-governance/internal/index"
	"github.com/FNB2026/nas-data-governance/internal/merge"
	"github.com/FNB2026/nas-data-governance/internal/privatefs"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// runReview 是 P1-5 人工复核管理界面的入口。它是一个交互式 CLI，
// 支持四类复核对象：plans / rules / merges / conflicts。
//
// 用法：
//
//	nas-governance review plans   --db ./var/governance.db [--plan-file ./var/plan.json]
//	nas-governance review rules   --db ./var/governance.db
//	nas-governance review merges  --index ./var/index.jsonl [--out ./var/plan.json]
//	nas-governance review conflicts --db ./var/governance.db
//
// 所有操作都是只读 + 显式确认，不自动执行任何文件系统动作。
func runReview(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("review requires a subcommand: plans|rules|merges|conflicts")
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "plans":
		return runReviewPlans(rest)
	case "rules":
		return runReviewRules(rest)
	case "merges":
		return runReviewMerges(rest)
	case "conflicts":
		return runReviewConflicts(rest)
	default:
		return fmt.Errorf("unknown review subcommand: %s", sub)
	}
}

// ============================================================
// review plans：列出含 REVIEW action 的 plan，交互式审阅
// ============================================================

func runReviewPlans(args []string) error {
	fs := flag.NewFlagSet("review plans", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database (optional, for loading plans)")
	planFile := fs.String("plan-file", "./var/plan.json", "plan JSON file")
	approvedOut := fs.String("out", "./var/approved.json", "output file for approved plans")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var plans []domain.OperationPlan
	if *dbPath != "" {
		st, err := store.Open(ctx, *dbPath)
		if err != nil {
			return err
		}
		defer st.Close()
		// 从 DB 加载所有 task 的 plan
		tasks, err := st.ListTasks(ctx)
		if err != nil {
			return err
		}
		for _, t := range tasks {
			taskPlans, err := st.ListPlans(ctx, t.ID)
			if err != nil {
				return err
			}
			plans = append(plans, taskPlans...)
		}
	} else {
		var err error
		plans, err = readPlans(*planFile)
		if err != nil {
			return err
		}
	}

	// 筛选含 REVIEW action 的 plan
	var reviewPlans []domain.OperationPlan
	for _, p := range plans {
		for _, a := range p.Actions {
			if a.Action == domain.OperationReview {
				reviewPlans = append(reviewPlans, p)
				break
			}
		}
	}

	if len(reviewPlans) == 0 {
		fmt.Println("no plans with REVIEW actions found")
		return nil
	}

	fmt.Printf("found %d plan(s) with REVIEW actions\n\n", len(reviewPlans))

	reader := bufio.NewReader(os.Stdin)
	var acknowledged []domain.OperationPlan
	quit := false
	for len(reviewPlans) > 0 && !quit {
		p := reviewPlans[0]
		displayPlanReview(p)
		fmt.Printf("\n[%d/%d] keep all files? [k]eep-all (REVIEW→SKIP) / [n]o (leave REVIEW) / [q]uit: ", len(acknowledged)+1, len(reviewPlans))
		resp, _ := reader.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
		switch resp {
		case "k", "keep", "keep-all", "y", "yes":
			// 将 REVIEW action 转为 SKIP（保留所有文件，不执行任何文件操作）。
			// 注意：这不是执行批准（APPROVED），而是人工确认保留全部文件。
			for j := range p.Actions {
				if p.Actions[j].Action == domain.OperationReview {
					p.Actions[j].Action = domain.OperationSkip
					p.Actions[j].Reason = "keep-all by human review: " + p.Actions[j].Reason
				}
			}
			acknowledged = append(acknowledged, p)
			reviewPlans = reviewPlans[1:]
		case "n", "no":
			// 保留 REVIEW 标记，不做任何变更
			reviewPlans = reviewPlans[1:]
		case "q", "quit":
			fmt.Println("quitting review")
			quit = true
		default: // skip
			reviewPlans = reviewPlans[1:]
		}
	}

	if len(acknowledged) == 0 {
		fmt.Println("no plans acknowledged")
		return nil
	}

	// 写入已确认的 plan 文件（REVIEW 已转为 SKIP，不涉及执行批准）
	f, err := privatefs.Create(*approvedOut)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(acknowledged); err != nil {
		return err
	}
	fmt.Printf("\nacknowledged %d plan(s) → %s (REVIEW actions converted to SKIP, not approved for execution)\n", len(acknowledged), *approvedOut)
	return nil
}

func displayPlanReview(p domain.OperationPlan) {
	fmt.Printf("--- plan %s ---\n", p.ID)
	fmt.Printf("state: %s  risk: %s  size: %d bytes\n", p.State, p.Risk, p.Size)
	fmt.Printf("sha256: %s\n", p.ContentSHA256)
	if p.RetainPath != "" {
		fmt.Printf("retain: %s (score: %v)\n", filepath.Base(p.RetainPath), p.RetainScore)
	}
	fmt.Printf("evidence: %s\n", strings.Join(p.Evidence, "; "))
	fmt.Printf("actions (%d):\n", len(p.Actions))
	for i, a := range p.Actions {
		if a.Action == domain.OperationReview {
			fmt.Printf("  [%d] REVIEW  %s\n", i, filepath.Base(a.Path))
			fmt.Printf("       reason: %s\n", a.Reason)
			fmt.Printf("       role: %s\n", a.Context.Role)
		} else {
			fmt.Printf("  [%d] %-9s %s\n", i, a.Action, filepath.Base(a.Path))
		}
	}
}

// ============================================================
// review rules：填补 probation→approved 转正缺口，支持全链路
// ============================================================

func runReviewRules(args []string) error {
	fs := flag.NewFlagSet("review rules", flag.ContinueOnError)
	dbPath := fs.String("db", "./var/governance.db", "database path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	// 列出所有 draft 和 probation 规则
	drafts, err := st.ListRules(ctx, "", domain.RuleDraft)
	if err != nil {
		return err
	}
	probations, err := st.ListRules(ctx, "", domain.RuleProbation)
	if err != nil {
		return err
	}

	total := len(drafts) + len(probations)
	if total == 0 {
		fmt.Println("no rules pending review (no draft or probation rules)")
		return nil
	}

	fmt.Printf("rules pending review: %d draft, %d probation\n\n", len(drafts), len(probations))

	reader := bufio.NewReader(os.Stdin)
	processed := 0

	// 审阅 draft 规则：draft → probation 或 reject
	for _, r := range drafts {
		displayRule(r)
		fmt.Printf("\n[draft] [a]pprove→probation / [r]eject / [s]kip / [q]uit: ")
		resp, _ := reader.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
		switch resp {
		case "a", "approve":
			now := time.Now().UTC()
			if err := st.UpdateRuleStatus(ctx, r.ID, domain.RuleProbation, &now); err != nil {
				return err
			}
			fmt.Printf("  → %s now on probation\n", r.ID)
			processed++
		case "r", "reject":
			if err := st.UpdateRuleStatus(ctx, r.ID, domain.RuleRejected, nil); err != nil {
				return err
			}
			fmt.Printf("  → %s rejected\n", r.ID)
			processed++
		case "q", "quit":
			fmt.Println("quitting review")
			fmt.Printf("\nprocessed %d/%d rules\n", processed, total)
			return nil
		default:
			fmt.Printf("  → skipped %s\n", r.ID)
		}
	}

	// 审阅 probation 规则：probation → approved 或 reject 或 disable
	for _, r := range probations {
		displayRule(r)
		fmt.Printf("\n[probation] [p]romote→approved / [r]eject / [d]isable / [s]kip / [q]uit: ")
		resp, _ := reader.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
		switch resp {
		case "p", "promote":
			now := time.Now().UTC()
			if err := st.UpdateRuleStatus(ctx, r.ID, domain.RuleApproved, &now); err != nil {
				return err
			}
			fmt.Printf("  → %s now approved\n", r.ID)
			processed++
		case "r", "reject":
			if err := st.UpdateRuleStatus(ctx, r.ID, domain.RuleRejected, nil); err != nil {
				return err
			}
			fmt.Printf("  → %s rejected\n", r.ID)
			processed++
		case "d", "disable":
			if err := st.UpdateRuleStatus(ctx, r.ID, domain.RuleDisabled, nil); err != nil {
				return err
			}
			fmt.Printf("  → %s disabled\n", r.ID)
			processed++
		case "q", "quit":
			fmt.Println("quitting review")
			fmt.Printf("\nprocessed %d/%d rules\n", processed, total)
			return nil
		default:
			fmt.Printf("  → skipped %s\n", r.ID)
		}
	}

	fmt.Printf("\nprocessed %d/%d rules\n", processed, total)
	return nil
}

func displayRule(r domain.Rule) {
	fmt.Printf("--- rule %s ---\n", r.ID)
	fmt.Printf("status: %s  source: %s  priority: %d  confidence: %.2f\n",
		r.Status, r.Source, r.Priority, r.Confidence)
	if r.BatchID != "" {
		fmt.Printf("batch: %s\n", r.BatchID)
	}
	if r.Definition != "" {
		// 只显示前 200 字符避免刷屏
		def := r.Definition
		if len(def) > 200 {
			def = def[:200] + "..."
		}
		fmt.Printf("definition: %s\n", def)
	}
}

// ============================================================
// review merges：列出合并建议，确认后生成 plan
// ============================================================

func runReviewMerges(args []string) error {
	fs := flag.NewFlagSet("review merges", flag.ContinueOnError)
	indexFile := fs.String("index", "./var/index.jsonl", "index file from scan")
	out := fs.String("out", "./var/merge-plan.json", "output plan file for approved merges")
	if err := fs.Parse(args); err != nil {
		return err
	}

	files, err := idx.Read(*indexFile)
	if err != nil {
		return err
	}

	suggestions := merge.Suggest(files)
	if len(suggestions) == 0 {
		fmt.Println("no merge suggestions found")
		return nil
	}

	fmt.Printf("found %d merge suggestion(s)\n\n", len(suggestions))

	reader := bufio.NewReader(os.Stdin)
	var approved []domain.OperationPlan
	quit := false
	for i := 0; i < len(suggestions) && !quit; i++ {
		s := suggestions[i]
		displayMergeSuggestion(s, i+1, len(suggestions))
		fmt.Printf("\napprove this merge? [y]es / [n]o / [q]uit: ")
		resp, _ := reader.ReadString('\n')
		resp = strings.TrimSpace(strings.ToLower(resp))
		switch resp {
		case "y", "yes":
			plan := mergeSuggestionToPlan(s)
			approved = append(approved, plan)
			fmt.Printf("  → approved (plan %s)\n", plan.ID)
		case "q", "quit":
			fmt.Println("quitting review")
			quit = true
		default:
			fmt.Printf("  → skipped\n")
		}
	}

	if len(approved) == 0 {
		fmt.Println("no merges approved")
		return nil
	}

	f, err := privatefs.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(approved); err != nil {
		return err
	}
	fmt.Printf("\n%d merge plan(s) → %s (DRAFT state, awaiting approve command)\n", len(approved), *out)
	return nil
}

func displayMergeSuggestion(s domain.MergeSuggestion, idx, total int) {
	fmt.Printf("--- merge [%d/%d] %s ---\n", idx, total, s.ID)
	fmt.Printf("target: %s\n", filepath.Base(s.TargetDir))
	fmt.Printf("sources (%d):\n", len(s.SourceDirs))
	for _, src := range s.SourceDirs {
		fmt.Printf("  - %s\n", filepath.Base(src))
	}
	fmt.Printf("reason: %s\n", s.Reason)
	fmt.Printf("confidence: %.2f\n", s.Confidence)
	if len(s.Evidence) > 0 {
		fmt.Printf("evidence: %s\n", strings.Join(s.Evidence, "; "))
	}
}

// mergeSuggestionToPlan 把合并建议转换为 DRAFT 状态的 plan。
// 每个 source 目录生成一个 MOVE action（target = target_dir）。
// 所有 action 标记为 REVIEW（人工确认后才能执行）。
func mergeSuggestionToPlan(s domain.MergeSuggestion) domain.OperationPlan {
	actions := make([]domain.PlannedAction, 0, len(s.SourceDirs))
	for _, src := range s.SourceDirs {
		actions = append(actions, domain.PlannedAction{
			Path:       src,
			Action:     domain.OperationReview, // 合并需要人工确认
			TargetPath: s.TargetDir,
			Reason:     fmt.Sprintf("merge into %s (confidence: %.2f)", filepath.Base(s.TargetDir), s.Confidence),
		})
	}
	return domain.OperationPlan{
		ID:       "merge-" + s.ID,
		State:    domain.PlanDraft,
		Risk:     domain.RiskMedium,
		Actions:  actions,
		Evidence: append([]string{"generated from merge suggestion"}, s.Evidence...),
	}
}

// ============================================================
// review conflicts：检测保护/清理规则冲突，显式进入复核
// ============================================================

// conflictDetector 检测保护规则与清理规则之间的冲突。
// 一个文件同时匹配保护规则（priority >= 90）和清理规则（priority <= 60）
// 时，视为冲突，需要人工复核（AGENTS rule 5, K-008）。
func runReviewConflicts(args []string) error {
	fs := flag.NewFlagSet("review conflicts", flag.ContinueOnError)
	dbPath := fs.String("db", "./var/governance.db", "database path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	// 加载所有已批准的规则
	approvedRules, err := st.ListRules(ctx, "", domain.RuleApproved)
	if err != nil {
		return err
	}
	// 加载内置规则
	builtinRules, err := st.ListRules(ctx, domain.RuleSourceBuiltin, "")
	if err != nil {
		return err
	}
	allRules := append(approvedRules, builtinRules...)

	// 按优先级分组：保护规则 (>= 90) vs 清理规则 (<= 60)
	var protectRules, cleanRules []domain.Rule
	for _, r := range allRules {
		if !r.Enabled {
			continue
		}
		if r.Priority >= 90 {
			protectRules = append(protectRules, r)
		} else if r.Priority <= 60 {
			cleanRules = append(cleanRules, r)
		}
	}

	if len(protectRules) == 0 || len(cleanRules) == 0 {
		fmt.Println("no potential conflicts: need both protection (priority >= 90) and cleanup (priority <= 60) rules")
		return nil
	}

	// 检测冲突：同名规则可能产生冲突
	// 例如：一个规则保护 "临时" 目录，另一个规则清理 "临时" 文件名
	conflicts := detectRuleConflicts(protectRules, cleanRules)

	if len(conflicts) == 0 {
		fmt.Printf("no conflicts detected (%d protection rules, %d cleanup rules)\n",
			len(protectRules), len(cleanRules))
		return nil
	}

	fmt.Printf("detected %d potential conflict(s):\n\n", len(conflicts))
	for i, c := range conflicts {
		fmt.Printf("[%d] PROTECTION: %s (priority %d)\n", i+1, c.ProtectID, c.ProtectPriority)
		fmt.Printf("    CLEANUP:    %s (priority %d)\n", c.CleanID, c.CleanPriority)
		fmt.Printf("    reason: %s\n", c.Reason)
		fmt.Printf("    action: review affected files manually before executing cleanup\n\n")
	}

	fmt.Printf("conflicts require manual resolution (AGENTS rule 5: protection > cleanup)\n")
	return nil
}

// ruleConflict 描述两条规则之间的潜在冲突。
type ruleConflict struct {
	ProtectID       string
	ProtectPriority int
	CleanID         string
	CleanPriority   int
	Reason          string
}

// detectRuleConflicts 检测保护规则与清理规则之间的命名冲突。
// 这是一个启发式检测：检查规则定义中的关键词重叠。
func detectRuleConflicts(protect, clean []domain.Rule) []ruleConflict {
	var conflicts []ruleConflict
	for _, p := range protect {
		for _, c := range clean {
			// 启发式：如果两条规则的 ID 前缀相同（来自同一目录名学习），
			// 或者定义中包含相同的目录名关键词，视为潜在冲突。
			pKeywords := extractKeywords(p.ID, p.Definition)
			cKeywords := extractKeywords(c.ID, c.Definition)
			overlap := keywordOverlap(pKeywords, cKeywords)
			if len(overlap) > 0 {
				conflicts = append(conflicts, ruleConflict{
					ProtectID:       p.ID,
					ProtectPriority: p.Priority,
					CleanID:         c.ID,
					CleanPriority:   c.Priority,
					Reason:          fmt.Sprintf("shared keywords: %s (protection overrides cleanup per K-008)", strings.Join(overlap, ", ")),
				})
			}
		}
	}
	return conflicts
}

// extractKeywords 从规则 ID 和定义中提取关键词（小写化）。
func extractKeywords(id, def string) []string {
	// 简化：用非字母数字字符分割，过滤空和短词
	text := strings.ToLower(id + " " + def)
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-'
	})
	var keywords []string
	for _, p := range parts {
		if len(p) >= 3 { // 忽略太短的词
			keywords = append(keywords, p)
		}
	}
	return keywords
}

// keywordOverlap 返回两个关键词集合的交集。
func keywordOverlap(a, b []string) []string {
	bset := make(map[string]bool, len(b))
	for _, x := range b {
		bset[x] = true
	}
	var overlap []string
	seen := make(map[string]bool)
	for _, x := range a {
		if bset[x] && !seen[x] {
			overlap = append(overlap, x)
			seen[x] = true
		}
	}
	return overlap
}
