package learning

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"nas-data-governance/internal/dircontext"
	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/store"
)

// CorpusStats is the output of one corpus learning run. It holds candidate
// terms extracted from user-provided trusted documents, never the document
// text itself (K-009: learning artifacts retain only extracted terms and
// role suggestions, not source content or file paths).
type CorpusStats struct {
	// Terms lists candidate directory-role terms extracted from the corpus,
	// sorted by frequency then lexicographically.
	Terms []CorpusTerm `json:"terms"`
	// SensitiveCandidates are terms the user may want to mark sensitive
	// (e.g., industry-specific privacy terms like "病历"). They are not
	// auto-applied; the user confirms via `rules approve`.
	SensitiveCandidates []CorpusTerm `json:"sensitive_candidates"`
	// FilesRead is the number of corpus files successfully parsed.
	FilesRead int `json:"files_read"`
	// FilesSkipped is the number of files that could not be parsed
	// (unsupported format, read error, oversized).
	FilesSkipped int `json:"files_skipped"`
}

// CorpusTerm is one extracted candidate term with its frequency and the
// documents it appeared in (by basename, not full path — K-009).
type CorpusTerm struct {
	Term   string `json:"term"`
	Count  int    `json:"count"`
	DocSources []string `json:"doc_sources,omitempty"`
	SuggestedRole domain.DirectoryRole `json:"suggested_role,omitempty"`
}

// CorpusOptions controls corpus learning behavior.
type CorpusOptions struct {
	// Dir is the trusted corpus directory. Required.
	Dir string
	// MaxFileBytes caps how much text is read from a single document.
	// Documents larger than this are truncated. Default 1 MiB.
	MaxFileBytes int64
	// MinTermFreq is the minimum frequency for a term to become a candidate.
	// Default 2.
	MinTermFreq int
	// MaxTermLen caps term rune length to avoid junk long tokens.
	// Default 16.
	MaxTermLen int
}

func (o *CorpusOptions) withDefaults() CorpusOptions {
	out := *o
	if out.MaxFileBytes == 0 {
		out.MaxFileBytes = 1 << 20 // 1 MiB
	}
	if out.MinTermFreq == 0 {
		out.MinTermFreq = 2
	}
	if out.MaxTermLen == 0 {
		out.MaxTermLen = 16
	}
	return out
}

// Supported corpus file extensions.
const (
	extTXT  = ".txt"
	extMD   = ".md"
	extDOCX = ".docx"
)

// maxDocSourcesPerTerm caps how many source basenames we record per term, to
// keep the CorpusStats payload small.
const maxDocSourcesPerTerm = 5

// docxMaxXMLBytes caps a single word/document.xml read size. Real documents
// are bounded; this is a safety net against malicious archives.
const docxMaxXMLBytes = 8 << 20 // 8 MiB

// LearnFromCorpus walks the trusted corpus directory, extracts text from
// supported documents (TXT/MD/DOCX), tokenizes, and produces candidate
// directory-role terms plus sensitive-term candidates.
//
// Privacy (K-009):
//   - Only the corpus directory is read; never the user's NAS data.
//   - Output retains only extracted terms and doc basenames, never full
//     paths or document content.
//   - The learning batch record stores no source content.
//
// Rule generation is separate: call GenerateCorpusDrafts to persist.
func LearnFromCorpus(ctx context.Context, opts CorpusOptions) (*CorpusStats, error) {
	opts = opts.withDefaults()
	if opts.Dir == "" {
		return nil, fmt.Errorf("learning: corpus dir is required")
	}

	entries, err := os.ReadDir(opts.Dir)
	if err != nil {
		return nil, fmt.Errorf("learning: read corpus dir: %w", err)
	}

	builtinRoles := dircontext.BuiltinTermRoles()
	// Also include L2-learned terms already in builtin (via MergeLearned) so
	// corpus learning does not duplicate them. For L3 we rely on BuiltinTermRoles
	// which covers builtin + sensitive; already-approved learned rules are
	// checked during GenerateCorpusDrafts via ListRules.

	stats := &CorpusStats{}
	termFreq := map[string]*CorpusTerm{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		path := filepath.Join(opts.Dir, name)

		text, ok, err := extractText(path, ext, opts.MaxFileBytes)
		if err != nil {
			stats.FilesSkipped++
			continue
		}
		if !ok {
			stats.FilesSkipped++
			continue
		}
		stats.FilesRead++

		for _, token := range tokenize(text, opts.MaxTermLen) {
			// Skip builtin-covered terms (they already classify correctly).
			if _, exists := builtinRoles[strings.ToLower(token)]; exists {
				continue
			}
			t := termFreq[token]
			if t == nil {
				t = &CorpusTerm{Term: token}
				termFreq[token] = t
			}
			t.Count++
			if len(t.DocSources) < maxDocSourcesPerTerm {
				t.DocSources = append(t.DocSources, name)
			}
		}
	}

	// Apply threshold and split into role candidates vs sensitive candidates.
	// Heuristic: terms containing any sensitive-indicating substring (医/证/密
	// etc.) are routed to SensitiveCandidates for user confirmation.
	for _, t := range termFreq {
		if t.Count < opts.MinTermFreq {
			continue
		}
		t.SuggestedRole = suggestCorpusRole(t.Term, builtinRoles)
		if looksSensitive(t.Term) {
			stats.SensitiveCandidates = append(stats.SensitiveCandidates, *t)
		} else {
			stats.Terms = append(stats.Terms, *t)
		}
	}
	sort.Slice(stats.Terms, func(i, j int) bool {
		if stats.Terms[i].Count != stats.Terms[j].Count {
			return stats.Terms[i].Count > stats.Terms[j].Count
		}
		return stats.Terms[i].Term < stats.Terms[j].Term
	})
	sort.Slice(stats.SensitiveCandidates, func(i, j int) bool {
		if stats.SensitiveCandidates[i].Count != stats.SensitiveCandidates[j].Count {
			return stats.SensitiveCandidates[i].Count > stats.SensitiveCandidates[j].Count
		}
		return stats.SensitiveCandidates[i].Term < stats.SensitiveCandidates[j].Term
	})
	return stats, nil
}

// extractText reads a corpus file and returns its text content. ok=false
// means the format is unsupported; err non-nil means a read failure.
func extractText(path, ext string, maxBytes int64) (string, bool, error) {
	switch ext {
	case extTXT, extMD:
		data, err := os.ReadFile(path)
		if err != nil {
			return "", false, err
		}
		if int64(len(data)) > maxBytes {
			data = data[:maxBytes]
		}
		return string(data), true, nil
	case extDOCX:
		text, err := extractDocxText(path, maxBytes)
		if err != nil {
			return "", false, err
		}
		return text, true, nil
	default:
		// PDF text extraction is intentionally deferred (requires a parser
		// dependency); other formats are unsupported in L3.
		return "", false, nil
	}
}

// extractDocxText opens the OOXML zip and concatenates text from
// word/document.xml <w:t> elements. It does not call OCR or any external
// service (K-006/K-009).
func extractDocxText(path string, maxBytes int64) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("open docx: %w", err)
	}
	defer r.Close()

	var sb strings.Builder
	for _, f := range r.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("open word/document.xml: %w", err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, docxMaxXMLBytes))
		rc.Close()
		if err != nil {
			return "", fmt.Errorf("read word/document.xml: %w", err)
		}
		sb.WriteString(extractWtText(string(data)))
		break
	}
	text := sb.String()
	if int64(len(text)) > maxBytes {
		text = text[:maxBytes]
	}
	return text, nil
}

// docxTExtractor pulls text out of OOXML <w:t> elements. We use a minimal
// pull parser instead of a full XML DOM to bound memory.
func extractWtText(xmlContent string) string {
	dec := xml.NewDecoder(strings.NewReader(xmlContent))
	var sb strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		// Local name "t" in namespace "w" — but the default decoder does not
		// resolve namespace prefixes without schema. We match on the local
		// name only, which is sufficient for our <w:t> case.
		if se.Name.Local != "t" {
			continue
		}
		var text string
		if err := dec.DecodeElement(&text, &se); err != nil {
			continue
		}
		sb.WriteString(text)
	}
	return sb.String()
}

// tokenize splits text into candidate terms. For CJK text we treat each
// run of CJK runes as a stream where 2-4 char n-grams are candidates; for
// Latin text we take alphanumeric runs of length >= 2 as tokens. This is a
// lightweight heuristic, not a proper segmenter — good enough to surface
// high-frequency directory-name candidates from corpus documents.
func tokenize(text string, maxLen int) []string {
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if utf8.RuneCountInString(s) > maxLen {
			return
		}
		out = append(out, s)
	}

	var cjkBuf strings.Builder
	flushCJK := func() {
		if cjkBuf.Len() == 0 {
			return
		}
		s := cjkBuf.String()
		cjkBuf.Reset()
		runes := []rune(s)
		// Generate 2-4 char n-grams from a CJK run.
		for n := 2; n <= 4; n++ {
			if len(runes) < n {
				continue
			}
			for i := 0; i+n <= len(runes); i++ {
				add(string(runes[i : i+n]))
			}
		}
	}

	var latinBuf strings.Builder
	flushLatin := func() {
		if latinBuf.Len() == 0 {
			return
		}
		add(latinBuf.String())
		latinBuf.Reset()
	}

	for _, r := range text {
		switch {
		case isCJK(r):
			flushLatin()
			cjkBuf.WriteRune(r)
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			latinBuf.WriteRune(r)
		default:
			flushLatin()
			flushCJK()
		}
	}
	flushLatin()
	flushCJK()
	return out
}

func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0xF900 && r <= 0xFAFF) // CJK Compatibility Ideographs
}

// suggestCorpusRole proposes a role for a corpus term. Since corpus terms
// come from industry documents (not co-occurrence data like L2), the
// suggestion is weak: terms containing archive-related substrings get
// RoleFormalArchive; project-related get RoleProjectWork; otherwise
// RoleUnorganized. The user confirms via `rules approve`.
func suggestCorpusRole(term string, builtinRoles map[string]domain.DirectoryRole) domain.DirectoryRole {
	lower := strings.ToLower(term)
	if containsAny(lower, "归档", "存档", "archive", "final", "竣工", "结项", "交付") {
		return domain.RoleFormalArchive
	}
	if containsAny(lower, "项目", "project", "工程", "客户", "client") {
		return domain.RoleProjectWork
	}
	if containsAny(lower, "临时", "temp", "tmp", "草稿", "draft") {
		return domain.RoleTemporary
	}
	if containsAny(lower, "备份", "backup", "bak") {
		return domain.RoleBackup
	}
	if containsAny(lower, "原始", "raw", "源文件", "master") {
		return domain.RoleRaw
	}
	if containsAny(lower, "缓存", "cache", "缩略图", "thumbnail") {
		return domain.RoleCache
	}
	return domain.RoleUnorganized
}

// looksSensitive returns true for terms that look like industry-specific
// privacy markers. These become SensitiveCandidates rather than ordinary
// role candidates, because misclassifying them as a normal directory role
// could cause sensitive files to lose protection.
func looksSensitive(term string) bool {
	return containsAny(term,
		"病历", "处方", "诊断", "患者", "医疗",
		"身份证", "护照", "证件", "户口",
		"银行", "账号", "账户", "财务", "税务", "报表",
		"密码", "密钥", "凭证", "token", "secret",
		"合同", "法务", "人事", "薪资", "薪酬",
		"保险", "理赔",
	)
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// GenerateCorpusDrafts persists corpus-derived candidates as rule drafts.
// Role candidates become directory_signal drafts (priority <= 60, K-008).
// Sensitive candidates become directory_signal drafts targeting the
// sensitive role at priority 60 (still below builtin protection at 100,
// so builtin sensitive rules always win on ties — but the learned rule
// extends coverage to industry-specific terms builtin does not know).
//
// As with L2, rules that already left draft status are never overwritten.
func GenerateCorpusDrafts(ctx context.Context, st store.Store, stats *CorpusStats, batchID string) ([]domain.Rule, error) {
	if batchID == "" {
		return nil, fmt.Errorf("learning: batchID is required")
	}
	existing, err := st.ListRules(ctx, domain.RuleSourceLearned, "")
	if err != nil {
		return nil, fmt.Errorf("learning: list existing rules: %w", err)
	}
	skip := make(map[string]bool, len(existing))
	for _, r := range existing {
		if r.Status != domain.RuleDraft {
			skip[r.ID] = true
		}
	}

	rules := make([]domain.Rule, 0, len(stats.Terms)+len(stats.SensitiveCandidates))

	emit := func(term string, role domain.DirectoryRole) error {
		id := ruleIDForName(strings.ToLower(term))
		if skip[id] {
			return nil
		}
		authority := authorityForLearnedRole(role)
		if role == domain.RoleSensitive {
			// Sensitive learned rules sit at 60 — below builtin's 100 but
			// above ordinary learned rules so they win over working/temporary.
			authority = 60
		}
		r := domain.Rule{
			ID:         id,
			Version:    1,
			Priority:   authority,
			Enabled:    true,
			Source:     domain.RuleSourceLearned,
			BatchID:    batchID,
			Confidence: 0.4, // corpus terms are user-curated but role is a guess
			Status:     domain.RuleDraft,
			Definition: buildDefinition(strings.ToLower(term), role, authority),
		}
		if err := st.SaveRule(ctx, r); err != nil {
			return fmt.Errorf("learning: save corpus rule %s: %w", r.ID, err)
		}
		rules = append(rules, r)
		return nil
	}

	for i := range stats.Terms {
		t := &stats.Terms[i]
		if t.SuggestedRole == domain.RoleUnknown {
			continue
		}
		if err := emit(t.Term, t.SuggestedRole); err != nil {
			return nil, err
		}
	}
	for i := range stats.SensitiveCandidates {
		t := &stats.SensitiveCandidates[i]
		if err := emit(t.Term, domain.RoleSensitive); err != nil {
			return nil, err
		}
	}

	// Record the learning batch.
	now := time.Now().UTC()
	batch := store.LearningBatch{
		ID:          batchID,
		Source:      "corpus",
		StartedAt:   now,
		CompletedAt: &now,
		Status:      "completed",
		RuleCount:   len(rules),
	}
	if err := st.SaveLearningBatch(ctx, batch); err != nil {
		return nil, fmt.Errorf("learning: save corpus batch: %w", err)
	}
	return rules, nil
}
