package planner

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"nas-data-governance/internal/dircontext"
	"nas-data-governance/internal/domain"
)

// Build creates reviewable recommendations only. It never touches the filesystem.
func Build(groups []domain.DuplicateGroup) []domain.OperationPlan {
	plans := make([]domain.OperationPlan, 0, len(groups))
	for _, group := range groups {
		plans = append(plans, buildGroup(group))
	}
	return plans
}

func buildGroup(group domain.DuplicateGroup) domain.OperationPlan {
	contexts := make([]domain.DirectoryContext, len(group.Files))
	for i, file := range group.Files {
		contexts[i] = dircontext.Classify(file.Path)
	}
	plan := domain.OperationPlan{ID: "dup-" + group.SHA256[:12], State: domain.PlanDraft, ContentSHA256: group.SHA256, Size: group.Size, Risk: domain.RiskHigh}
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
	role := contexts[0].Role
	retain := chooseRetain(group.Files, contexts)
	plan.RetainPath = retain.Path
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

func chooseRetain(files []domain.FileInstance, contexts []domain.DirectoryContext) domain.FileInstance {
	indices := make([]int, len(files))
	for i := range files {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		a, b := indices[i], indices[j]
		if contexts[a].AuthorityLevel != contexts[b].AuthorityLevel {
			return contexts[a].AuthorityLevel > contexts[b].AuthorityLevel
		}
		if files[a].ModifiedAt != files[b].ModifiedAt {
			return files[a].ModifiedAt.Before(files[b].ModifiedAt)
		}
		return strings.Compare(files[a].Path, files[b].Path) < 0
	})
	return files[indices[0]]
}

func reviewActions(files []domain.FileInstance, contexts []domain.DirectoryContext, reason string) []domain.PlannedAction {
	actions := make([]domain.PlannedAction, len(files))
	for i, file := range files {
		actions[i] = domain.PlannedAction{Path: file.Path, Action: domain.OperationReview, Reason: reason, Context: contexts[i]}
	}
	return actions
}

func quarantineActions(files []domain.FileInstance, contexts []domain.DirectoryContext, retainPath string) []domain.PlannedAction {
	actions := make([]domain.PlannedAction, len(files))
	for i, file := range files {
		action, reason := domain.OperationQuarantine, "同一低权威目录角色内的完全重复副本；待审批后隔离"
		if filepath.Clean(file.Path) == filepath.Clean(retainPath) {
			action, reason = domain.OperationKeep, "同组保留项：目录权威等级与稳定排序最优"
		}
		actions[i] = domain.PlannedAction{Path: file.Path, Action: action, Reason: reason, Context: contexts[i]}
	}
	return actions
}
