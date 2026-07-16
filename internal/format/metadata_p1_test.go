package format

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestExtractWAVDurationAndCodec(t *testing.T) {
	data := make([]byte, 44)
	copy(data[0:4], "RIFF")
	copy(data[8:12], "WAVE")
	copy(data[12:16], "fmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 1)
	binary.LittleEndian.PutUint16(data[22:24], 2)
	binary.LittleEndian.PutUint32(data[24:28], 48000)
	binary.LittleEndian.PutUint32(data[28:32], 192000)
	copy(data[36:40], "data")
	binary.LittleEndian.PutUint32(data[40:44], 384000)
	path := writeBytes(t, "duration.wav", data)
	info := extractWAVMetadata(path, domain.FormatInfo{Format: "wav", Category: domain.CategoryAudio})
	if math.Abs(info.Duration-2) > 0.001 || info.Codec != "pcm" {
		t.Fatalf("got %#v", info)
	}
}

func TestExtractAIFFDurationAndCodec(t *testing.T) {
	data := make([]byte, 38)
	copy(data[0:4], "FORM")
	copy(data[8:12], "AIFF")
	copy(data[12:16], "COMM")
	binary.BigEndian.PutUint32(data[16:20], 18)
	binary.BigEndian.PutUint16(data[20:22], 2)
	binary.BigEndian.PutUint32(data[22:26], 88200)
	binary.BigEndian.PutUint16(data[26:28], 16)
	copy(data[28:38], []byte{0x40, 0x0e, 0xac, 0x44, 0, 0, 0, 0, 0, 0}) // 44100 Hz
	path := writeBytes(t, "duration.aiff", data)
	info := extractAIFFMetadata(path, domain.FormatInfo{Format: "aiff", Category: domain.CategoryAudio})
	if math.Abs(info.Duration-2) > 0.001 || info.Codec != "pcm-be" {
		t.Fatalf("got %#v", info)
	}
}

func TestExtractFLACDuration(t *testing.T) {
	data := make([]byte, 42)
	copy(data[:4], "fLaC")
	data[4] = 0 // STREAMINFO
	data[7] = 34
	packed := uint64(48000)<<44 | uint64(96000)
	binary.BigEndian.PutUint64(data[18:26], packed)
	path := writeBytes(t, "duration.flac", data)
	info := extractFLACMetadata(path, domain.FormatInfo{Format: "flac", Category: domain.CategoryAudio})
	if math.Abs(info.Duration-2) > 0.001 || info.Codec != "flac" {
		t.Fatalf("got %#v", info)
	}
}

func TestExtractMP4MetadataWhenMoovIsAfterMedia(t *testing.T) {
	mvhdPayload := make([]byte, 24)
	binary.BigEndian.PutUint32(mvhdPayload[12:16], 1000)
	binary.BigEndian.PutUint32(mvhdPayload[16:20], 5000)
	tkhdPayload := make([]byte, 84)
	binary.BigEndian.PutUint32(tkhdPayload[len(tkhdPayload)-8:len(tkhdPayload)-4], 1920<<16)
	binary.BigEndian.PutUint32(tkhdPayload[len(tkhdPayload)-4:], 1080<<16)
	stsdPayload := make([]byte, 16)
	binary.BigEndian.PutUint32(stsdPayload[4:8], 1)
	binary.BigEndian.PutUint32(stsdPayload[8:12], 16)
	copy(stsdPayload[12:16], "avc1")
	moov := testAtom("moov", append(append(testAtom("mvhd", mvhdPayload), testAtom("tkhd", tkhdPayload)...), testAtom("stsd", stsdPayload)...))
	file := append(testAtom("ftyp", []byte("isom\x00\x00\x00\x00")), testAtom("mdat", make([]byte, 2048))...)
	file = append(file, moov...)
	path := writeBytes(t, "tail-moov.mp4", file)
	info := extractISOBaseMediaMetadata(path, domain.FormatInfo{Format: "mp4", Category: domain.CategoryVideo})
	if math.Abs(info.Duration-5) > 0.001 || info.Width != 1920 || info.Height != 1080 || info.Codec != "h264" {
		t.Fatalf("got %#v", info)
	}
}

func TestExtractAVIMetadata(t *testing.T) {
	data := make([]byte, 128)
	copy(data[:4], "RIFF")
	copy(data[8:12], "AVI ")
	copy(data[12:16], "avih")
	binary.LittleEndian.PutUint32(data[20:24], 40000)
	binary.LittleEndian.PutUint32(data[36:40], 250)
	binary.LittleEndian.PutUint32(data[52:56], 1280)
	binary.LittleEndian.PutUint32(data[56:60], 720)
	copy(data[60:64], "strh")
	copy(data[68:72], "vids")
	copy(data[72:76], "avc1")
	path := writeBytes(t, "metadata.avi", data)
	info := extractAVIMetadata(path, domain.FormatInfo{Format: "avi", Category: domain.CategoryVideo})
	if math.Abs(info.Duration-10) > 0.001 || info.Width != 1280 || info.Height != 720 || info.Codec != "h264" {
		t.Fatalf("got %#v", info)
	}
}

func TestExtractMPEGDimensions(t *testing.T) {
	data := []byte{0, 0, 1, 0xb3, 0x78, 0x04, 0x38, 0}
	path := writeBytes(t, "metadata.mpg", data)
	info := extractMPEGMetadata(path, domain.FormatInfo{Format: "mpeg", Category: domain.CategoryVideo})
	if info.Width != 1920 || info.Height != 1080 || info.Codec != "mpeg-video" {
		t.Fatalf("got %#v", info)
	}
}

func testAtom(kind string, payload []byte) []byte {
	atom := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(atom[:4], uint32(len(atom)))
	copy(atom[4:8], kind)
	copy(atom[8:], payload)
	return atom
}
