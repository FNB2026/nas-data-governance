package format

import (
	"encoding/binary"
	"io"
	"math"
	"os"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

const audioHeaderLimit = 64 << 10

func readPrefix(path string, limit int) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, limit)
	n, _ := io.ReadFull(f, buf)
	return buf[:n]
}

func extractWAVMetadata(path string, info domain.FormatInfo) domain.FormatInfo {
	buf := readPrefix(path, audioHeaderLimit)
	if len(buf) < 12 || (string(buf[:4]) != "RIFF" && string(buf[:4]) != "RF64") || string(buf[8:12]) != "WAVE" {
		return info
	}
	var byteRate uint32
	var dataSize uint64
	for offset, chunks := 12, 0; offset+8 <= len(buf) && chunks < 256; chunks++ {
		id := string(buf[offset : offset+4])
		size := uint64(binary.LittleEndian.Uint32(buf[offset+4 : offset+8]))
		data := offset + 8
		switch id {
		case "ds64":
			if data+16 <= len(buf) {
				dataSize = binary.LittleEndian.Uint64(buf[data+8 : data+16])
			}
		case "fmt ":
			if data+16 <= len(buf) {
				code := binary.LittleEndian.Uint16(buf[data : data+2])
				byteRate = binary.LittleEndian.Uint32(buf[data+8 : data+12])
				info.Codec = wavCodec(code)
			}
		case "data":
			if size != uint64(0xffffffff) {
				dataSize = size
			}
		}
		if byteRate > 0 && dataSize > 0 {
			info.Duration = float64(dataSize) / float64(byteRate)
			return info
		}
		step := size + size%2
		if step > uint64(len(buf)) || uint64(data)+step > uint64(len(buf)) {
			break
		}
		offset = data + int(step)
	}
	return info
}

func wavCodec(code uint16) string {
	switch code {
	case 1:
		return "pcm"
	case 3:
		return "pcm-float"
	case 6:
		return "a-law"
	case 7:
		return "mu-law"
	case 0xfffe:
		return "wave-extensible"
	default:
		if code != 0 {
			return "wav-codec-unknown"
		}
		return ""
	}
}

func extractAIFFMetadata(path string, info domain.FormatInfo) domain.FormatInfo {
	buf := readPrefix(path, audioHeaderLimit)
	if len(buf) < 12 || string(buf[:4]) != "FORM" || (string(buf[8:12]) != "AIFF" && string(buf[8:12]) != "AIFC") {
		return info
	}
	for offset, chunks := 12, 0; offset+8 <= len(buf) && chunks < 256; chunks++ {
		id := string(buf[offset : offset+4])
		size := int(binary.BigEndian.Uint32(buf[offset+4 : offset+8]))
		data := offset + 8
		if id == "COMM" && size >= 18 && data+18 <= len(buf) {
			frames := binary.BigEndian.Uint32(buf[data+2 : data+6])
			rate := extended80(buf[data+8 : data+18])
			if string(buf[8:12]) == "AIFC" && size >= 22 && data+22 <= len(buf) {
				info.Codec = aiffCodec(string(buf[data+18 : data+22]))
			} else {
				info.Codec = "pcm-be"
			}
			if frames > 0 && rate > 0 {
				info.Duration = float64(frames) / rate
			}
			return info
		}
		step := size + size%2
		if size < 0 || data+step > len(buf) {
			break
		}
		offset = data + step
	}
	return info
}

func extended80(value []byte) float64 {
	if len(value) < 10 {
		return 0
	}
	exponent := int(binary.BigEndian.Uint16(value[:2]) & 0x7fff)
	mantissa := binary.BigEndian.Uint64(value[2:10])
	if exponent == 0 || mantissa == 0 {
		return 0
	}
	rate := math.Ldexp(float64(mantissa), exponent-16383-63)
	if value[0]&0x80 != 0 {
		return -rate
	}
	return rate
}

func aiffCodec(code string) string {
	switch code {
	case "NONE", "twos":
		return "pcm-be"
	case "sowt":
		return "pcm-le"
	case "fl32", "FL32":
		return "pcm-float"
	case "alaw":
		return "a-law"
	case "ulaw":
		return "mu-law"
	default:
		if code != "" {
			return code
		}
		return "aiff-codec-unknown"
	}
}

func extractFLACMetadata(path string, info domain.FormatInfo) domain.FormatInfo {
	buf := readPrefix(path, 64)
	if len(buf) < 42 || string(buf[:4]) != "fLaC" || buf[4]&0x7f != 0 {
		return info
	}
	packed := binary.BigEndian.Uint64(buf[18:26])
	sampleRate := packed >> 44
	totalSamples := packed & 0x0000000fffffffff
	if sampleRate > 0 && totalSamples > 0 {
		info.Duration = float64(totalSamples) / float64(sampleRate)
	}
	info.Codec = "flac"
	return info
}
