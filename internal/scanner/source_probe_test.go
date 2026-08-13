package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProfileForFilesystemIsConservativeForNetworkFilesystems(t *testing.T) {
	for _, fsType := range []string{"smbfs", "nfs", "webdav", "fusefs", "macfuse"} {
		normalized, network, reliable := profileForFilesystem(fsType)
		if normalized != fsType || !network || reliable {
			t.Fatalf("profileForFilesystem(%q) = %q network=%v reliable=%v", fsType, normalized, network, reliable)
		}
	}
}

func TestRecommendedWorkersBoundsHighLatencyNetworkSources(t *testing.T) {
	if got := recommendedWorkers(true, 413*time.Millisecond); got != 1 {
		t.Fatalf("high-latency network workers = %d, want 1", got)
	}
	if got := recommendedWorkers(true, 40*time.Millisecond); got != 2 {
		t.Fatalf("low-latency network workers = %d, want 2", got)
	}
	if got := recommendedWorkers(false, time.Second); got != 4 {
		t.Fatalf("local workers = %d, want 4", got)
	}
}

func TestProbeSourceReadsDirectoryWithoutTraversingFiles(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(root, "must-not-open.txt")
	if err := os.WriteFile(secret, []byte("content must not be read"), 0); err != nil {
		t.Fatal(err)
	}
	profile, err := ProbeSource(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Status != SourceOnline || profile.RecommendedWorkers < 1 {
		t.Fatalf("unexpected profile: %#v", profile)
	}
}

func TestProbeSourceRejectsSymlinkRoot(t *testing.T) {
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "source-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := ProbeSource(context.Background(), link)
	var probeErr *ProbeError
	if !errors.As(err, &probeErr) || probeErr.Code != "symlink_rejected" {
		t.Fatalf("error = %v, want symlink_rejected", err)
	}
}

func TestProbeSourceCancelledContextReturnsPathFreeTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ProbeSource(ctx, "/private/sensitive/customer-share")
	if err == nil || err.Error() != "scanner: source preflight timed out; path omitted" {
		t.Fatalf("error = %v", err)
	}
}

// TestProbeMountedSourceFromEnv is an opt-in acceptance test for a real,
// already-mounted source. It performs the same bounded read-only preflight as
// the desktop app and never prints the source path. Example:
//
//	NDG_TEST_MOUNTED_SOURCE=/Volumes/example go test ./internal/scanner -run TestProbeMountedSourceFromEnv -v
func TestProbeMountedSourceFromEnv(t *testing.T) {
	root := os.Getenv("NDG_TEST_MOUNTED_SOURCE")
	if root == "" {
		t.Skip("NDG_TEST_MOUNTED_SOURCE is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	profile, err := ProbeSource(ctx, root)
	if err != nil {
		t.Fatalf("mounted source preflight failed (path omitted): %v", err)
	}
	if profile.Status != SourceOnline {
		t.Fatalf("mounted source status = %q, want online", profile.Status)
	}
	t.Logf("mounted source accepted: filesystem=%s network=%v latency_ms=%d workers=%d physical_identity_reliable=%v",
		profile.FilesystemType, profile.Network, profile.Latency.Milliseconds(), profile.RecommendedWorkers, profile.PhysicalIdentityReliable)
}
