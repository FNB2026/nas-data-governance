package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nas-data-governance/internal/domain"
)

// makeFile 创建一个用于测试的 FileInstance。
func makeFile(path, hash string, size int64) domain.FileInstance {
	return domain.FileInstance{
		StorageID:     "test",
		Path:          path,
		Name:          filepath.Base(path),
		Size:          size,
		Mode:          0o644,
		ModifiedAt:    time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Device:        1,
		Inode:         100,
		QuickHash:     hash,
		ContentSHA256: hash,
		DiscoveredAt:  time.Date(2024, 1, 16, 8, 0, 0, 0, time.UTC),
	}
}

// TestWriteReadRoundTrip 验证 Write 后 Read 能恢复完整数据。
func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.jsonl")

	original := []domain.FileInstance{
		makeFile("/data/file1.txt", "abc123", 1024),
		makeFile("/data/file2.txt", "def456", 2048),
		makeFile("/data/sub/file3.txt", "ghi789", 512),
	}

	if err := Write(path, original); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != len(original) {
		t.Fatalf("expected %d files, got %d", len(original), len(got))
	}
	for i, want := range original {
		if got[i].Path != want.Path {
			t.Errorf("file[%d].Path = %q, want %q", i, got[i].Path, want.Path)
		}
		if got[i].Size != want.Size {
			t.Errorf("file[%d].Size = %d, want %d", i, got[i].Size, want.Size)
		}
		if got[i].ContentSHA256 != want.ContentSHA256 {
			t.Errorf("file[%d].ContentSHA256 = %q, want %q", i, got[i].ContentSHA256, want.ContentSHA256)
		}
		if !got[i].ModifiedAt.Equal(want.ModifiedAt) {
			t.Errorf("file[%d].ModifiedAt = %v, want %v", i, got[i].ModifiedAt, want.ModifiedAt)
		}
	}
}

func TestWalkReportsMalformedLineWithoutContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.jsonl")
	if err := os.WriteFile(path, []byte("{}\nprivate-secret-not-json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	visited := 0
	err := Walk(path, func(domain.FileInstance) error {
		visited++
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("expected line-numbered error, got %v", err)
	}
	if strings.Contains(err.Error(), "private-secret") {
		t.Fatalf("malformed content leaked: %v", err)
	}
	if visited != 1 {
		t.Fatalf("visited=%d, want 1", visited)
	}
}

// TestWriteEmptyList 验证空列表写入后读取也是空。
func TestWriteEmptyList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")

	if err := Write(path, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 files, got %d", len(got))
	}
}

// TestWriteCreatesParentDir 验证 Write 不会创建父目录（os.Create 要求父目录存在）。
func TestWriteMissingParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent", "index.jsonl")

	err := Write(path, []domain.FileInstance{makeFile("/a", "x", 1)})
	if err == nil {
		t.Fatal("expected error for missing parent directory")
	}
}

// TestReadNonexistentFile 验证读取不存在的文件返回错误。
func TestReadNonexistentFile(t *testing.T) {
	_, err := Read("/tmp/nas-governance-nonexistent-test-file-12345.jsonl")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected os.IsNotExist error, got %v", err)
	}
}

// TestReadCorruptedJSON 验证损坏的 JSON 行返回错误。
func TestReadCorruptedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.jsonl")
	// 第一行正常，第二行损坏
	content := `{"path":"/ok","size":100}` + "\n" + `{invalid json` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(path)
	if err == nil {
		t.Fatal("expected error for corrupted JSON")
	}
}

// TestReadEmptyFile 验证读取空文件返回空列表无错误。
func TestReadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 files from empty file, got %d", len(got))
	}
}

// TestReadBlankLineReturnsError 验证空行会导致 JSON 解析错误。
// bufio.Scanner 对空行返回空 token，json.Unmarshal(空) 会报错。
// 这是预期行为：JSONL 文件不应包含空行。
func TestReadBlankLineReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blanks.jsonl")
	content := `{"path":"/a","size":1}` + "\n\n" + `{"path":"/b","size":2}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(path)
	if err == nil {
		t.Fatal("expected error for blank line in JSONL")
	}
}
