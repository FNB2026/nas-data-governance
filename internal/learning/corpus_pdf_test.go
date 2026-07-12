package learning

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMinimalPDF builds a valid single-page PDF whose page content is the
// given ASCII text rendered with Helvetica. CJK is not tested here because
// Type1/Helvetica does not encode CJK; CJK extraction is covered by the
// TXT/MD/DOCX tests. This test verifies the PDF extraction link works.
func writeMinimalPDF(t *testing.T, path, text string) {
	t.Helper()
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%%PDF-1.4\n")
	offsets := make([]int64, 6)

	offsets[1] = int64(buf.Len())
	fmt.Fprintf(&buf, "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	offsets[2] = int64(buf.Len())
	fmt.Fprintf(&buf, "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	offsets[3] = int64(buf.Len())
	fmt.Fprintf(&buf, "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n")

	stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", escapePDFString(text))
	offsets[4] = int64(buf.Len())
	fmt.Fprintf(&buf, "4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(stream), stream)

	offsets[5] = int64(buf.Len())
	fmt.Fprintf(&buf, "5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	xrefPos := int64(buf.Len())
	fmt.Fprintf(&buf, "xref\n0 6\n")
	fmt.Fprintf(&buf, "0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefPos)

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
}

func escapePDFString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	return s
}

// TestLearnFromCorpus_PDFExtraction verifies that text is extracted from a
// minimal PDF and term frequencies are counted correctly.
func TestLearnFromCorpus_PDFExtraction(t *testing.T) {
	dir := t.TempDir()
	// Use ASCII terms; "deliverables" and "report" are not builtin terms.
	writeMinimalPDF(t, filepath.Join(dir, "spec.pdf"),
		"deliverables report deliverables report deliverables report")

	stats, err := LearnFromCorpus(context.Background(), CorpusOptions{
		Dir: dir, MinTermFreq: 2,
	})
	if err != nil {
		t.Fatalf("LearnFromCorpus: %v", err)
	}
	if stats.FilesRead != 1 {
		t.Errorf("FilesRead = %d, want 1", stats.FilesRead)
	}
	if stats.FilesSkipped != 0 {
		t.Errorf("FilesSkipped = %d, want 0", stats.FilesSkipped)
	}
	// "deliverables" should appear 3 times in the text.
	term, ok := findCorpusTerm(stats, "deliverables")
	if !ok {
		t.Fatalf("expected candidate 'deliverables', got: %+v", stats.Terms)
	}
	if term.Count < 3 {
		t.Errorf("'deliverables' count = %d, want >= 3", term.Count)
	}
	// "report" should also appear.
	if _, ok := findCorpusTerm(stats, "report"); !ok {
		t.Errorf("expected candidate 'report'")
	}
}

// TestLearnFromCorpus_PDFAndTxtMixed verifies PDF and TXT are both read in
// the same corpus run, confirming the multi-format pipeline.
func TestLearnFromCorpus_PDFAndTxtMixed(t *testing.T) {
	dir := t.TempDir()
	writeMinimalPDF(t, filepath.Join(dir, "a.pdf"),
		"deliverables deliverables deliverables")
	writeCorpusFile(t, dir, "b.txt",
		"deliverables deliverables deliverables")

	stats, err := LearnFromCorpus(context.Background(), CorpusOptions{
		Dir: dir, MinTermFreq: 2,
	})
	if err != nil {
		t.Fatalf("LearnFromCorpus: %v", err)
	}
	if stats.FilesRead != 2 {
		t.Errorf("FilesRead = %d, want 2 (pdf+txt)", stats.FilesRead)
	}
	term, ok := findCorpusTerm(stats, "deliverables")
	if !ok {
		t.Fatalf("expected 'deliverables' across both files")
	}
	// Should have contributions from both PDF and TXT.
	if term.Count < 6 {
		t.Errorf("'deliverables' count = %d, want >= 6 (3 pdf + 3 txt)", term.Count)
	}
}

// TestLearnFromCorpus_MalformedPDFSkipped verifies a non-PDF file with .pdf
// extension is counted as skipped, not crashing the run.
func TestLearnFromCorpus_MalformedPDFSkipped(t *testing.T) {
	dir := t.TempDir()
	// Write a non-PDF file with .pdf extension.
	writeCorpusFile(t, dir, "fake.pdf", "this is not a real pdf file")
	// Also write a valid TXT so the run is not entirely empty.
	writeCorpusFile(t, dir, "ok.txt", "deliverables deliverables deliverables")

	stats, err := LearnFromCorpus(context.Background(), CorpusOptions{
		Dir: dir, MinTermFreq: 2,
	})
	if err != nil {
		t.Fatalf("LearnFromCorpus: %v", err)
	}
	if stats.FilesRead != 1 {
		t.Errorf("FilesRead = %d, want 1 (only txt)", stats.FilesRead)
	}
	if stats.FilesSkipped != 1 {
		t.Errorf("FilesSkipped = %d, want 1 (malformed pdf)", stats.FilesSkipped)
	}
}
