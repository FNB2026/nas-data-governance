// Package formatdiag builds private, read-only review material from the
// project database. It never opens source files and never grants cleanup
// authority; paths are confined to the explicitly private report.
package formatdiag

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

type ReviewItem struct {
	StorageID     string    `json:"storage_id"`
	Path          string    `json:"path"`
	Size          int64     `json:"size"`
	ModifiedAt    time.Time `json:"modified_at"`
	QuickHash     string    `json:"quick_hash,omitempty"`
	ContentSHA256 string    `json:"content_sha256,omitempty"`
	Reason        string    `json:"reason"`
}

type ExtensionMismatch struct {
	StorageID string   `json:"storage_id"`
	Path      string   `json:"path"`
	Size      int64    `json:"size"`
	Extension string   `json:"extension"`
	Expected  []string `json:"expected"`
	Detected  string   `json:"detected"`
	Reason    string   `json:"reason"`
}

type MetadataGap struct {
	Format            string `json:"format"`
	Category          string `json:"category"`
	MissingDuration   int    `json:"missing_duration"`
	MissingDimensions int    `json:"missing_dimensions"`
	Bytes             int64  `json:"bytes"`
}

type Summary struct {
	Files                  int `json:"files"`
	FormatRows             int `json:"format_rows"`
	MissingFormatRows      int `json:"missing_format_rows"`
	LargeUnknown           int `json:"large_unknown"`
	ExtensionMismatches    int `json:"extension_mismatches"`
	FormatsWithMetadataGap int `json:"formats_with_metadata_gap"`
}

type Report struct {
	GeneratedAt         time.Time           `json:"generated_at"`
	LargeUnknownMinimum int64               `json:"large_unknown_minimum"`
	Summary             Summary             `json:"summary"`
	LargeUnknown        []ReviewItem        `json:"large_unknown"`
	ExtensionMismatches []ExtensionMismatch `json:"extension_mismatches"`
	MetadataGaps        []MetadataGap       `json:"metadata_gaps"`
	SafetyNotes         []string            `json:"safety_notes"`
}

type gapKey struct {
	format, category string
}

// Build joins already-indexed files with already-persisted format records.
// It performs no NAS access. largeUnknownMinimum is normally 100 MiB.
func Build(files []domain.FileInstance, formats []store.FormatRecord, largeUnknownMinimum int64, now time.Time) Report {
	byKey := make(map[string]domain.FormatInfo, len(formats))
	for _, record := range formats {
		byKey[record.StorageID+"\x00"+record.Path] = record.Info
	}
	report := Report{
		GeneratedAt: now.UTC(), LargeUnknownMinimum: largeUnknownMinimum,
		SafetyNotes: []string{
			"本报告只用于人工复核，不是删除、移动或隔离授权",
			"扩展名与文件头不一致可能是合法工程流程，必须结合项目语境复核",
		},
	}
	gaps := map[gapKey]*MetadataGap{}
	for _, file := range files {
		info, ok := byKey[file.StorageID+"\x00"+file.Path]
		if !ok {
			report.Summary.MissingFormatRows++
			continue
		}
		if (info.Format == "" || info.Format == "unknown" || info.Category == domain.CategoryUnknown) && file.Size >= largeUnknownMinimum {
			report.LargeUnknown = append(report.LargeUnknown, ReviewItem{
				StorageID: file.StorageID, Path: file.Path, Size: file.Size, ModifiedAt: file.ModifiedAt,
				QuickHash: file.QuickHash, ContentSHA256: file.ContentSHA256,
				Reason: "大型未识别文件：默认保留，需在私有复核环境确认封装、损坏或专有格式",
			})
		}
		if expected, ok := expectedFormats(filepath.Ext(file.Path)); ok && info.Format != "" && info.Format != "unknown" && !contains(expected, info.Format) {
			report.ExtensionMismatches = append(report.ExtensionMismatches, ExtensionMismatch{
				StorageID: file.StorageID, Path: file.Path, Size: file.Size,
				Extension: strings.ToLower(filepath.Ext(file.Path)), Expected: expected, Detected: info.Format,
				Reason: "扩展名与文件头不一致；仅记录证据，不自动重命名",
			})
		}
		if info.Category == domain.CategoryAudio || info.Category == domain.CategoryVideo {
			missingDuration := info.Duration <= 0
			missingDimensions := info.Category == domain.CategoryVideo && (info.Width <= 0 || info.Height <= 0)
			if missingDuration || missingDimensions {
				key := gapKey{format: info.Format, category: string(info.Category)}
				gap := gaps[key]
				if gap == nil {
					gap = &MetadataGap{Format: info.Format, Category: string(info.Category)}
					gaps[key] = gap
				}
				if missingDuration {
					gap.MissingDuration++
				}
				if missingDimensions {
					gap.MissingDimensions++
				}
				gap.Bytes += file.Size
			}
		}
	}
	sort.Slice(report.LargeUnknown, func(i, j int) bool {
		if report.LargeUnknown[i].Size != report.LargeUnknown[j].Size {
			return report.LargeUnknown[i].Size > report.LargeUnknown[j].Size
		}
		return report.LargeUnknown[i].Path < report.LargeUnknown[j].Path
	})
	sort.Slice(report.ExtensionMismatches, func(i, j int) bool {
		if report.ExtensionMismatches[i].Detected != report.ExtensionMismatches[j].Detected {
			return report.ExtensionMismatches[i].Detected < report.ExtensionMismatches[j].Detected
		}
		return report.ExtensionMismatches[i].Path < report.ExtensionMismatches[j].Path
	})
	for _, gap := range gaps {
		report.MetadataGaps = append(report.MetadataGaps, *gap)
	}
	sort.Slice(report.MetadataGaps, func(i, j int) bool { return report.MetadataGaps[i].Bytes > report.MetadataGaps[j].Bytes })
	report.Summary = Summary{
		Files: len(files), FormatRows: len(formats), MissingFormatRows: report.Summary.MissingFormatRows,
		LargeUnknown: len(report.LargeUnknown), ExtensionMismatches: len(report.ExtensionMismatches),
		FormatsWithMetadataGap: len(report.MetadataGaps),
	}
	return report
}

func expectedFormats(ext string) ([]string, bool) {
	expected, ok := map[string][]string{
		".wav": {"wav"}, ".wave": {"wav"}, ".aif": {"aiff"}, ".aiff": {"aiff"}, ".aifc": {"aiff"},
		".mp3": {"mp3"}, ".flac": {"flac"}, ".ogg": {"ogg"}, ".m4a": {"m4a", "mp4"},
		".mp4": {"mp4"}, ".mov": {"mov", "mp4"}, ".m4v": {"m4v", "mp4"}, ".mkv": {"mkv"},
		".avi": {"avi"}, ".mpg": {"mpeg"}, ".mpeg": {"mpeg"}, ".flv": {"flv"},
		".jpg": {"jpeg"}, ".jpeg": {"jpeg"}, ".png": {"png"}, ".gif": {"gif"}, ".bmp": {"bmp"},
		".tif": {"tiff"}, ".tiff": {"tiff"}, ".webp": {"webp"}, ".heic": {"heic"}, ".psd": {"psd"},
		".pdf": {"pdf"}, ".doc": {"doc"}, ".xls": {"xls"}, ".ppt": {"ppt"},
		".docx": {"docx"}, ".xlsx": {"xlsx"}, ".pptx": {"pptx"},
	}[strings.ToLower(ext)]
	return expected, ok
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
