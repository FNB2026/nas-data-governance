// Package format performs lightweight, read-only format analysis on files.
// Per knowledge card K-006 (progressive analysis), this package only reads
// file headers — it never decodes media content, runs OCR, or invokes AI.
// The goal is to identify the real format behind a file extension and
// extract cheap metadata that helps the planner distinguish "same content,
// different encoding" from true byte-level duplicates.
package format

import (
	"bytes"
	"errors"
	"io"
	"os"

	"nas-data-governance/internal/domain"
)

// headerSize is the maximum number of bytes read from a file for detection.
// Most magic bytes live in the first 512 bytes, which also aligns with the
// standard filesystem block size.
const headerSize = 512

// ErrUnrecognized is returned by Detect when no known format matches.
var ErrUnrecognized = errors.New("format: unrecognized")

// detector pairs a magic-byte signature with its format info.
type detector struct {
	// offset is the byte position where the signature begins.
	offset int
	// sig is the byte sequence to match at offset.
	sig []byte
	// info is the format info returned on match.
	info domain.FormatInfo
}

// detectors lists every known format signature. Order matters: more specific
// signatures must come before generic ones. For example, ZIP-based Office
// formats (docx, xlsx) must be checked before plain ZIP.
var detectors = []detector{
	// --- Images ---
	{0, []byte("\xFF\xD8\xFF"), domain.FormatInfo{Format: "jpeg", Category: domain.CategoryImage, MIME: "image/jpeg"}},
	{0, []byte("\x89PNG\r\n\x1a\n"), domain.FormatInfo{Format: "png", Category: domain.CategoryImage, MIME: "image/png"}},
	{0, []byte("GIF87a"), domain.FormatInfo{Format: "gif", Category: domain.CategoryImage, MIME: "image/gif"}},
	{0, []byte("GIF89a"), domain.FormatInfo{Format: "gif", Category: domain.CategoryImage, MIME: "image/gif"}},
	{0, []byte("BM"), domain.FormatInfo{Format: "bmp", Category: domain.CategoryImage, MIME: "image/bmp"}},
	{0, []byte("II*\x00"), domain.FormatInfo{Format: "tiff", Category: domain.CategoryImage, MIME: "image/tiff"}},
	{0, []byte("MM\x00*"), domain.FormatInfo{Format: "tiff", Category: domain.CategoryImage, MIME: "image/tiff"}},
	{0, []byte("8BPS"), domain.FormatInfo{Format: "psd", Category: domain.CategoryImage, MIME: "image/vnd.adobe.photoshop"}},
	// HEIC: ftyp box with heic brand
	{4, []byte("ftypheic"), domain.FormatInfo{Format: "heic", Category: domain.CategoryImage, MIME: "image/heic"}},
	{4, []byte("ftypheix"), domain.FormatInfo{Format: "heic", Category: domain.CategoryImage, MIME: "image/heic"}},

	// --- Video ---
	{4, []byte("ftypqt  "), domain.FormatInfo{Format: "mov", Category: domain.CategoryVideo, MIME: "video/quicktime"}},
	{4, []byte("ftypisom"), domain.FormatInfo{Format: "mp4", Category: domain.CategoryVideo, MIME: "video/mp4"}},
	{4, []byte("ftypmp42"), domain.FormatInfo{Format: "mp4", Category: domain.CategoryVideo, MIME: "video/mp4"}},
	{4, []byte("ftypM4V "), domain.FormatInfo{Format: "m4v", Category: domain.CategoryVideo, MIME: "video/x-m4v"}},
	{4, []byte("ftypM4A "), domain.FormatInfo{Format: "m4a", Category: domain.CategoryAudio, MIME: "audio/mp4"}},
	// Generic ftyp (ISO base media) — checked after specific brands.
	{4, []byte("ftyp"), domain.FormatInfo{Format: "mp4", Category: domain.CategoryVideo, MIME: "video/mp4"}},
	{0, []byte("\x00\x00\x01\xBA"), domain.FormatInfo{Format: "mpeg", Category: domain.CategoryVideo, MIME: "video/mpeg"}},
	{0, []byte("\x1A\x45\xDF\xA3"), domain.FormatInfo{Format: "mkv", Category: domain.CategoryVideo, MIME: "video/x-matroska"}},
	{0, []byte("FLV"), domain.FormatInfo{Format: "flv", Category: domain.CategoryVideo, MIME: "video/x-flv"}},

	// --- Audio ---
	{0, []byte("\xFF\xFB"), domain.FormatInfo{Format: "mp3", Category: domain.CategoryAudio, MIME: "audio/mpeg"}},
	{0, []byte("\xFF\xF3"), domain.FormatInfo{Format: "mp3", Category: domain.CategoryAudio, MIME: "audio/mpeg"}},
	{0, []byte("\xFF\xF2"), domain.FormatInfo{Format: "mp3", Category: domain.CategoryAudio, MIME: "audio/mpeg"}},
	{0, []byte("ID3"), domain.FormatInfo{Format: "mp3", Category: domain.CategoryAudio, MIME: "audio/mpeg"}},
	{0, []byte("fLaC"), domain.FormatInfo{Format: "flac", Category: domain.CategoryAudio, MIME: "audio/flac"}},
	{0, []byte("OggS"), domain.FormatInfo{Format: "ogg", Category: domain.CategoryAudio, MIME: "audio/ogg"}},
	// --- Documents ---
	{0, []byte("%PDF"), domain.FormatInfo{Format: "pdf", Category: domain.CategoryDocument, MIME: "application/pdf"}},
	{0, []byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1"), domain.FormatInfo{Format: "ole", Category: domain.CategoryDocument, MIME: "application/x-ole-storage"}},

	// --- Archives and Office ---
	// Office OOXML is ZIP-based; detect via internal [Content_Types].xml.
	// We check for PK signature here and refine in metadata extraction.
	{0, []byte("PK\x03\x04"), domain.FormatInfo{Format: "zip", Category: domain.CategoryArchive, MIME: "application/zip"}},
	{257, []byte("ustar"), domain.FormatInfo{Format: "tar", Category: domain.CategoryArchive, MIME: "application/x-tar"}},
	{0, []byte("7z\xBC\xAF\x27\x1C"), domain.FormatInfo{Format: "7z", Category: domain.CategoryArchive, MIME: "application/x-7z-compressed"}},
	{0, []byte("Rar!\x1a\x07"), domain.FormatInfo{Format: "rar", Category: domain.CategoryArchive, MIME: "application/x-rar"}},
	{0, []byte("\x1f\x8b"), domain.FormatInfo{Format: "gzip", Category: domain.CategoryArchive, MIME: "application/gzip"}},
	{0, []byte("BZh"), domain.FormatInfo{Format: "bzip2", Category: domain.CategoryArchive, MIME: "application/x-bzip2"}},
}

// Detect reads the first headerSize bytes from path and matches against
// known signatures. Returns ErrUnrecognized when no match is found.
//
// This function is the entry point for format analysis. Callers should run
// it as part of the progressive pipeline (K-006): size + quick hash first,
// then Detect, then ExtractMetadata if needed.
func Detect(path string) (domain.FormatInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return domain.FormatInfo{}, err
	}
	defer f.Close()
	header := make([]byte, headerSize)
	n, err := io.ReadFull(f, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return domain.FormatInfo{}, err
	}
	header = header[:n]
	return detectFromBytes(header)
}

// detectFromBytes matches header against known signatures. Exported via
// Detect; separated for testing.
func detectFromBytes(header []byte) (domain.FormatInfo, error) {
	if len(header) >= 12 && (bytes.Equal(header[:4], []byte("RIFF")) || bytes.Equal(header[:4], []byte("RF64"))) {
		switch string(header[8:12]) {
		case "WEBP":
			return domain.FormatInfo{Format: "webp", Category: domain.CategoryImage, MIME: "image/webp"}, nil
		case "WAVE":
			return domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio, MIME: "audio/wav"}, nil
		case "AVI ":
			return domain.FormatInfo{Format: "avi", Category: domain.CategoryVideo, MIME: "video/x-msvideo"}, nil
		}
	}
	if len(header) >= 12 && bytes.Equal(header[:4], []byte("FORM")) &&
		(bytes.Equal(header[8:12], []byte("AIFF")) || bytes.Equal(header[8:12], []byte("AIFC"))) {
		return domain.FormatInfo{Format: "aiff", Category: domain.CategoryAudio, MIME: "audio/aiff"}, nil
	}
	for _, d := range detectors {
		if d.offset+d.lenSig() > len(header) {
			continue
		}
		if bytes.Equal(header[d.offset:d.offset+len(d.sig)], d.sig) {
			info := d.info // copy so callers can mutate
			return info, nil
		}
	}
	return domain.FormatInfo{Format: "unknown", Category: domain.CategoryUnknown}, ErrUnrecognized
}

// lenSig avoids importing the reflect package just for len.
func (d detector) lenSig() int { return len(d.sig) }

// IsZipBased reports whether info is a ZIP-based format. Used by metadata
// extraction to decide whether to open the file as a ZIP archive and look
// for Office OOXML content types.
func IsZipBased(info domain.FormatInfo) bool {
	return info.Format == "zip"
}
