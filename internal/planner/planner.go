package planner

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/dircontext"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/filepolicy"
)

// Build creates reviewable recommendations only. It never touches the filesystem.
func Build(groups []domain.DuplicateGroup) []domain.OperationPlan {
	return BuildAt(groups, time.Now())
}

// BuildAt is the time-injectable form for deterministic tests.
func BuildAt(groups []domain.DuplicateGroup, now time.Time) []domain.OperationPlan {
	plans := make([]domain.OperationPlan, 0, len(groups))
	for _, group := range groups {
		plans = append(plans, buildGroup(group, now))
	}
	return plans
}

func buildGroup(group domain.DuplicateGroup, now time.Time) domain.OperationPlan {
	contexts := make([]domain.DirectoryContext, len(group.Files))
	for i, file := range group.Files {
		contexts[i] = dircontext.Classify(file.Path)
	}
	plan := domain.OperationPlan{ID: planID(group), State: domain.PlanDraft, ContentSHA256: group.SHA256, Size: group.Size, Risk: domain.RiskHigh}
	if len(group.SHA256) != 64 {
		plan.Risk = domain.RiskCritical
		plan.Evidence = []string{"重复组缺少有效的完整 SHA-256", "无法证明字节级完全重复，禁止生成清理建议"}
		plan.Actions = reviewActions(group.Files, contexts, "完整哈希无效，必须重新扫描并人工复核")
		return plan
	}
	if anyDependencyProtected(group.Files) {
		plan.Risk = domain.RiskCritical
		plan.Evidence = []string{
			"重复组包含项目源文件、元数据侧车或可再生缓存",
			"侧车依赖尚未验证；保护规则优先于缓存或临时目录清理规则",
		}
		plan.Actions = reviewActions(group.Files, contexts, "项目/侧车依赖必须验证并人工复核")
		return plan
	}
	if anyProtected(contexts) {
		plan.Risk = domain.RiskCritical
		plan.Evidence = []string{"至少一个副本位于受保护目录（敏感、原始、备份或系统目录）", "白皮书要求保护范围不得参与普通自动去重"}
		plan.Actions = reviewActions(group.Files, contexts, "受保护目录中的相同内容必须人工复核")
		return plan
	}
	if !sameRole(contexts) {
		plan.Evidence = []string{"副本目录角色不同", "内容相同不等于存储职责相同；可能属于交叉归档或不同业务用途"}
		plan.Actions = reviewActions(group.Files, contexts, "目录职责不同，默认保留并人工复核")
		return plan
	}
	if anchorsDiverge(contexts) {
		plan.Evidence = []string{"副本业务锚点不同", "即便目录角色一致，业务锚点不同意味着承载不同业务用途"}
		plan.Actions = reviewActions(group.Files, contexts, "业务锚点不同，默认保留并人工复核")
		return plan
	}
	role := contexts[0].Role
	retain, score := chooseRetain(group.Files, contexts, now)
	plan.RetainPath = retain.Path
	plan.RetainScore = score
	if role == domain.RoleTemporary || role == domain.RoleCache {
		plan.Risk = domain.RiskMedium
		plan.Evidence = []string{fmt.Sprintf("所有副本均位于 %s 目录", role), "完整 SHA-256 一致", "建议隔离而非永久删除；执行前仍需状态复核和审批"}
		plan.Actions = quarantineActions(group.Files, contexts, retain.Path)
		return plan
	}
	plan.Evidence = []string{fmt.Sprintf("所有副本均位于同一目录角色：%s", role), "完整 SHA-256 一致", "非临时/缓存资料仍可能承载版本或业务语境，需人工确认"}
	plan.Actions = reviewActions(group.Files, contexts, "同职责重复：系统可建议，但必须人工确认")
	return plan
}

func anyDependencyProtected(files []domain.FileInstance) bool {
	for _, file := range files {
		if filepolicy.IsDependencyProtected(file.Path) {
			return true
		}
	}
	return false
}

func planID(group domain.DuplicateGroup) string {
	if len(group.SHA256) >= 12 {
		return "dup-" + group.SHA256[:12]
	}
	seed := group.SHA256
	for _, file := range group.Files {
		seed += "\x00" + file.Path
	}
	sum := sha256.Sum256([]byte(seed))
	return fmt.Sprintf("dup-invalid-%x", sum[:6])
}

func anyProtected(contexts []domain.DirectoryContext) bool {
	for _, c := range contexts {
		if c.Protected {
			return true
		}
	}
	return false
}
func sameRole(contexts []domain.DirectoryContext) bool {
	for _, c := range contexts[1:] {
		if c.Role != contexts[0].Role {
			return false
		}
	}
	return true
}

// anchorsDiverge returns true when at least two contexts carry a non-empty
// business anchor and they are not all equal. Empty anchors are ignored —
// lack of a detectable anchor should not by itself trigger review.
func anchorsDiverge(contexts []domain.DirectoryContext) bool {
	seen := map[string]struct{}{}
	count := 0
	for _, c := range contexts {
		if c.BusinessAnchor == "" {
			continue
		}
		seen[c.BusinessAnchor] = struct{}{}
		count++
	}
	return count >= 2 && len(seen) > 1
}

// chooseRetain picks the highest-scoring copy. Ties fall back to older mtime
// then to lexicographic path, matching the previous stable ordering so
// existing behavior is preserved when scores are equal.
func chooseRetain(files []domain.FileInstance, contexts []domain.DirectoryContext, now time.Time) (domain.FileInstance, domain.RetentionScore) {
	indices := make([]int, len(files))
	for i := range files {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		a, b := indices[i], indices[j]
		sa := ScoreRetention(files[a], contexts[a], now)
		sb := ScoreRetention(files[b], contexts[b], now)
		if sa.Total != sb.Total {
			return sa.Total > sb.Total
		}
		if !files[a].ModifiedAt.Equal(files[b].ModifiedAt) {
			return files[a].ModifiedAt.Before(files[b].ModifiedAt)
		}
		return strings.Compare(files[a].Path, files[b].Path) < 0
	})
	winner := indices[0]
	return files[winner], ScoreRetention(files[winner], contexts[winner], now)
}

// ScoreRetention produces an explainable retention score for one duplicate copy.
//   - Authority: directory authority level (0-100), from DirectoryContext.
//   - Stability: age in days / 7, capped at 30. Older files are less likely
//     transient.
//   - PathDepth: directory depth, capped at 10. Deeper paths suggest more
//     intentional placement.
//   - RoleBonus: +20 for raw/formal archive (source of truth), -20 for
//     temporary/cache (likely disposable).
//
// Total = Authority + Stability + PathDepth + RoleBonus. Higher wins.
func ScoreRetention(file domain.FileInstance, ctx domain.DirectoryContext, now time.Time) domain.RetentionScore {
	s := domain.RetentionScore{}

	s.Authority = ctx.AuthorityLevel
	s.Reasons = append(s.Reasons, fmt.Sprintf("目录权威等级 %d（+%d）", ctx.AuthorityLevel, s.Authority))

	ageDays := now.Sub(file.ModifiedAt).Hours() / 24
	if ageDays < 0 {
		ageDays = 0
	}
	s.Stability = int(ageDays / 7)
	if s.Stability > 30 {
		s.Stability = 30
	}
	s.Reasons = append(s.Reasons, fmt.Sprintf("存在 %.0f 天（稳定性 +%d）", ageDays, s.Stability))

	depth := strings.Count(filepath.ToSlash(filepath.Dir(file.Path)), "/")
	if depth > 10 {
		depth = 10
	}
	if depth < 0 {
		depth = 0
	}
	s.PathDepth = depth
	s.Reasons = append(s.Reasons, fmt.Sprintf("目录深度 %d（+%d）", depth, depth))

	switch ctx.Role {
	case domain.RoleRaw, domain.RoleFormalArchive:
		s.RoleBonus = 20
		s.Reasons = append(s.Reasons, fmt.Sprintf("角色 %s：权威来源奖励 +20", ctx.Role))
	case domain.RoleTemporary, domain.RoleCache:
		s.RoleBonus = -20
		s.Reasons = append(s.Reasons, fmt.Sprintf("角色 %s：临时/缓存惩罚 -20", ctx.Role))
	default:
		s.Reasons = append(s.Reasons, fmt.Sprintf("角色 %s：无加减分", ctx.Role))
	}

	s.Total = s.Authority + s.Stability + s.PathDepth + s.RoleBonus
	return s
}

func reviewActions(files []domain.FileInstance, contexts []domain.DirectoryContext, reason string) []domain.PlannedAction {
	actions := make([]domain.PlannedAction, len(files))
	for i, file := range files {
		actions[i] = domain.PlannedAction{Path: file.Path, Action: domain.OperationReview, Reason: reason, Context: contexts[i], File: file}
	}
	return actions
}

func quarantineActions(files []domain.FileInstance, contexts []domain.DirectoryContext, retainPath string) []domain.PlannedAction {
	actions := make([]domain.PlannedAction, len(files))
	for i, file := range files {
		action, reason := domain.OperationQuarantine, "同一低权威目录角色内的完全重复副本；待审批后隔离"
		if filepath.Clean(file.Path) == filepath.Clean(retainPath) {
			action, reason = domain.OperationKeep, "同组保留项：保留评分最高"
		}
		actions[i] = domain.PlannedAction{Path: file.Path, Action: action, Reason: reason, Context: contexts[i], File: file}
	}
	return actions
}
