package dircontext

import (
	"path/filepath"
	"regexp"
	"strings"

	"nas-data-governance/internal/domain"
)

type signal struct {
	role      domain.DirectoryRole
	authority int
	terms     []string
}

var roleSignals = []signal{
	{domain.RoleSystem, 100, []string{".git", "docker", "appdata", "database", "数据库", "系统", "虚拟机", "vm", "configuration", "配置"}},
	{domain.RoleBackup, 95, []string{"backup", "bak", "备份", "冷备", "镜像", "snapshot", "快照", "同步"}},
	{domain.RoleCache, 10, []string{"cache", "缓存", "thumbnail", "缩略图", "proxy", "代理", "transcode", "转码", "preview", "预览", "render", "渲染"}},
	{domain.RoleTemporary, 20, []string{"temp", "tmp", "临时", "中转", "待发送", "待上传", "待处理", "下载", "导出"}},
	{domain.RoleRaw, 90, []string{"raw", "原始", "原件", "源文件", "master", "source", "相机导出", "原始素材"}},
	{domain.RoleFormalArchive, 90, []string{"归档", "存档", "正式", "已完成", "已结项", "已完结", "最终交付", "档案", "archive", "final"}},
	{domain.RoleProjectWork, 65, []string{"项目", "project", "客户", "client", "需求", "交付", "工程", "work"}},
	{domain.RoleUnorganized, 45, []string{"未整理", "待整理", "杂项", "未知", "新建文件夹", "汇总", "收集", "inbox"}},
}

var sensitiveTerms = []string{"私密", "个人", "证件", "医疗", "病历", "财务", "银行", "密码", "合同", "法务", "人事", "身份证", "户口", "保险"}

// chainDepth caps how many ancestors we record, per white paper §5-7.
const chainDepth = 6

// projectCodePattern matches stable identifiers like PRJ-2024-001, P2024001,
// CLIENT-A. Used as a business anchor when no explicit project folder exists.
var projectCodePattern = regexp.MustCompile(`^[A-Z]{2,}[-_]\d{2,}[A-Z0-9-]*$`)

// Classify converts a file's parent path into a conservative, explainable context.
// It never touches the filesystem; only the path string is interpreted.
func Classify(path string) domain.DirectoryContext {
	segments := strings.FieldsFunc(strings.ToLower(filepath.ToSlash(filepath.Dir(path))), func(r rune) bool { return r == '/' || r == '\\' })

	// Original-case segments are needed for business anchor detection, which
	// is case-sensitive (project codes are uppercase by convention).
	originalSegments := strings.FieldsFunc(filepath.ToSlash(filepath.Dir(path)), func(r rune) bool { return r == '/' || r == '\\' })

	ctx := classifySegments(segments)
	ctx.ParentChain = buildChain(originalSegments)
	ctx.BranchPoint = detectBranchPoint(segments)
	ctx.BusinessAnchor = detectBusinessAnchor(originalSegments)
	return ctx
}

func classifySegments(segments []string) domain.DirectoryContext {
	for _, term := range sensitiveTerms {
		if matches(segments, term) {
			return domain.DirectoryContext{Role: domain.RoleSensitive, AuthorityLevel: 100, PrivacyLevel: "high", Protected: true, MatchedTerms: []string{term}}
		}
	}
	// activeSignals is globally priority-sorted. Builtin protection roles at
	// 90-100 always win; learned rules are capped at 60 per K-008.
	for _, candidate := range activeSignals() {
		matched := make([]string, 0)
		for _, term := range candidate.terms {
			if matches(segments, term) {
				matched = append(matched, term)
			}
		}
		if len(matched) > 0 {
			protected := candidate.role == domain.RoleSystem || candidate.role == domain.RoleBackup || candidate.role == domain.RoleRaw
			return domain.DirectoryContext{Role: candidate.role, AuthorityLevel: candidate.authority, PrivacyLevel: "normal", Protected: protected, MatchedTerms: matched}
		}
	}
	return domain.DirectoryContext{Role: domain.RoleUnknown, AuthorityLevel: 50, PrivacyLevel: "unknown"}
}

// buildChain records up to chainDepth ancestors, nearest first. Each entry
// carries its own role/authority so the planner can compare hierarchies.
func buildChain(originalSegments []string) []domain.ChainNode {
	if len(originalSegments) == 0 {
		return nil
	}
	start := 0
	if len(originalSegments) > chainDepth {
		start = len(originalSegments) - chainDepth
	}
	chain := make([]domain.ChainNode, 0, chainDepth)
	for i := len(originalSegments) - 1; i >= start; i-- {
		// Build the cumulative path for this ancestor.
		path := "/" + strings.Join(originalSegments[:i+1], "/")
		role, authority := roleForSegment(originalSegments[i])
		chain = append(chain, domain.ChainNode{
			Path:      path,
			Name:      originalSegments[i],
			Role:      role,
			Authority: authority,
		})
	}
	return chain
}

// roleForSegment classifies a single directory name. It mirrors
// classifySegments but operates on one token, used for chain nodes.
func roleForSegment(name string) (domain.DirectoryRole, int) {
	lower := strings.ToLower(name)
	segments := []string{lower}
	for _, term := range sensitiveTerms {
		if matches(segments, term) {
			return domain.RoleSensitive, 100
		}
	}
	for _, candidate := range activeSignals() {
		for _, term := range candidate.terms {
			if matches(segments, term) {
				return candidate.role, candidate.authority
			}
		}
	}
	return domain.RoleUnknown, 50
}

// detectBranchPoint returns the deepest ancestor whose single-segment role
// differs from the file's immediate parent role. Empty when every ancestor
// shares the immediate parent's role (uniform chain) or has no detectable
// role. Unknown roles are ignored so unstructured paths don't false-positive.
//
// This is single-file branch detection: it flags paths where the role
// transitions between ancestor and immediate parent. Cross-file divergence
// (where two duplicate paths split at a common ancestor) is computed by the
// planner using both files' ParentChain.
func detectBranchPoint(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	immediate := segments[len(segments)-1]
	parentRole, _ := roleForSegment(immediate)
	for i := len(segments) - 2; i >= 0; i-- {
		role, _ := roleForSegment(segments[i])
		if role == domain.RoleUnknown || role == parentRole {
			continue
		}
		return "/" + strings.Join(segments[:i+1], "/")
	}
	return ""
}

// detectBusinessAnchor looks for stable identifiers along the path: project
// codes (PRJ-2024-001) or year-based folders (2024). Returns the first match
// found, nearest to the file. Empty when none is found.
//
// Anchors are intentionally conservative: a false positive would silently
// route two same-content files to "different business purpose" review, which
// is safe (it just defers to human); a false negative would collapse them
// into the same group, which the white paper warns against.
func detectBusinessAnchor(originalSegments []string) string {
	for i := len(originalSegments) - 1; i >= 0; i-- {
		seg := originalSegments[i]
		if projectCodePattern.MatchString(seg) {
			return seg
		}
		if isYearSegment(seg) {
			return seg
		}
	}
	return ""
}

// isYearSegment recognizes 4-digit year folders (1990-2099) used in many
// archive layouts as a stable anchor.
func isYearSegment(s string) bool {
	if len(s) != 4 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	year := int(s[0]-'0')*1000 + int(s[1]-'0')*100 + int(s[2]-'0')*10 + int(s[3]-'0')
	return year >= 1990 && year <= 2099
}

// Chinese terms can be part of a descriptive directory name; ASCII terms must
// be a path token (or a token joined with '-'/'_') to avoid treating /tmp/...,
// templates, or unrelated filenames as semantic directory roles.
func matches(segments []string, term string) bool {
	for _, segment := range segments {
		if hasNonASCII(term) {
			if strings.Contains(segment, term) {
				return true
			}
			continue
		}
		if segment == term || strings.HasPrefix(segment, term+"-") || strings.HasPrefix(segment, term+"_") || strings.HasSuffix(segment, "-"+term) || strings.HasSuffix(segment, "_"+term) {
			return true
		}
	}
	return false
}

func hasNonASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return true
		}
	}
	return false
}
