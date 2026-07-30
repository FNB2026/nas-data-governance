package format

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func writeBytes(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDetectJPEG(t *testing.T) {
	path := writeBytes(t, "test.jpg", []byte("\xFF\xD8\xFF\xE0\x00\x10JFIF"))
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "jpeg" || info.Category != domain.CategoryImage {
		t.Fatalf("got %#v", info)
	}
}

func TestDetectPNG(t *testing.T) {
	path := writeBytes(t, "test.png", append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...))
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "png" {
		t.Fatalf("got %s", info.Format)
	}
}

func TestDetectGIF(t *testing.T) {
	for _, sig := range []string{"GIF87a", "GIF89a"} {
		path := writeBytes(t, "test.gif", append([]byte(sig), make([]byte, 10)...))
		info, err := Detect(path)
		if err != nil {
			t.Fatalf("detect %s: %v", sig, err)
		}
		if info.Format != "gif" {
			t.Fatalf("got %s for %s", info.Format, sig)
		}
	}
}

func TestDetectBMP(t *testing.T) {
	path := writeBytes(t, "test.bmp", append([]byte("BM"), make([]byte, 30)...))
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "bmp" {
		t.Fatalf("got %s", info.Format)
	}
}

func TestDetectTIFF(t *testing.T) {
	for _, sig := range [][]byte{{0x49, 0x49, 0x2A, 0x00}, {0x4D, 0x4D, 0x00, 0x2A}} {
		path := writeBytes(t, "test.tiff", append(sig, make([]byte, 20)...))
		info, err := Detect(path)
		if err != nil {
			t.Fatalf("detect: %v", err)
		}
		if info.Format != "tiff" {
			t.Fatalf("got %s", info.Format)
		}
	}
}

func TestDetectPDF(t *testing.T) {
	path := writeBytes(t, "test.pdf", []byte("%PDF-1.7\nrest of file"))
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "pdf" || info.Category != domain.CategoryDocument {
		t.Fatalf("got %#v", info)
	}
}

func TestDetectMP4(t *testing.T) {
	// ftyp box at offset 4
	header := make([]byte, 16)
	copy(header[4:], []byte("ftypisom"))
	path := writeBytes(t, "test.mp4", header)
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "mp4" || info.Category != domain.CategoryVideo {
		t.Fatalf("got %#v", info)
	}
}

func TestDetectMOV(t *testing.T) {
	header := make([]byte, 16)
	copy(header[4:], []byte("ftypqt  "))
	path := writeBytes(t, "test.mov", header)
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "mov" {
		t.Fatalf("got %s", info.Format)
	}
}

func TestDetectMKV(t *testing.T) {
	path := writeBytes(t, "test.mkv", append([]byte{0x1A, 0x45, 0xDF, 0xA3}, make([]byte, 20)...))
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "mkv" {
		t.Fatalf("got %s", info.Format)
	}
}

func TestDetectProfessionalAndLegacyFormats(t *testing.T) {
	tests := []struct {
		name, format string
		category     domain.FormatCategory
		data         []byte
	}{
		{"sample.mxf", "mxf", domain.CategoryVideo, append([]byte{0x06, 0x0e, 0x2b, 0x34}, make([]byte, 24)...)},
		{"sample.dpx", "dpx", domain.CategoryImage, append([]byte("XPDS"), make([]byte, 24)...)},
		{"sample.eps", "eps", domain.CategoryImage, []byte("%!PS-Adobe-3.0 EPSF-3.0")},
		{"font.ttf", "ttf", domain.CategoryOther, append([]byte{0, 1, 0, 0}, make([]byte, 24)...)},
		{"catalog.db", "sqlite", domain.CategoryOther, []byte("SQLite format 3\x00more")},
		{"model.step", "step", domain.CategoryOther, []byte("ISO-10303-21;\nHEADER;")},
		{"page.html", "html", domain.CategoryCode, []byte("<!doctype html><html>")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info, err := Detect(writeBytes(t, test.name, test.data))
			if err != nil || info.Format != test.format || info.Category != test.category {
				t.Fatalf("got %#v err=%v", info, err)
			}
		})
	}
}

func TestDetectTransportStreams(t *testing.T) {
	for _, offset := range []int{0, 4} {
		packet := 188
		if offset == 4 {
			packet = 192
		}
		header := make([]byte, 512)
		header[offset], header[offset+packet] = 0x47, 0x47
		info, err := Detect(writeBytes(t, "sample.mts", header))
		if err != nil || info.Format != "mpeg-ts" || info.Category != domain.CategoryVideo {
			t.Fatalf("offset %d: got %#v err=%v", offset, info, err)
		}
	}
}

func TestDetectAACAndMPEGAudio(t *testing.T) {
	for _, test := range []struct {
		header []byte
		want   string
	}{{[]byte{0xff, 0xf1, 0x4c, 0x80}, "aac"}, {[]byte{0xff, 0xfa, 0x93, 0x64}, "mp3"}} {
		info, err := Detect(writeBytes(t, "audio.bin", test.header))
		if err != nil || info.Format != test.want {
			t.Fatalf("got %#v err=%v, want %s", info, err, test.want)
		}
	}
}

func TestAnalyzeProtectsProfessionalProjectSources(t *testing.T) {
	for _, test := range []struct{ name, want string }{
		{"project.aep", "after-effects-project"},
		{"timeline.prproj", "premiere-project"},
		{"catalog.lrcat", "lightroom-catalog"},
		{"model.step", "step"},
	} {
		info, err := Analyze(writeBytes(t, test.name, []byte("extension-governed project source")))
		if err != nil || info.Format != test.want || info.Role != domain.FormatRoleProjectSource || !info.Protected {
			t.Fatalf("%s: got %#v err=%v", test.name, info, err)
		}
	}
}

func TestDetectMP3(t *testing.T) {
	for _, sig := range [][]byte{{0xFF, 0xFB}, {0xFF, 0xF3}, {0xFF, 0xF2}} {
		path := writeBytes(t, "test.mp3", append(sig, make([]byte, 20)...))
		info, err := Detect(path)
		if err != nil {
			t.Fatalf("detect %x: %v", sig, err)
		}
		if info.Format != "mp3" {
			t.Fatalf("got %s for %x", info.Format, sig)
		}
	}
	// ID3 tag
	path := writeBytes(t, "test_id3.mp3", append([]byte("ID3\x03\x00\x00\x00\x00\x00\x00"), make([]byte, 20)...))
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect ID3: %v", err)
	}
	if info.Format != "mp3" {
		t.Fatalf("got %s for ID3", info.Format)
	}
}

func TestDetectFLAC(t *testing.T) {
	path := writeBytes(t, "test.flac", append([]byte("fLaC"), make([]byte, 20)...))
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "flac" || info.Category != domain.CategoryAudio {
		t.Fatalf("got %#v", info)
	}
}

func TestDetectWAV(t *testing.T) {
	header := append([]byte("RIFF"), make([]byte, 4)...)
	header = append(header, []byte("WAVEfmt ")...)
	path := writeBytes(t, "test.wav", header)
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "wav" {
		t.Fatalf("got %s", info.Format)
	}
}

func TestDetectAIFF(t *testing.T) {
	header := append([]byte("FORM"), make([]byte, 4)...)
	header = append(header, []byte("AIFFCOMM")...)
	path := writeBytes(t, "test.aif", header)
	info, err := Analyze(path)
	if err != nil || info.Format != "aiff" || info.Category != domain.CategoryAudio {
		t.Fatalf("got %#v err=%v", info, err)
	}
}

func TestDetectPSDAsProtectedProjectSource(t *testing.T) {
	path := writeBytes(t, "project.psd", append([]byte("8BPS"), make([]byte, 20)...))
	info, err := Analyze(path)
	if err != nil || info.Format != "psd" || info.Role != domain.FormatRoleProjectSource || !info.Protected {
		t.Fatalf("got %#v err=%v", info, err)
	}
}

func TestDetectOLELegacyOffice(t *testing.T) {
	header := append([]byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"), make([]byte, 32)...)
	for name, want := range map[string]string{"old.doc": "doc", "old.xls": "xls"} {
		path := writeBytes(t, name, header)
		info, err := Analyze(path)
		if err != nil || info.Format != want || info.Category != domain.CategoryDocument {
			t.Fatalf("%s: got %#v err=%v", name, info, err)
		}
	}
}

func TestAnalyzeClassifiesSidecarWithoutMagic(t *testing.T) {
	path := writeBytes(t, "audio.wav.peak", []byte("proprietary cache"))
	info, err := Analyze(path)
	if err != nil || info.Role != domain.FormatRoleRegenerableCache || !info.Protected || !info.Regenerable {
		t.Fatalf("got %#v err=%v", info, err)
	}
}

func TestAnalyzeRejectsSidecarSymlink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "real.peak")
	if err := os.WriteFile(target, []byte("cache"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "linked.peak")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Analyze(link); err == nil {
		t.Fatal("sidecar symlink must be rejected")
	}
}

func TestAnalyzeRejectsOrdinarySymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.pdf")
	if err := os.WriteFile(target, []byte("%PDF-1.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.pdf")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Analyze(link); err == nil || !strings.Contains(err.Error(), "symlink rejected") {
		t.Fatalf("expected ordinary symlink rejection, got %v", err)
	}
}

func TestDetectZIP(t *testing.T) {
	// Minimal ZIP: PK\x03\x04
	path := writeBytes(t, "test.zip", []byte("PK\x03\x04\x00\x00rest"))
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "zip" || info.Category != domain.CategoryArchive {
		t.Fatalf("got %#v", info)
	}
}

func TestDetectGzip(t *testing.T) {
	path := writeBytes(t, "test.gz", append([]byte{0x1f, 0x8b, 0x08}, make([]byte, 20)...))
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "gzip" {
		t.Fatalf("got %s", info.Format)
	}
}

func TestDetect7z(t *testing.T) {
	path := writeBytes(t, "test.7z", append([]byte("7z\xBC\xAF\x27\x1C"), make([]byte, 20)...))
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "7z" {
		t.Fatalf("got %s", info.Format)
	}
}

func TestDetectUnrecognized(t *testing.T) {
	path := writeBytes(t, "unknown.bin", []byte("random bytes that match nothing"))
	_, err := Detect(path)
	if err != ErrUnrecognized {
		t.Fatalf("expected ErrUnrecognized, got %v", err)
	}
}

func TestDetectEmptyFile(t *testing.T) {
	path := writeBytes(t, "empty.bin", []byte{})
	_, err := Detect(path)
	if err != ErrUnrecognized {
		t.Fatalf("expected ErrUnrecognized for empty file, got %v", err)
	}
}

// --- Metadata extraction tests ---

func TestExtractPNGDimensions(t *testing.T) {
	// Minimal PNG: signature + IHDR chunk with 1920x1080
	png := []byte("\x89PNG\r\n\x1a\n")
	// IHDR length (13) + type + data + CRC
	png = append(png, 0x00, 0x00, 0x00, 0x0D) // length
	png = append(png, []byte("IHDR")...)
	// width=1920 (BE), height=1080 (BE), bitdepth=8, colortype=2, etc.
	png = append(png, 0x00, 0x00, 0x07, 0x80) // 1920
	png = append(png, 0x00, 0x00, 0x04, 0x38) // 1080
	png = append(png, 0x08, 0x02, 0x00, 0x00, 0x00)
	png = append(png, make([]byte, 4)...) // CRC
	path := writeBytes(t, "test.png", png)
	info, _ := Detect(path)
	info = ExtractMetadata(path, info)
	if info.Width != 1920 || info.Height != 1080 {
		t.Fatalf("expected 1920x1080, got %dx%d", info.Width, info.Height)
	}
}

func TestExtractGIFDimensions(t *testing.T) {
	// GIF89a with 800x600
	gif := []byte("GIF89a")
	gif = append(gif, 0x20, 0x03) // 800 (LE)
	gif = append(gif, 0x58, 0x02) // 600 (LE)
	gif = append(gif, make([]byte, 4)...)
	path := writeBytes(t, "test.gif", gif)
	info, _ := Detect(path)
	info = ExtractMetadata(path, info)
	if info.Width != 800 || info.Height != 600 {
		t.Fatalf("expected 800x600, got %dx%d", info.Width, info.Height)
	}
}

func TestExtractBMPDimensions(t *testing.T) {
	// Minimal BMP: "BM" + 16 bytes + width(4B LE) + height(4B LE)
	bmp := []byte("BM")
	bmp = append(bmp, make([]byte, 16)...)    // file header
	bmp = append(bmp, 0x40, 0x01, 0x00, 0x00) // 320 (LE)
	bmp = append(bmp, 0xF0, 0x00, 0x00, 0x00) // 240 (LE)
	path := writeBytes(t, "test.bmp", bmp)
	info, _ := Detect(path)
	info = ExtractMetadata(path, info)
	if info.Width != 320 || info.Height != 240 {
		t.Fatalf("expected 320x240, got %dx%d", info.Width, info.Height)
	}
}

func TestExtractPDFPages(t *testing.T) {
	pdf := []byte("%PDF-1.7\n")
	pdf = append(pdf, []byte("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")...)
	pdf = append(pdf, []byte("2 0 obj\n<< /Type /Pages /Count 42 /Kids [] >>\nendobj\n")...)
	path := writeBytes(t, "test.pdf", pdf)
	info, _ := Detect(path)
	info = ExtractMetadata(path, info)
	if info.Pages != 42 {
		t.Fatalf("expected 42 pages, got %d", info.Pages)
	}
}

func TestExtractZipArchiveEntryCount(t *testing.T) {
	// Create a real ZIP with 3 entries.
	path := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write([]byte("content"))
	}
	w.Close()
	f.Close()
	info, _ := Detect(path)
	info = ExtractMetadata(path, info)
	if info.ArchiveEntryCount != 3 {
		t.Fatalf("expected 3 entries, got %d", info.ArchiveEntryCount)
	}
}

func TestExtractZipOOXMLDocx(t *testing.T) {
	// Create a ZIP with [Content_Types].xml indicating docx.
	path := filepath.Join(t.TempDir(), "test.docx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	ct, err := w.Create("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	ct.Write([]byte(`<?xml version="1.0"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`))
	w.Create("word/document.xml")
	w.Close()
	f.Close()
	info, _ := Detect(path)
	info = ExtractMetadata(path, info)
	if info.Format != "docx" || info.Category != domain.CategoryDocument {
		t.Fatalf("expected docx, got %#v", info)
	}
}

func TestExtractZipOOXMLXlsx(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.xlsx")
	f, _ := os.Create(path)
	w := zip.NewWriter(f)
	ct, _ := w.Create("[Content_Types].xml")
	ct.Write([]byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
</Types>`))
	w.Close()
	f.Close()
	info, _ := Detect(path)
	info = ExtractMetadata(path, info)
	if info.Format != "xlsx" {
		t.Fatalf("expected xlsx, got %s", info.Format)
	}
}

func TestZipMetadataCapsContentTypesExpansion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.docx")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	ct, err := w.Create("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ct.Write(bytes.Repeat([]byte("x"), contentTypesLimit+1)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	info := extractZipMetadata(path, domain.FormatInfo{Format: "zip", Category: domain.CategoryArchive})
	if info.Format != "zip" {
		t.Fatalf("oversized content types must not be parsed: %#v", info)
	}
}

func TestAnalyzeComposedForm(t *testing.T) {
	// Test that Analyze (Detect + ExtractMetadata) works end-to-end.
	png := []byte("\x89PNG\r\n\x1a\n")
	png = append(png, 0x00, 0x00, 0x00, 0x0D)
	png = append(png, []byte("IHDR")...)
	png = append(png, 0x00, 0x00, 0x07, 0x80) // 1920
	png = append(png, 0x00, 0x00, 0x04, 0x38) // 1080
	png = append(png, 0x08, 0x02, 0x00, 0x00, 0x00)
	png = append(png, make([]byte, 4)...)
	path := writeBytes(t, "test.png", png)
	info, err := Analyze(path)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if info.Format != "png" || info.Width != 1920 || info.Height != 1080 {
		t.Fatalf("got %#v", info)
	}
}

func TestAnalyzeUnrecognizedReturnsUnknown(t *testing.T) {
	path := writeBytes(t, "unknown.bin", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10})
	info, err := Analyze(path)
	if err != nil {
		t.Fatalf("analyze should not error on unrecognized: %v", err)
	}
	if info.Category != domain.CategoryUnknown {
		t.Fatalf("expected unknown category, got %s", info.Category)
	}
}

func TestAnalyzeRecognizesStructuredTextWithoutTrustingExtension(t *testing.T) {
	cases := []struct {
		name, data, want string
	}{
		{"payload.bin", `{"kind":"inventory","count":2}`, "json"},
		{"notes.dat", "plain UTF-8 notes with no binary control bytes", "text"},
		{"preview.eps", string([]byte{0xC5, 0xD0, 0xD3, 0xC6}) + "preview", "eps"},
	}
	for _, tc := range cases {
		path := writeBytes(t, tc.name, []byte(tc.data))
		got, err := Analyze(path)
		if err != nil || got.Format != tc.want {
			t.Fatalf("%s: got=%#v err=%v", tc.name, got, err)
		}
	}
}

func TestAnalyzeKeepsBinaryDataUnknown(t *testing.T) {
	path := writeBytes(t, "opaque.dat", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10})
	got, err := Analyze(path)
	if err != nil || got.Format != "unknown" {
		t.Fatalf("got=%#v err=%v", got, err)
	}
}

// detectFromBytes is tested directly for signature matching.
func TestDetectFromBytes(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"jpeg", []byte("\xFF\xD8\xFF"), "jpeg"},
		{"png", []byte("\x89PNG\r\n\x1a\n"), "png"},
		{"pdf", []byte("%PDF-1.7"), "pdf"},
		{"mp3_id3", append([]byte("ID3"), make([]byte, 10)...), "mp3"},
	}
	for _, tc := range cases {
		info, err := detectFromBytes(tc.data)
		if err != nil {
			t.Errorf("%s: unexpected error %v", tc.name, err)
			continue
		}
		if info.Format != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, info.Format, tc.want)
		}
	}
}

func TestIsZipBased(t *testing.T) {
	if !IsZipBased(domain.FormatInfo{Format: "zip"}) {
		t.Error("expected zip to be zip-based")
	}
	if IsZipBased(domain.FormatInfo{Format: "pdf"}) {
		t.Error("pdf should not be zip-based")
	}
}

// Verify that the detectFromBytes function handles short headers.
func TestDetectFromBytesShortHeader(t *testing.T) {
	short := []byte{0xFF}
	info, err := detectFromBytes(short)
	// Should not match any signature and return unrecognized.
	_ = info
	if err == nil {
		t.Error("expected error for short header")
	}
	// Ensure no panic on very short input.
	_, _ = detectFromBytes([]byte{})
	_ = bytes.NewReader // keep import alive
}

// ---- P1-6: 补齐缺失格式检测测试 ----

func TestDetectWebP(t *testing.T) {
	// RIFF header + file size + WEBP
	data := []byte("RIFF\x00\x00\x00\x00WEBP")
	path := writeBytes(t, "test.webp", data)
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "webp" || info.Category != domain.CategoryImage {
		t.Fatalf("got %#v", info)
	}
}

func TestDetectHEIC(t *testing.T) {
	// ftypheic signature at offset 4
	data := []byte("\x00\x00\x00\x20ftypheic")
	path := writeBytes(t, "test.heic", data)
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "heic" || info.Category != domain.CategoryImage {
		t.Fatalf("got %#v", info)
	}
}

func TestDetectAVI(t *testing.T) {
	// AVI is a RIFF subtype at offset 8.
	data := []byte("RIFF\x00\x00\x00\x00AVI ")
	path := writeBytes(t, "test.avi", data)
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "avi" || info.Category != domain.CategoryVideo {
		t.Fatalf("got %#v", info)
	}
}

func TestRIFFSubtypeRequiresRIFFContainer(t *testing.T) {
	for _, data := range [][]byte{
		[]byte("xxxx\x00\x00\x00\x00WAVE"),
		[]byte("xxxx\x00\x00\x00\x00WEBP"),
		[]byte("xxxx\x00\x00\x00\x00AVI "),
	} {
		if _, err := detectFromBytes(data); err != ErrUnrecognized {
			t.Fatalf("invalid RIFF container matched: %q err=%v", data, err)
		}
	}
}

func TestDetectM4ABeforeGenericISOBaseMedia(t *testing.T) {
	data := []byte("\x00\x00\x00\x20ftypM4A ")
	path := writeBytes(t, "test.m4a", data)
	info, err := Detect(path)
	if err != nil || info.Format != "m4a" || info.Category != domain.CategoryAudio {
		t.Fatalf("got %#v err=%v", info, err)
	}
}

func TestDetectTAR(t *testing.T) {
	// tar magic "ustar" at offset 257
	data := make([]byte, 300)
	copy(data[257:], []byte("ustar"))
	path := writeBytes(t, "test.tar", data)
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "tar" || info.Category != domain.CategoryArchive {
		t.Fatalf("got %#v", info)
	}
}

func TestDetectRAR(t *testing.T) {
	data := []byte("Rar!\x1a\x07\x00")
	path := writeBytes(t, "test.rar", data)
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "rar" || info.Category != domain.CategoryArchive {
		t.Fatalf("got %#v", info)
	}
}

func TestDetectBzip2(t *testing.T) {
	data := []byte("BZh91AY&SY")
	path := writeBytes(t, "test.bz2", data)
	info, err := Detect(path)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if info.Format != "bzip2" || info.Category != domain.CategoryArchive {
		t.Fatalf("got %#v", info)
	}
}

func TestDetectNonexistentFile(t *testing.T) {
	_, err := Detect("/tmp/nas-governance-nonexistent-test-file-99999.bin")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}
