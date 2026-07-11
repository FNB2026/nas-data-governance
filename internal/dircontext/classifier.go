package dircontext

import (
	"path/filepath"
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

// Classify converts a file's parent path into a conservative, explainable context.
func Classify(path string) domain.DirectoryContext {
	segments := strings.FieldsFunc(strings.ToLower(filepath.ToSlash(filepath.Dir(path))), func(r rune) bool { return r == '/' || r == '\\' })
	for _, term := range sensitiveTerms {
		if matches(segments, term) {
			return domain.DirectoryContext{Role: domain.RoleSensitive, AuthorityLevel: 100, PrivacyLevel: "high", Protected: true, MatchedTerms: []string{term}}
		}
	}
	for _, candidate := range roleSignals {
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
