package learning

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nas-data-governance/internal/domain"
)

// writeCorpusFile writes a file into a corpus dir.
func writeCorpusFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// writeDocx creates a minimal valid docx zip with the given text in
// word/document.xml <w:t> elements.
func writeDocx(t *testing.T, dir, name, text string) {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	ct, err := w.Create("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	ct.Write([]byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="text/xml"/><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`))
	doc, err := w.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	// Split text into multiple <w:t> elements to verify concatenation.
	parts := strings.Split(text, "|")
	var sb strings.Builder
	sb.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, p := range parts {
		sb.WriteString(`<w:p><w:r><w:t>`)
		sb.WriteString(p)
		sb.WriteString(`</w:t></w:r></w:p>`)
	}
	sb.WriteString(`</w:body></w:document>`)
	doc.Write([]byte(sb.String()))
	if err := w.Close(); err != nil {
		t.Fatalf("close docx zip: %v", err)
	}
}

func findCorpusTerm(stats *CorpusStats, term string) (CorpusTerm, bool) {
	for _, t := range stats.Terms {
		if t.Term == term {
			return t, true
		}
	}
	for _, t := range stats.SensitiveCandidates {
		if t.Term == term {
			return t, true
		}
	}
	return CorpusTerm{}, false
}

// TestLearnFromCorpus_TxtMdDocx verifies text extraction from all three
// supported formats and term frequency counting.
func TestLearnFromCorpus_TxtMdDocx(t *testing.T) {
	dir := t.TempDir()
	writeCorpusFile(t, dir, "spec.txt",
		"竣工图 归档规范\n竣工图 交付物\n施工日志 项目文件")
	writeCorpusFile(t, dir, "notes.md",
		"竣工图 出现多次\n竣工图 再次出现")
	// docx with text split across <w:t> elements by "|"
	writeDocx(t, dir, "report.docx", "竣工图|归档|交付物")

	stats, err := LearnFromCorpus(context.Background(), CorpusOptions{
		Dir: dir, MinTermFreq: 2,
	})
	if err != nil {
		t.Fatalf("LearnFromCorpus: %v", err)
	}
	if stats.FilesRead != 3 {
		t.Errorf("FilesRead = %d, want 3", stats.FilesRead)
	}
	if stats.FilesSkipped != 0 {
		t.Errorf("FilesSkipped = %d, want 0", stats.FilesSkipped)
	}
	// "竣工图" (3 chars) appears in all 3 files → count >= 3.
	term, ok := findCorpusTerm(stats, "竣工图")
	if !ok {
		t.Fatalf("expected candidate '竣工图', got: %+v", stats.Terms)
	}
	if term.Count < 3 {
		t.Errorf("'竣工图' count = %d, want >= 3", term.Count)
	}
	if term.SuggestedRole != domain.RoleFormalArchive {
		t.Errorf("'竣工图' role = %s, want formal_archive", term.SuggestedRole)
	}
}

// TestLearnFromCorpus_SkipsBuiltinTerms verifies builtin-covered terms are
// not re-extracted as candidates.
func TestLearnFromCorpus_SkipsBuiltinTerms(t *testing.T) {
	dir := t.TempDir()
	writeCorpusFile(t, dir, "doc.txt",
		"归档 归档 归档\n备份 备份\n临时 临时")

	stats, err := LearnFromCorpus(context.Background(), CorpusOptions{
		Dir: dir, MinTermFreq: 2,
	})
	if err != nil {
		t.Fatalf("LearnFromCorpus: %v", err)
	}
	// "归档", "备份", "临时" are builtin terms → must not appear as candidates.
	if _, ok := findCorpusTerm(stats, "归档"); ok {
		t.Errorf("builtin '归档' must not be a candidate")
	}
	if _, ok := findCorpusTerm(stats, "备份"); ok {
		t.Errorf("builtin '备份' must not be a candidate")
	}
	if _, ok := findCorpusTerm(stats, "临时"); ok {
		t.Errorf("builtin '临时' must not be a candidate")
	}
}

// TestLearnFromCorpus_RoutesSensitiveCandidates verifies industry-specific
// privacy terms not already in builtin are routed to SensitiveCandidates.
// builtin sensitiveTerms already cover 病历/处方/etc., so we use terms like
// "诊断" and "患者" which are industry-sensitive but not builtin.
func TestLearnFromCorpus_RoutesSensitiveCandidates(t *testing.T) {
	dir := t.TempDir()
	writeCorpusFile(t, dir, "medical.txt",
		"诊断报告归档\n诊断报告管理\n患者信息表\n患者信息留存")

	stats, err := LearnFromCorpus(context.Background(), CorpusOptions{
		Dir: dir, MinTermFreq: 2,
	})
	if err != nil {
		t.Fatalf("LearnFromCorpus: %v", err)
	}
	// "诊断" appears in 2 lines → candidate, and looksSensitive matches "诊断".
	term, ok := findCorpusTerm(stats, "诊断")
	if !ok {
		t.Fatalf("expected sensitive candidate '诊断', got terms=%v sensitive=%v", stats.Terms, stats.SensitiveCandidates)
	}
	// It must be in SensitiveCandidates, not Terms.
	foundInTerms := false
	for _, c := range stats.Terms {
		if c.Term == "诊断" {
			foundInTerms = true
		}
	}
	if foundInTerms {
		t.Errorf("'诊断' must be in SensitiveCandidates, not Terms")
	}
	_ = term
}

// TestLearnFromCorpus_UnsupportedFormatSkipped verifies unsupported file
// types (e.g. .pdf, .png) are counted as skipped, not errored.
func TestLearnFromCorpus_UnsupportedFormatSkipped(t *testing.T) {
	dir := t.TempDir()
	writeCorpusFile(t, dir, "ok.txt", "竣工图 竣工图")
	writeCorpusFile(t, dir, "img.png", "\x89PNG fake bytes")
	writeCorpusFile(t, dir, "doc.pdf", "%PDF-1.4 fake")

	stats, err := LearnFromCorpus(context.Background(), CorpusOptions{
		Dir: dir, MinTermFreq: 2,
	})
	if err != nil {
		t.Fatalf("LearnFromCorpus: %v", err)
	}
	if stats.FilesRead != 1 {
		t.Errorf("FilesRead = %d, want 1", stats.FilesRead)
	}
	if stats.FilesSkipped != 2 {
		t.Errorf("FilesSkipped = %d, want 2", stats.FilesSkipped)
	}
}

// TestGenerateCorpusDrafts_PersistsRules verifies drafts are written for
// both ordinary role candidates and sensitive candidates, with priority
// capped at 60 (K-008).
func TestGenerateCorpusDrafts_PersistsRules(t *testing.T) {
	dir := t.TempDir()
	writeCorpusFile(t, dir, "spec.txt",
		"竣工图 竣工图 竣工图\n诊断 诊断 诊断")
	st := newLearnStore(t)

	stats, err := LearnFromCorpus(context.Background(), CorpusOptions{
		Dir: dir, MinTermFreq: 2,
	})
	if err != nil {
		t.Fatalf("LearnFromCorpus: %v", err)
	}
	rules, err := GenerateCorpusDrafts(context.Background(), st, stats, "corpus-batch-1")
	if err != nil {
		t.Fatalf("GenerateCorpusDrafts: %v", err)
	}
	if len(rules) < 2 {
		t.Fatalf("expected >= 2 drafts, got %d", len(rules))
	}
	// Check priority caps and sensitive role routing.
	var sawSensitive, sawArchive bool
	for _, r := range rules {
		if r.Priority > 60 {
			t.Errorf("rule %s priority %d > 60 (K-008)", r.ID, r.Priority)
		}
		if r.Source != domain.RuleSourceLearned {
			t.Errorf("rule %s source = %s, want learned", r.ID, r.Source)
		}
		if r.Status != domain.RuleDraft {
			t.Errorf("rule %s status = %s, want draft", r.ID, r.Status)
		}
		if strings.Contains(r.Definition, "sensitive") {
			sawSensitive = true
		}
		if strings.Contains(r.Definition, "formal_archive") {
			sawArchive = true
		}
	}
	if !sawSensitive {
		t.Errorf("expected at least one sensitive-role draft")
	}
	if !sawArchive {
		t.Errorf("expected at least one formal_archive draft")
	}

	// No rule definition may contain corpus file content beyond the term.
	for _, r := range rules {
		if strings.Contains(r.Definition, "竣工图 竣工图") {
			t.Errorf("rule definition leaked corpus content: %s", r.Definition)
		}
	}
}

// TestGenerateCorpusDrafts_PreservesApprovedRules verifies re-running corpus
// learning does not overwrite an already-approved rule (human decision kept).
func TestGenerateCorpusDrafts_PreservesApprovedRules(t *testing.T) {
	dir := t.TempDir()
	writeCorpusFile(t, dir, "spec.txt", "竣工图 竣工图 竣工图")
	st := newLearnStore(t)
	ctx := context.Background()

	// Seed an approved rule with the same ID GenerateCorpusDrafts would produce.
	approved := domain.Rule{
		ID:         "learned-dir-竣工图",
		Version:    1,
		Priority:   60,
		Enabled:    true,
		Source:     domain.RuleSourceLearned,
		BatchID:    "old",
		Confidence: 0.9,
		Status:     domain.RuleApproved,
		Definition: "match:\n  segment_contains: \"竣工图\"\neffect:\n  role: formal_archive\n  authority: 60",
	}
	if err := st.SaveRule(ctx, approved); err != nil {
		t.Fatal(err)
	}
	stats, err := LearnFromCorpus(ctx, CorpusOptions{Dir: dir, MinTermFreq: 2})
	if err != nil {
		t.Fatal(err)
	}
	rules, err := GenerateCorpusDrafts(ctx, st, stats, "new")
	if err != nil {
		t.Fatalf("GenerateCorpusDrafts: %v", err)
	}
	for _, r := range rules {
		if r.ID == approved.ID {
			t.Fatalf("approved rule %s was overwritten by corpus drafts", r.ID)
		}
	}
}

// TestLearnFromCorpus_NoRawContentInOutput verifies K-009: CorpusStats
// contains only terms and doc basenames, never full paths or file contents.
func TestLearnFromCorpus_NoRawContentInOutput(t *testing.T) {
	dir := t.TempDir()
	// Embed a unique sentinel string to verify it doesn't leak.
	writeCorpusFile(t, dir, "secret.txt",
		"SECRET_SENTINEL_DO_NOT_LEAK 竣工图 竣工图")
	stats, err := LearnFromCorpus(context.Background(), CorpusOptions{
		Dir: dir, MinTermFreq: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range stats.Terms {
		if strings.Contains(term.Term, "SECRET_SENTINEL") {
			t.Errorf("raw content leaked into term: %s", term.Term)
		}
		for _, src := range term.DocSources {
			if src != "secret.txt" {
				t.Errorf("doc source = %q, want basename only", src)
			}
		}
	}
}
