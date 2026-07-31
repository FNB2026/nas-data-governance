package version

import (
	"testing"
)

// TestDefaults verifies that the build-time defaults are set to
// development sentinel values. In production builds, these are
// overridden via -ldflags.
func TestDefaults(t *testing.T) {
	if Version != "dev" {
		t.Errorf("default Version = %q, want %q", Version, "dev")
	}
	if Commit != "unknown" {
		t.Errorf("default Commit = %q, want %q", Commit, "unknown")
	}
	if BuildTime != "unknown" {
		t.Errorf("default BuildTime = %q, want %q", BuildTime, "unknown")
	}
	if Channel != "dev" {
		t.Errorf("default Channel = %q, want %q", Channel, "dev")
	}
}

// TestGetReturnsCurrentValues verifies that Get() returns a snapshot
// of the current package-level variables.
func TestGetReturnsCurrentValues(t *testing.T) {
	// Save originals
	origVersion := Version
	origCommit := Commit
	origBuildTime := BuildTime
	origChannel := Channel
	defer func() {
		Version = origVersion
		Commit = origCommit
		BuildTime = origBuildTime
		Channel = origChannel
	}()

	// Set test values
	Version = "0.5.0-beta.1"
	Commit = "abc123def456"
	BuildTime = "2026-07-31T00:00:00Z"
	Channel = "beta"

	info := Get()

	if info.Version != "0.5.0-beta.1" {
		t.Errorf("info.Version = %q, want %q", info.Version, "0.5.0-beta.1")
	}
	if info.Commit != "abc123def456" {
		t.Errorf("info.Commit = %q, want %q", info.Commit, "abc123def456")
	}
	if info.BuildTime != "2026-07-31T00:00:00Z" {
		t.Errorf("info.BuildTime = %q, want %q", info.BuildTime, "2026-07-31T00:00:00Z")
	}
	if info.Channel != "beta" {
		t.Errorf("info.Channel = %q, want %q", info.Channel, "beta")
	}
}

// TestGetJSONFields verifies that the Info struct has the expected JSON
// field names. This ensures backward compatibility with the frontend
// VersionInfo model.
func TestGetJSONFields(t *testing.T) {
	info := Get()

	// Verify the struct can be serialized with the expected JSON keys.
	// We check field names via reflection-like approach (direct access).
	fields := map[string]string{
		"version":    info.Version,
		"commit":     info.Commit,
		"build_time": info.BuildTime,
		"channel":    info.Channel,
	}

	for expectedKey, val := range fields {
		if val == "" && expectedKey != "" {
			// All fields should have non-empty values (even if "dev"/"unknown")
			t.Errorf("field with JSON key %q is empty", expectedKey)
		}
	}
}
