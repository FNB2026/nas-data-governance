// Package governancediag builds private, read-only P2 review material.
// It never persists plans, approves actions, or invokes the executor.
package governancediag

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nas-data-governance/internal/dircontext"
	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/planner"
	"nas-data-governance/internal/relations"
	"nas-data-governance/internal/report"
)

const RecommendationKeepReview = "KEEP_AND_REVIEW"

type Summary struct {
	Files                      int   `json:"files"`
	FormatRows                 int   `json:"format_rows"`
	MissingFormatRows          int   `json:"missing_format_rows"`
	DuplicateGroups            int   `json:"duplicate_groups"`
	DuplicateFiles             int   `json:"duplicate_files"`
	TheoreticalRedundantBytes  int64 `json:"theoretical_redundant_bytes"`
	DraftPlans                 int   `json:"draft_plans"`
	NonDraftPlans              int   `json:"non_draft_plans"`
	CriticalPlans              int   `json:"critical_plans"`
	ReviewActions              int   `json:"review_actions"`
	QuarantineCandidateActions int   `json:"quarantine_candidate_actions"`
	ZeroByteFiles              int   `json:"zero_byte_files"`
	LargeMediaFiles            int   `json:"large_media_files"`
	LargeMediaBytes            int64 `json:"large_media_bytes"`
	LargeMediaWithRelations    int   `json:"large_media_with_relations"`
	LargeMediaWithAnchor       int   `json:"large_media_with_business_anchor"`
	LargeMediaProjectWork      int   `json:"large_media_project_work"`
	LargeMediaProtected        int   `json:"large_media_protected"`
	LargeMediaMissingCodec     int   `json:"large_media_missing_codec"`
	LargeMediaMissingDuration  int   `json:"large_media_missing_duration"`
	MediaRelations             int   `json:"media_relations"`
}

type DuplicateReview struct {
	SHA256         string               `json:"sha256"`
	Size           int64                `json:"size"`
	Copies         int                  `json:"copies"`
	RedundantBytes int64                `json:"redundant_bytes"`
	Plan           domain.OperationPlan `json:"draft_plan"`
}

type ZeroByteReview struct {
	StorageID      string                  `json:"storage_id"`
	Path           string                  `json:"path"`
	Classification string                  `json:"classification"`
	Context        domain.DirectoryContext `json:"context"`
	Evidence       []string                `json:"evidence"`
	Recommendation string                  `json:"recommendation"`
}

type MediaAggregate struct {
	Format            string `json:"format"`
	Codec             string `json:"codec"`
	Files             int    `json:"files"`
	Bytes             int64  `json:"bytes"`
	LargeFiles        int    `json:"large_files"`
	MissingDuration   int    `json:"missing_duration"`
	MissingDimensions int    `json:"missing_dimensions"`
}

type LargeMediaReview struct {
	StorageID      string                  `json:"storage_id"`
	Path           string                  `json:"path"`
	Size           int64                   `json:"size"`
	Format         domain.FormatInfo       `json:"format"`
	Context        domain.DirectoryContext `json:"context"`
	RelationCount  int                     `json:"relation_count"`
	Recommendation string                  `json:"recommendation"`
	Evidence       []string                `json:"evidence"`
}

type Report struct {
	GeneratedAt         time.Time             `json:"generated_at"`
	LargeMediaMinimum   int64                 `json:"large_media_minimum"`
	ExecutionAuthorized bool                  `json:"execution_authorized"`
	Summary             Summary               `json:"summary"`
	DuplicateReviews    []DuplicateReview     `json:"duplicate_reviews"`
	ZeroByteReviews     []ZeroByteReview      `json:"zero_byte_reviews"`
	MediaAggregates     []MediaAggregate      `json:"media_aggregates"`
	LargeMediaReviews   []LargeMediaReview    `json:"large_media_reviews"`
	MediaRelations      []domain.FileRelation `json:"media_relations"`
	SafetyNotes         []string              `json:"safety_notes"`
}

type mediaKey struct{ format, codec string }

func Build(files []domain.FileInstance, largeMediaMinimum int64, now time.Time) Report {
	result := Report{
		GeneratedAt: now.UTC(), LargeMediaMinimum: largeMediaMinimum, ExecutionAuthorized: false,
		SafetyNotes: []string{
			"报告仅包含 DRAFT 建议，未写入审批状态，未调用执行器",
			"理论重复字节不等于可删除字节；目录职责、侧车依赖和保护规则优先",
			"零字节文件和媒体关系默认 KEEP_AND_REVIEW",
		},
	}
	result.Summary.Files = len(files)
	for _, file := range files {
		if file.Format.Format == "" {
			result.Summary.MissingFormatRows++
		} else {
			result.Summary.FormatRows++
		}
	}

	groups := report.DuplicateGroups(files)
	sort.Slice(groups, func(i, j int) bool { return groups[i].SHA256 < groups[j].SHA256 })
	plans := planner.BuildAt(groups, now)
	for i, group := range groups {
		plan := plans[i]
		review := DuplicateReview{
			SHA256: group.SHA256, Size: group.Size, Copies: len(group.Files),
			RedundantBytes: int64(len(group.Files)-1) * group.Size, Plan: plan,
		}
		result.DuplicateReviews = append(result.DuplicateReviews, review)
		result.Summary.DuplicateFiles += len(group.Files)
		result.Summary.TheoreticalRedundantBytes += review.RedundantBytes
		if plan.State == domain.PlanDraft {
			result.Summary.DraftPlans++
		} else {
			result.Summary.NonDraftPlans++
		}
		if plan.Risk == domain.RiskCritical {
			result.Summary.CriticalPlans++
		}
		for _, action := range plan.Actions {
			switch action.Action {
			case domain.OperationReview:
				result.Summary.ReviewActions++
			case domain.OperationQuarantine:
				result.Summary.QuarantineCandidateActions++
			}
		}
	}
	result.Summary.DuplicateGroups = len(result.DuplicateReviews)

	for _, file := range files {
		if file.Size != 0 {
			continue
		}
		ctx := dircontext.Classify(file.Path)
		classification, evidence := classifyZeroByte(file, ctx)
		result.ZeroByteReviews = append(result.ZeroByteReviews, ZeroByteReview{
			StorageID: file.StorageID, Path: file.Path, Classification: classification,
			Context: ctx, Evidence: evidence, Recommendation: RecommendationKeepReview,
		})
	}
	sort.Slice(result.ZeroByteReviews, func(i, j int) bool { return result.ZeroByteReviews[i].Path < result.ZeroByteReviews[j].Path })
	result.Summary.ZeroByteFiles = len(result.ZeroByteReviews)

	allRelations := relations.Relations(files)
	mediaPaths := make(map[string]bool)
	for _, file := range files {
		if isMedia(file.Format) {
			mediaPaths[file.Path] = true
		}
	}
	relationCounts := map[string]int{}
	for _, relation := range allRelations {
		if !mediaPaths[relation.A] && !mediaPaths[relation.B] {
			continue
		}
		result.MediaRelations = append(result.MediaRelations, relation)
		relationCounts[relation.A]++
		relationCounts[relation.B]++
	}
	result.Summary.MediaRelations = len(result.MediaRelations)

	aggregates := map[mediaKey]*MediaAggregate{}
	for _, file := range files {
		if !isMedia(file.Format) {
			continue
		}
		codec := file.Format.Codec
		if codec == "" {
			codec = "unknown"
		}
		key := mediaKey{format: file.Format.Format, codec: codec}
		aggregate := aggregates[key]
		if aggregate == nil {
			aggregate = &MediaAggregate{Format: key.format, Codec: key.codec}
			aggregates[key] = aggregate
		}
		aggregate.Files++
		aggregate.Bytes += file.Size
		if file.Format.Duration <= 0 {
			aggregate.MissingDuration++
		}
		if file.Format.Category == domain.CategoryVideo && (file.Format.Width <= 0 || file.Format.Height <= 0) {
			aggregate.MissingDimensions++
		}
		if file.Size < largeMediaMinimum {
			continue
		}
		aggregate.LargeFiles++
		ctx := dircontext.Classify(file.Path)
		result.LargeMediaReviews = append(result.LargeMediaReviews, LargeMediaReview{
			StorageID: file.StorageID, Path: file.Path, Size: file.Size, Format: file.Format,
			Context: ctx, RelationCount: relationCounts[file.Path], Recommendation: RecommendationKeepReview,
			Evidence: mediaEvidence(file, ctx, relationCounts[file.Path]),
		})
		result.Summary.LargeMediaBytes += file.Size
		if relationCounts[file.Path] > 0 {
			result.Summary.LargeMediaWithRelations++
		}
		if ctx.BusinessAnchor != "" {
			result.Summary.LargeMediaWithAnchor++
		}
		if ctx.Role == domain.RoleProjectWork {
			result.Summary.LargeMediaProjectWork++
		}
		if ctx.Protected {
			result.Summary.LargeMediaProtected++
		}
		if file.Format.Codec == "" {
			result.Summary.LargeMediaMissingCodec++
		}
		if file.Format.Duration <= 0 {
			result.Summary.LargeMediaMissingDuration++
		}
	}
	for _, aggregate := range aggregates {
		result.MediaAggregates = append(result.MediaAggregates, *aggregate)
	}
	sort.Slice(result.MediaAggregates, func(i, j int) bool {
		if result.MediaAggregates[i].Bytes != result.MediaAggregates[j].Bytes {
			return result.MediaAggregates[i].Bytes > result.MediaAggregates[j].Bytes
		}
		if result.MediaAggregates[i].Format != result.MediaAggregates[j].Format {
			return result.MediaAggregates[i].Format < result.MediaAggregates[j].Format
		}
		return result.MediaAggregates[i].Codec < result.MediaAggregates[j].Codec
	})
	sort.Slice(result.LargeMediaReviews, func(i, j int) bool {
		if result.LargeMediaReviews[i].Size != result.LargeMediaReviews[j].Size {
			return result.LargeMediaReviews[i].Size > result.LargeMediaReviews[j].Size
		}
		return result.LargeMediaReviews[i].Path < result.LargeMediaReviews[j].Path
	})
	result.Summary.LargeMediaFiles = len(result.LargeMediaReviews)
	return result
}

func classifyZeroByte(file domain.FileInstance, ctx domain.DirectoryContext) (string, []string) {
	name := strings.ToLower(filepath.Base(file.Path))
	ext := strings.ToLower(filepath.Ext(name))
	if name == ".gitkeep" || name == ".keep" || name == ".nomedia" || strings.Contains(name, "placeholder") || strings.Contains(name, "占位") {
		return "placeholder_marker", []string{"文件名显式表示目录占位或应用标记", "占位文件可能维持目录/应用语义，默认保留"}
	}
	if ext == ".tmp" || ext == ".part" || ext == ".crdownload" || containsAny(name, "failed", "error", "失败", "错误") {
		return "incomplete_or_failed_output", []string{"文件名或扩展名命中不完整/失败产物特征", "需先确认对应任务和成功输出，不自动删除"}
	}
	if ctx.Role == domain.RoleTemporary || ctx.Role == domain.RoleCache || containsAny(name, "export", "render", "output", "导出", "渲染", "输出") {
		return "potential_transient_artifact", []string{"目录职责或文件名表明可能为临时/导出产物", "仅是潜在低职责候选，仍须人工复核"}
	}
	return "unexplained_empty_file", []string{"无足够证据区分占位符、失败导出或空文档", "保护规则优先，默认保留并人工复核"}
}

func containsAny(value string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func isMedia(info domain.FormatInfo) bool {
	return info.Category == domain.CategoryAudio || info.Category == domain.CategoryVideo
}

func mediaEvidence(file domain.FileInstance, ctx domain.DirectoryContext, relationCount int) []string {
	evidence := []string{
		"大容量媒体必须结合编码、时长、尺寸、版本和项目职责复核",
		"不得仅根据文件大小或扩展名决定压缩、迁移或删除",
	}
	if file.Format.Codec == "" {
		evidence = append(evidence, "编码未识别，保留复核")
	}
	if file.Format.Duration <= 0 {
		evidence = append(evidence, "时长缺失，不做容量效率判断")
	}
	if ctx.Protected {
		evidence = append(evidence, "所在目录受保护")
	}
	if relationCount > 0 {
		evidence = append(evidence, "存在版本、派生或侧车关系，禁止脱离项目语境处理")
	}
	return evidence
}
