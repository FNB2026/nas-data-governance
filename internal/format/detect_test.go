package format

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"nas-data-governance/internal/domain"
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
	bmp = append(bmp, make([]byte, 16)...) // file header
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
	path := writeBytes(t, "unknown.bin", []byte("nothing matches this"))
	info, err := Analyze(path)
	if err != nil {
		t.Fatalf("analyze should not error on unrecognized: %v", err)
	}
	if info.Category != domain.CategoryUnknown {
		t.Fatalf("expected unknown category, got %s", info.Category)
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
