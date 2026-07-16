package format

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

const moovReadLimit = 4 << 20

func extractISOBaseMediaMetadata(path string, info domain.FormatInfo) domain.FormatInfo {
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return info
	}
	buf := readMoovPrefix(f, stat.Size())
	if len(buf) == 0 {
		buf = make([]byte, minInt64(stat.Size(), 1<<20))
		n, _ := f.ReadAt(buf, 0)
		buf = buf[:n]
	}
	info = parseMovieHeader(buf, info)
	info = parseTrackHeaders(buf, info)
	info = parseSampleDescription(buf, info)
	return info
}

func readMoovPrefix(f *os.File, fileSize int64) []byte {
	var offset int64
	header := make([]byte, 16)
	for atoms := 0; offset+8 <= fileSize && atoms < 1024; atoms++ {
		n, err := f.ReadAt(header[:8], offset)
		if err != nil && err != io.EOF || n < 8 {
			return nil
		}
		size := int64(binary.BigEndian.Uint32(header[:4]))
		headerSize := int64(8)
		if size == 1 {
			n, err = f.ReadAt(header[8:16], offset+8)
			if err != nil && err != io.EOF || n < 8 {
				return nil
			}
			size = int64(binary.BigEndian.Uint64(header[8:16]))
			headerSize = 16
		} else if size == 0 {
			size = fileSize - offset
		}
		if size < headerSize || size > fileSize-offset {
			return nil
		}
		if string(header[4:8]) == "moov" {
			readSize := minInt64(size-headerSize, moovReadLimit)
			buf := make([]byte, int(readSize))
			n, _ := f.ReadAt(buf, offset+headerSize)
			return buf[:n]
		}
		offset += size
	}
	return nil
}

func parseMovieHeader(buf []byte, info domain.FormatInfo) domain.FormatInfo {
	idx := bytes.Index(buf, []byte("mvhd"))
	if idx < 4 || idx+24 > len(buf) {
		return info
	}
	version := buf[idx+4]
	var timescale uint32
	var duration uint64
	if version == 1 {
		if idx+36 > len(buf) {
			return info
		}
		timescale = binary.BigEndian.Uint32(buf[idx+24 : idx+28])
		duration = binary.BigEndian.Uint64(buf[idx+28 : idx+36])
	} else {
		timescale = binary.BigEndian.Uint32(buf[idx+16 : idx+20])
		duration = uint64(binary.BigEndian.Uint32(buf[idx+20 : idx+24]))
	}
	if timescale > 0 && duration > 0 {
		info.Duration = float64(duration) / float64(timescale)
	}
	return info
}

func parseTrackHeaders(buf []byte, info domain.FormatInfo) domain.FormatInfo {
	for search := 0; search+4 < len(buf); {
		rel := bytes.Index(buf[search:], []byte("tkhd"))
		if rel < 0 {
			break
		}
		idx := search + rel
		if idx >= 4 {
			size := int(binary.BigEndian.Uint32(buf[idx-4 : idx]))
			end := idx - 4 + size
			if size >= 16 && end <= len(buf) {
				width := int(binary.BigEndian.Uint32(buf[end-8:end-4]) >> 16)
				height := int(binary.BigEndian.Uint32(buf[end-4:end]) >> 16)
				if width > info.Width && height > info.Height {
					info.Width, info.Height = width, height
				}
				search = end
				continue
			}
		}
		search = idx + 4
	}
	return info
}

func parseSampleDescription(buf []byte, info domain.FormatInfo) domain.FormatInfo {
	for search := 0; search+20 <= len(buf); {
		rel := bytes.Index(buf[search:], []byte("stsd"))
		if rel < 0 {
			break
		}
		idx := search + rel
		if idx+20 <= len(buf) {
			if codec := videoCodec(string(buf[idx+16 : idx+20])); codec != "" {
				info.Codec = codec
				return info
			}
		}
		search = idx + 4
	}
	return info
}

func videoCodec(code string) string {
	switch code {
	case "avc1", "avc3":
		return "h264"
	case "hvc1", "hev1":
		return "hevc"
	case "mp4v":
		return "mpeg4-video"
	case "vp09":
		return "vp9"
	case "av01":
		return "av1"
	default:
		return ""
	}
}

func extractAVIMetadata(path string, info domain.FormatInfo) domain.FormatInfo {
	buf := readPrefix(path, 1<<20)
	if len(buf) < 12 || string(buf[:4]) != "RIFF" || string(buf[8:12]) != "AVI " {
		return info
	}
	idx := bytes.Index(buf, []byte("avih"))
	if idx >= 0 && idx+48 <= len(buf) {
		microsPerFrame := binary.LittleEndian.Uint32(buf[idx+8 : idx+12])
		totalFrames := binary.LittleEndian.Uint32(buf[idx+24 : idx+28])
		info.Width = int(binary.LittleEndian.Uint32(buf[idx+40 : idx+44]))
		info.Height = int(binary.LittleEndian.Uint32(buf[idx+44 : idx+48]))
		if microsPerFrame > 0 && totalFrames > 0 {
			info.Duration = float64(microsPerFrame) * float64(totalFrames) / 1e6
		}
	}
	idx = bytes.Index(buf, []byte("strh"))
	if idx >= 0 && idx+16 <= len(buf) && string(buf[idx+8:idx+12]) == "vids" {
		info.Codec = videoCodec(string(buf[idx+12 : idx+16]))
	}
	return info
}

func extractMPEGMetadata(path string, info domain.FormatInfo) domain.FormatInfo {
	buf := readPrefix(path, 512<<10)
	idx := bytes.Index(buf, []byte{0, 0, 1, 0xb3})
	if idx >= 0 && idx+8 <= len(buf) {
		info.Width = int(buf[idx+4])<<4 | int(buf[idx+5]>>4)
		info.Height = int(buf[idx+5]&0x0f)<<8 | int(buf[idx+6])
		info.Codec = "mpeg-video"
	}
	return info
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
