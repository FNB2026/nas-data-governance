package format

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"

	"nas-data-governance/internal/domain"
)

// contentTypesLimit prevents a malicious ZIP/OOXML entry from expanding into
// an unbounded allocation. Real [Content_Types].xml files are tiny; 1 MiB is
// deliberately generous.
const contentTypesLimit = 1 << 20

// ExtractMetadata enriches a FormatInfo with cheap, header-only metadata.
// It must be called after Detect. The path is opened again to extract
// format-specific fields; no media decoding or OCR is performed (K-006).
//
// Returns the original info unchanged for formats we don't have a metadata
// extractor for. Errors from metadata extraction are non-fatal: the caller
// still gets the detected format from Detect.
func ExtractMetadata(path string, info domain.FormatInfo) domain.FormatInfo {
	switch info.Format {
	case "png":
		return extractPNGDimensions(path, info)
	case "jpeg":
		return extractJPEGDimensions(path, info)
	case "gif":
		return extractGIFDimensions(path, info)
	case "bmp":
		return extractBMPDimensions(path, info)
	case "webp":
		return extractWebPDimensions(path, info)
	case "pdf":
		return extractPDFPages(path, info)
	case "zip":
		return extractZipMetadata(path, info)
	case "mp4", "mov", "m4v", "mkv":
		return extractVideoMetadata(path, info)
	case "mp3":
		return extractMP3Duration(path, info)
	default:
		return info
	}
}

// --- PNG ---
// PNG IHDR chunk: bytes 16-24 contain width (4B BE) and height (4B BE).
func extractPNGDimensions(path string, info domain.FormatInfo) domain.FormatInfo {
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	header := make([]byte, 24)
	if _, err := io.ReadFull(f, header); err != nil {
		return info
	}
	if len(header) < 24 {
		return info
	}
	info.Width = int(binary.BigEndian.Uint32(header[16:20]))
	info.Height = int(binary.BigEndian.Uint32(header[20:24]))
	return info
}

// --- JPEG ---
// JPEG dimensions require scanning for SOF0/SOF2 markers. We read up to
// 64KB to find the frame header.
func extractJPEGDimensions(path string, info domain.FormatInfo) domain.FormatInfo {
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	buf := make([]byte, 65536)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	for i := 0; i < len(buf)-9; i++ {
		if buf[i] != 0xFF {
			continue
		}
		marker := buf[i+1]
		// SOF0 (0xC0) through SOF15 (0xCF), excluding SOF4/SOF8/SOF12
		// which are reserved/unsupported.
		if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
			if i+9 > len(buf) {
				return info
			}
			info.Height = int(binary.BigEndian.Uint16(buf[i+5 : i+7]))
			info.Width = int(binary.BigEndian.Uint16(buf[i+7 : i+9]))
			return info
		}
	}
	return info
}

// --- GIF ---
// GIF logical screen descriptor: bytes 6-10 contain width (2B LE) and
// height (2B LE).
func extractGIFDimensions(path string, info domain.FormatInfo) domain.FormatInfo {
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	header := make([]byte, 10)
	if _, err := io.ReadFull(f, header); err != nil {
		return info
	}
	info.Width = int(binary.LittleEndian.Uint16(header[6:8]))
	info.Height = int(binary.LittleEndian.Uint16(header[8:10]))
	return info
}

// --- BMP ---
// BMP DIB header: bytes 18-22 contain width (4B LE) and height (4B LE).
func extractBMPDimensions(path string, info domain.FormatInfo) domain.FormatInfo {
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	header := make([]byte, 26)
	if _, err := io.ReadFull(f, header); err != nil {
		return info
	}
	if len(header) < 26 {
		return info
	}
	info.Width = int(int32(binary.LittleEndian.Uint32(header[18:22])))
	info.Height = int(int32(binary.LittleEndian.Uint32(header[22:26])))
	return info
}

// --- WebP ---
// WebP is RIFF-based. The VP8/VP8L/VP8X chunk carries dimensions.
func extractWebPDimensions(path string, info domain.FormatInfo) domain.FormatInfo {
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	buf := make([]byte, 30)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	if len(buf) < 30 {
		return info
	}
	// Check for VP8X (extended) first.
	if bytes.Equal(buf[12:16], []byte("VP8X")) {
		info.Width = int(binary.LittleEndian.Uint16(buf[24:26])) + 1
		info.Height = int(binary.LittleEndian.Uint16(buf[26:28])) + 1
		return info
	}
	// VP8 (lossy): dimensions at offset 26-29.
	if bytes.Equal(buf[12:16], []byte("VP8 ")) {
		if len(buf) >= 29 {
			info.Width = int(binary.LittleEndian.Uint16(buf[26:28]))
			info.Height = int(binary.LittleEndian.Uint16(buf[28:30]))
		}
		return info
	}
	// VP8L (lossless): dimensions encoded in VP8L header.
	if bytes.Equal(buf[12:16], []byte("VP8L")) {
		if len(buf) >= 25 {
			// VP8L signature byte at offset 20, then 14 bits width-1, 14 bits height-1
			bits := binary.LittleEndian.Uint32(buf[21:25])
			info.Width = int(bits&0x3FFF) + 1
			info.Height = int((bits>>14)&0x3FFF) + 1
		}
		return info
	}
	return info
}

// --- PDF ---
// PDF page count requires finding the "/Count" entry. We scan the first
// 1MB of the file for "/Type /Pages" followed by "/Count N". This is a
// best-effort heuristic; complex PDFs may need a full parser.
func extractPDFPages(path string, info domain.FormatInfo) domain.FormatInfo {
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	// Read up to 1MB — page count is usually declared early.
	buf := make([]byte, 1024*1024)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	// Look for "/Count " pattern, parse the following integer.
	idx := bytes.Index(buf, []byte("/Count "))
	if idx < 0 || idx+7 >= len(buf) {
		return info
	}
	pages := 0
	for i := idx + 7; i < len(buf) && i < idx+20; i++ {
		c := buf[i]
		if c < '0' || c > '9' {
			break
		}
		pages = pages*10 + int(c-'0')
	}
	if pages > 0 {
		info.Pages = pages
	}
	return info
}

// --- ZIP / Office ---
// ZIP-based files may be Office OOXML (docx, xlsx, pptx). We open the ZIP
// and look for [Content_Types].xml to determine the Office subtype. We also
// count entries for archive management decisions.
func extractZipMetadata(path string, info domain.FormatInfo) domain.FormatInfo {
	r, err := zip.OpenReader(path)
	if err != nil {
		return info
	}
	defer r.Close()
	info.ArchiveEntryCount = len(r.File)
	// Look for Office OOXML content types.
	for _, f := range r.File {
		if f.Name != "[Content_Types].xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return info
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, contentTypesLimit+1))
		if err != nil {
			return info
		}
		if len(data) > contentTypesLimit {
			return info
		}
		content := string(data)
		info = classifyOOXML(content, info)
		return info
	}
	return info
}

// classifyOOXML inspects [Content_Types].xml to determine the Office subtype.
func classifyOOXML(contentTypes string, info domain.FormatInfo) domain.FormatInfo {
	switch {
	case strings.Contains(contentTypes, "wordprocessingml"):
		info.Format = "docx"
		info.Category = domain.CategoryDocument
		info.MIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case strings.Contains(contentTypes, "spreadsheetml"):
		info.Format = "xlsx"
		info.Category = domain.CategoryDocument
		info.MIME = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case strings.Contains(contentTypes, "presentationml"):
		info.Format = "pptx"
		info.Category = domain.CategoryDocument
		info.MIME = "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	}
	return info
}

// --- Video (MP4/MOV/MKV) ---
// Video metadata extraction is limited to scanning the moov atom for
// duration. This is a best-effort heuristic; a full implementation would
// use a proper MP4 parser.
func extractVideoMetadata(path string, info domain.FormatInfo) domain.FormatInfo {
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	// Read up to 1MB looking for the mvhd atom.
	buf := make([]byte, 1024*1024)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	// Search for "mvhd" atom.
	idx := bytes.Index(buf, []byte("mvhd"))
	if idx < 0 || idx+24 > len(buf) {
		return info
	}
	// Version is at idx+4; we assume version 0 for simplicity.
	version := buf[idx+4]
	var timescale, duration uint32
	if version == 1 {
		// 64-bit duration: skip 8+8+8 bytes (version+flags+creation+modification)
		if idx+32 > len(buf) {
			return info
		}
		timescale = binary.BigEndian.Uint32(buf[idx+28 : idx+32])
		// 64-bit duration is at idx+32, but we only read 32 bits for safety.
	} else {
		if idx+24 > len(buf) {
			return info
		}
		timescale = binary.BigEndian.Uint32(buf[idx+12 : idx+16])
		duration = binary.BigEndian.Uint32(buf[idx+16 : idx+20])
	}
	if timescale > 0 && duration > 0 {
		info.Duration = float64(duration) / float64(timescale)
	}
	return info
}

// --- MP3 ---
// MP3 duration estimation from frame headers. This is a rough heuristic;
// a full implementation would parse every frame.
func extractMP3Duration(path string, info domain.FormatInfo) domain.FormatInfo {
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	// Read first 4 bytes after ID3 tag (if any).
	header := make([]byte, 10)
	if _, err := io.ReadFull(f, header); err != nil {
		return info
	}
	// Skip ID3v2 tag if present.
	if bytes.Equal(header[:3], []byte("ID3")) {
		size := int(header[6]&0x7F)<<21 | int(header[7]&0x7F)<<14 | int(header[8]&0x7F)<<7 | int(header[9]&0x7F)
		if _, err := f.Seek(int64(size+10), 0); err != nil {
			return info
		}
	} else {
		if _, err := f.Seek(0, 0); err != nil {
			return info
		}
	}
	// Read first MPEG frame.
	frame := make([]byte, 4)
	if _, err := io.ReadFull(f, frame); err != nil {
		return info
	}
	// MPEG version and layer from frame header.
	if frame[0] != 0xFF || (frame[1]&0xE0) != 0xE0 {
		return info
	}
	bitrateTable := [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	bitrateIdx := (frame[2] >> 4) & 0x0F
	bitrate := bitrateTable[bitrateIdx] * 1000
	if bitrate == 0 {
		return info
	}
	// Estimate duration from file size and bitrate.
	stat, err := f.Stat()
	if err != nil {
		return info
	}
	if bitrate > 0 {
		info.Duration = float64(stat.Size()*8) / float64(bitrate)
	}
	return info
}

// Analyze is the composed entry point: Detect + ExtractMetadata.
// It returns the full FormatInfo for a file. If detection fails, the
// returned info has Category=unknown.
func Analyze(path string) (domain.FormatInfo, error) {
	info, err := Detect(path)
	if err != nil && !errors.Is(err, ErrUnrecognized) {
		return info, err
	}
	// Even if unrecognized, return the info (with unknown category).
	// ExtractMetadata is skipped for unrecognized formats.
	if info.Format == "unknown" || info.Format == "" {
		return info, nil
	}
	info = ExtractMetadata(path, info)
	return info, nil
}
