package merge

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"nas-data-governance/internal/domain"
)

type DiagnosticSummary struct {
	Files                int   `json:"files"`
	Directories          int   `json:"directories"`
	SiblingParents       int   `json:"sibling_parents"`
	SiblingPairs         int64 `json:"sibling_pairs"`
	NameSimilarPairs     int   `json:"name_similar_pairs"`
	PositiveOverlapPairs int   `json:"positive_overlap_pairs"`
	OverlapAtLeast10     int   `json:"overlap_at_least_0_10"`
	OverlapAtLeast25     int   `json:"overlap_at_least_0_25"`
	OverlapAtLeast50     int   `json:"overlap_at_least_0_50"`
	Suggestions          int   `json:"suggestions"`
}

type PairReview struct {
	DirectoryA      string   `json:"directory_a"`
	DirectoryB      string   `json:"directory_b"`
	FilenameJaccard float64  `json:"filename_jaccard"`
	Gate            string   `json:"gate"`
	Evidence        []string `json:"evidence"`
}

type DiagnosticReport struct {
	GeneratedAt         time.Time         `json:"generated_at"`
	ExecutionAuthorized bool              `json:"execution_authorized"`
	SuggestionThreshold float64           `json:"suggestion_threshold"`
	Summary             DiagnosticSummary `json:"summary"`
	NameSimilarReviews  []PairReview      `json:"name_similar_reviews"`
	SafetyNotes         []string          `json:"safety_notes"`
}

// Diagnose explains every gate used by Suggest without creating plans or
// changing the conservative production threshold.
func Diagnose(files []domain.FileInstance, now time.Time) DiagnosticReport {
	report := DiagnosticReport{
		GeneratedAt: now.UTC(), ExecutionAuthorized: false, SuggestionThreshold: nameOverlapThreshold,
		SafetyNotes: []string{
			"诊断只读取索引，不访问或修改源文件",
			"低于阈值的近似目录对仅供人工复核，不生成合并计划",
			"目录职责和存储用途必须在任何合并决策前单独确认",
		},
	}
	report.Summary.Files = len(files)
	dirNames := make(map[string]map[string]struct{})
	for _, file := range files {
		dir := filepath.ToSlash(filepath.Dir(file.Path))
		name := file.Name
		if name == "" {
			name = filepath.Base(file.Path)
		}
		if dirNames[dir] == nil {
			dirNames[dir] = make(map[string]struct{})
		}
		dirNames[dir][strings.ToLower(name)] = struct{}{}
	}
	report.Summary.Directories = len(dirNames)
	siblings := make(map[string][]string)
	for dir := range dirNames {
		parent := filepath.ToSlash(filepath.Dir(dir))
		if parent != dir {
			siblings[parent] = append(siblings[parent], dir)
		}
	}
	for _, dirs := range siblings {
		if len(dirs) < 2 {
			continue
		}
		report.Summary.SiblingParents++
		report.Summary.SiblingPairs += int64(len(dirs)*(len(dirs)-1)) / 2
		sort.Strings(dirs)
		for i := 0; i < len(dirs); i++ {
			for j := i + 1; j < len(dirs); j++ {
				a, b := dirs[i], dirs[j]
				if !namesSimilar(a, b) {
					continue
				}
				overlap := jaccard(dirNames[a], dirNames[b])
				review := PairReview{
					DirectoryA: a, DirectoryB: b, FilenameJaccard: overlap,
					Evidence: []string{"同一父目录", "目录名经有限备份/副本后缀归一后相同", "文件名集合 Jaccard 仅作复核证据"},
				}
				report.Summary.NameSimilarPairs++
				if overlap > 0 {
					report.Summary.PositiveOverlapPairs++
				}
				switch {
				case overlap >= nameOverlapThreshold:
					review.Gate = "meets_current_review_threshold"
					report.Summary.OverlapAtLeast50++
					report.Summary.OverlapAtLeast25++
					report.Summary.OverlapAtLeast10++
				case overlap >= 0.25:
					review.Gate = "near_miss_0_25_to_0_50"
					report.Summary.OverlapAtLeast25++
					report.Summary.OverlapAtLeast10++
				case overlap >= 0.10:
					review.Gate = "near_miss_0_10_to_0_25"
					report.Summary.OverlapAtLeast10++
				case overlap > 0:
					review.Gate = "weak_overlap_below_0_10"
				default:
					review.Gate = "no_filename_overlap"
				}
				report.NameSimilarReviews = append(report.NameSimilarReviews, review)
			}
		}
	}
	report.Summary.Suggestions = report.Summary.OverlapAtLeast50
	sort.Slice(report.NameSimilarReviews, func(i, j int) bool {
		if report.NameSimilarReviews[i].FilenameJaccard != report.NameSimilarReviews[j].FilenameJaccard {
			return report.NameSimilarReviews[i].FilenameJaccard > report.NameSimilarReviews[j].FilenameJaccard
		}
		if report.NameSimilarReviews[i].DirectoryA != report.NameSimilarReviews[j].DirectoryA {
			return report.NameSimilarReviews[i].DirectoryA < report.NameSimilarReviews[j].DirectoryA
		}
		return report.NameSimilarReviews[i].DirectoryB < report.NameSimilarReviews[j].DirectoryB
	})
	return report
}
