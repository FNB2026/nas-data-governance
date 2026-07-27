package events

import (
	"testing"
)

func TestSanitizePayload_RemovesSensitiveKeys(t *testing.T) {
	payload := map[string]any{
		"discovered":     int64(100),
		"path":           "/secret/path/to/file",
		"filename":       "secret.txt",
		"source_path":    "/mnt/nas/source",
		"target_path":    "/mnt/nas/target",
		"error":          "open /secret/path: permission denied",
		"quarantine":     "/var/quarantine/abc",
		"db_path":        "/home/user/project.db",
		"processed":      int64(50),
	}

	result := SanitizePayload(payload)

	// Sensitive keys must be removed.
	for _, key := range []string{"path", "filename", "source_path", "target_path", "error", "quarantine", "db_path"} {
		if _, ok := result[key]; ok {
			t.Errorf("SanitizePayload: sensitive key %q should be removed", key)
		}
	}

	// Safe keys must be preserved.
	if result["discovered"] != int64(100) {
		t.Errorf("SanitizePayload: safe key 'discovered' should be preserved, got %v", result["discovered"])
	}
	if result["processed"] != int64(50) {
		t.Errorf("SanitizePayload: safe key 'processed' should be preserved, got %v", result["processed"])
	}
}

func TestSanitizePayload_DropsNilValues(t *testing.T) {
	payload := map[string]any{
		"discovered": int64(10),
		"failed":     nil,
		"stage":      "QUICK_HASHING",
	}

	result := SanitizePayload(payload)

	if _, ok := result["failed"]; ok {
		t.Error("SanitizePayload: nil values should be dropped")
	}
	if len(result) != 2 {
		t.Errorf("SanitizePayload: expected 2 keys, got %d", len(result))
	}
}

func TestSanitizePayload_EmptyInput(t *testing.T) {
	result := SanitizePayload(nil)
	if result == nil {
		t.Error("SanitizePayload: nil input should return empty map, not nil")
	}
	if len(result) != 0 {
		t.Errorf("SanitizePayload: nil input should return empty map, got %d keys", len(result))
	}
}

func TestSanitizePayload_AllSensitive(t *testing.T) {
	payload := map[string]any{
		"path": "/a/b/c",
		"file": "secret.bin",
	}

	result := SanitizePayload(payload)

	if len(result) != 0 {
		t.Errorf("SanitizePayload: all-sensitive payload should be empty, got %d keys", len(result))
	}
}
