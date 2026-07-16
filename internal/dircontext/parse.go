package dircontext

import (
	"strings"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// parseDirectorySignal parses a minimal YAML-like rule definition into a
// signal. L1 only supports directory_signal rules with:
//
//	match:
//	  segment_contains: "术语"
//	effect:
//	  role: formal_archive
//	  authority: 90
//
// This avoids a YAML dependency; the format is intentionally minimal. When
// L2 produces real learned rules, they will use this same format.
func parseDirectorySignal(definition string) (signal, bool) {
	lines := strings.Split(definition, "\n")
	var sig signal
	var inMatch, inEffect bool
	var term string
	var roleStr string
	var authority int
	var hasTerm, hasRole bool

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case line == "match:":
			inMatch, inEffect = true, false
		case line == "effect:":
			inMatch, inEffect = false, true
		case strings.HasPrefix(line, "segment_contains:"):
			val := strings.TrimSpace(strings.TrimPrefix(line, "segment_contains:"))
			term = unquote(val)
			if term != "" {
				hasTerm = true
			}
		case strings.HasPrefix(line, "role:"):
			val := strings.TrimSpace(strings.TrimPrefix(line, "role:"))
			roleStr = unquote(val)
			hasRole = true
		case strings.HasPrefix(line, "authority:"):
			val := strings.TrimSpace(strings.TrimPrefix(line, "authority:"))
			authority = parseInt(val)
		}
		_ = inMatch
		_ = inEffect
	}
	if !hasTerm || !hasRole {
		return sig, false
	}
	role := parseRole(roleStr)
	if role == domain.RoleUnknown {
		return sig, false
	}
	sig.role = role
	sig.authority = authority
	sig.terms = []string{term}
	return sig, true
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func parseInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func parseRole(s string) domain.DirectoryRole {
	switch strings.ToLower(s) {
	case "system", "system_application":
		return domain.RoleSystem
	case "backup":
		return domain.RoleBackup
	case "cache", "cache_derived":
		return domain.RoleCache
	case "temporary":
		return domain.RoleTemporary
	case "raw", "raw_source":
		return domain.RoleRaw
	case "formal_archive":
		return domain.RoleFormalArchive
	case "project", "project_work":
		return domain.RoleProjectWork
	case "unorganized":
		return domain.RoleUnorganized
	case "sensitive":
		return domain.RoleSensitive
	default:
		return domain.RoleUnknown
	}
}
