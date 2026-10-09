package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/fingerprint"
	"github.com/FNB2026/nas-data-governance/internal/jobs"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func TestResumeRestoresFullHashCandidatesWithoutNewTraversal(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "source")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("f%d", i)), []byte(fmt.Sprintf("pair%d", i/2)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "unique"), []byte("ordinary unique"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, filepath.Join(tmp, "project.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	calls := 0
	svc := NewScanServiceWithHashFunc(st, fingerprint.Quick, func(path string, _ int64) (string, error) {
		calls++
		if calls > 2 {
			return "", os.ErrNotExist
		}
		return fingerprint.Full(path)
	})
	in := ScanInput{Root: root, StorageID: "full-resume", NetworkSource: true, Workers: 1, HashAttempts: 1}
	paused, err := svc.Scan(ctx, in)
	if !errors.Is(err, ErrNetworkSourceUnavailable) {
		t.Fatalf("initial scan: %v", err)
	}
	cp, err := st.LastCheckpoint(ctx, in.StorageID)
	if err != nil {
		t.Fatal(err)
	}
	if cp.ScannedCount != 7 || cp.Status != "paused_network" {
		t.Fatalf("checkpoint: %+v", cp)
	}
	svc = NewScanService(st)
	in.Resume = true
	resumed, err := svc.Scan(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.CheckpointID != paused.CheckpointID || resumed.ResumedCount != 7 {
		t.Fatalf("resume identity: %+v", resumed)
	}
	if p := svc.Progress(); p.Discovered != 0 || p.Processed != 0 || p.Failed != 0 {
		t.Fatalf("prefix recovery inflated traversal: %+v", p)
	}
	metas, err := st.ListFileMetadata(ctx, in.StorageID)
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string]int{}
	for _, m := range metas {
		if filepath.Base(m.Path) == "unique" {
			if m.ContentSHA256 != "" {
				t.Fatal("ordinary file received full hash")
			}
			continue
		}
		want, err := fingerprint.Full(m.Path)
		if err != nil {
			t.Fatal(err)
		}
		if m.ContentSHA256 != want {
			t.Fatalf("pending full hash not recovered")
		}
		groups[m.ContentSHA256]++
	}
	if len(metas) != 7 || len(groups) != 3 {
		t.Fatalf("files=%d groups=%d", len(metas), len(groups))
	}
	if _, err := st.LastCheckpoint(ctx, in.StorageID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("checkpoint not completed: %v", err)
	}
}

// This fixture pauses after traversal, using real quick hashes and an injected
// content-read interruption. All state is produced through normal service/store APIs.
func pausedFullHashFixture(t *testing.T) (*store.SQLiteStore, ScanInput, int64) {
	t.Helper()
	return pausedFullHashFixtureCount(t, 6)
}

func pausedFullHashFixtureCount(t *testing.T, count int) (*store.SQLiteStore, ScanInput, int64) {
	t.Helper()
	ctx := context.Background()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "source")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("f%d", i)), []byte("same duplicate content"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(ctx, filepath.Join(tmp, "project.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	in := ScanInput{Root: root, StorageID: "resume-canary", NetworkSource: true, Workers: 1, HashAttempts: 1}
	svc := NewScanServiceWithHashFunc(st, fingerprint.Quick, func(string, int64) (string, error) { return "", os.ErrNotExist })
	result, err := svc.Scan(ctx, in)
	if !errors.Is(err, ErrNetworkSourceUnavailable) {
		t.Fatalf("pause: %v", err)
	}
	in.Resume = true
	return st, in, result.CheckpointID
}

func TestResumeFullHashRepeatedInterruptAndIdempotence(t *testing.T) {
	st, in, cpID := pausedFullHashFixture(t)
	ctx := context.Background()
	calls := 0
	svc := NewScanServiceWithHashFunc(st, fingerprint.Quick, func(path string, _ int64) (string, error) {
		calls++
		if calls > 1 {
			return "", os.ErrNotExist
		}
		return fingerprint.Full(path)
	})
	_, err := svc.Scan(ctx, in)
	if !errors.Is(err, ErrNetworkSourceUnavailable) {
		t.Fatalf("second interruption: %v", err)
	}
	cp, err := st.LastCheckpoint(ctx, in.StorageID)
	if err != nil || cp.ID != cpID || cp.ScannedCount != 6 || cp.Status != "paused_network" {
		t.Fatalf("checkpoint regressed: %+v err=%v", cp, err)
	}
	files, err := st.ListFiles(ctx, in.StorageID)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := st.UpsertFiles(ctx, files)
	if err != nil {
		t.Fatal(err)
	}
	svc = NewScanService(st)
	res, err := svc.Scan(ctx, in)
	if err != nil || res.CheckpointID != cpID {
		t.Fatalf("final resume: %v", err)
	}
	calls = 0
	svc = NewScanServiceWithHashFunc(st, fingerprint.Quick, func(path string, _ int64) (string, error) { calls++; return fingerprint.Full(path) })
	if _, err := svc.Scan(ctx, in); err != nil {
		t.Fatal(err)
	} // completed checkpoint -> normal incremental pass
	after, err := st.ListFiles(ctx, in.StorageID)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || len(after) != 6 {
		t.Fatalf("idempotent scan: full calls=%d rows=%d", calls, len(after))
	}
	afterIDs, err := st.UpsertFiles(ctx, after)
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range after {
		if afterIDs[i] != ids[i] || f.ContentSHA256 == "" {
			t.Fatal("duplicate identity or lost hash")
		}
	}
}

func TestResumeFullHashCancellationKeepsWorkResumable(t *testing.T) {
	st, in, cpID := pausedFullHashFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc := NewScanServiceWithHashFunc(st, fingerprint.Quick, func(path string, _ int64) (string, error) { cancel(); return fingerprint.Full(path) })
	if _, err := svc.Scan(ctx, in); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled full hashing declared completion: %v", err)
	}
	cp, err := st.LastCheckpoint(context.Background(), in.StorageID)
	if err != nil || cp.ID != cpID || cp.Status != "aborted" || cp.ScannedCount != 6 {
		t.Fatalf("cancel checkpoint: %+v err=%v", cp, err)
	}
	if _, err := NewScanService(st).Scan(context.Background(), in); err != nil {
		t.Fatal(err)
	}
}

func TestResumeFullHashRevalidatesChangedFile(t *testing.T) {
	st, in, _ := pausedFullHashFixture(t)
	ctx := context.Background()
	changed := filepath.Join(in.Root, "f0")
	if err := os.WriteFile(changed, []byte("now unique content of different size"), 0600); err != nil {
		t.Fatal(err)
	}
	res, err := NewScanService(st).Scan(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	metas, err := st.ListFileMetadata(ctx, in.StorageID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range metas {
		if m.Path == changed {
			if m.ContentSHA256 != "" || m.Size != int64(len("now unique content of different size")) {
				t.Fatalf("changed ordinary file wrongly retained old hash: size=%d", m.Size)
			}
		} else if m.ContentSHA256 == "" {
			t.Fatal("remaining duplicates not hashed")
		}
	}
	if res.Missing != 0 || res.Unavailable != 0 {
		t.Fatal("recovery altered existence status")
	}
}

func TestResumeFullHashRejectsChangeDuringRead(t *testing.T) {
	st, in, _ := pausedFullHashFixture(t)
	changed := false
	svc := NewScanServiceWithHashFunc(st, fingerprint.Quick, func(path string, _ int64) (string, error) {
		h, err := fingerprint.Full(path)
		if !changed {
			changed = true
			os.WriteFile(path, []byte("different size"), 0600)
		}
		return h, err
	})
	res, err := svc.Scan(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.HashFailures) != 1 || svc.Progress().Failed != 1 {
		t.Fatalf("stale read not recorded: failures=%d", len(res.HashFailures))
	}
	metas, _ := st.ListFileMetadata(context.Background(), in.StorageID)
	for _, m := range metas {
		if m.Path == res.HashFailures[0].Path && m.ContentSHA256 != "" {
			t.Fatal("stale full hash persisted")
		}
	}
}

func TestResumeFullHashDoesNotFollowReplacedSymlink(t *testing.T) {
	st, in, _ := pausedFullHashFixture(t)
	path := filepath.Join(in.Root, "f0")
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	calls := 0
	svc := NewScanServiceWithHashFunc(st, func(path string, size int64) (string, error) {
		if path == outside {
			t.Fatal("read outside")
		}
		return fingerprint.Quick(path, size)
	}, func(string, int64) (string, error) { calls++; return "should-not-complete", nil })
	if _, err := svc.Scan(context.Background(), in); err == nil || errors.Is(err, ErrNetworkSourceUnavailable) {
		t.Fatalf("unsafe prefix silently completed: %v", err)
	}
	if calls != 0 {
		t.Fatal("hashed after failed boundary validation")
	}
	cp, err := st.LastCheckpoint(context.Background(), in.StorageID)
	if err != nil || cp.Status != "aborted" {
		t.Fatal("unsafe recovery not resumable")
	}
}

type failResumeStore struct {
	ScanStore
	failOutcome bool
}

func (s failResumeStore) UpsertFiles(ctx context.Context, files []domain.FileInstance) ([]int64, error) {
	if !s.failOutcome && len(files) > 0 {
		return nil, errors.New("injected persistence failure")
	}
	return s.ScanStore.UpsertFiles(ctx, files)
}
func (s failResumeStore) CompleteCheckpoint(ctx context.Context, id int64, status string) error {
	if s.failOutcome && status == "completed" {
		return errors.New("injected checkpoint failure")
	}
	return s.ScanStore.CompleteCheckpoint(ctx, id, status)
}
func TestResumeFullHashPersistenceRequiredForCompletion(t *testing.T) {
	for _, outcome := range []bool{false, true} {
		t.Run(fmt.Sprint(outcome), func(t *testing.T) {
			st, in, _ := pausedFullHashFixture(t)
			svc := NewScanService(failResumeStore{ScanStore: st, failOutcome: outcome})
			if _, err := svc.Scan(context.Background(), in); err == nil {
				t.Fatal("persistence failure declared complete")
			}
			if _, err := st.LastCheckpoint(context.Background(), in.StorageID); err != nil {
				t.Fatal("failed persistence lost resumable checkpoint")
			}
		})
	}
}

func TestResumeFullHashCrossesCheckpointWithCompletedPeer(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	root := filepath.Join(tmp, "source")
	os.Mkdir(root, 0700)
	for _, name := range []string{"a", "b"} {
		os.WriteFile(filepath.Join(root, name), []byte("pair"), 0600)
	}
	st, err := store.Open(ctx, filepath.Join(tmp, "project.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	in := ScanInput{Root: root, StorageID: "cross-prefix", Workers: 1, HashAttempts: 1}
	if _, err := NewScanService(st).Scan(ctx, in); err != nil {
		t.Fatal(err)
	}
	files, _ := st.ListFiles(ctx, in.StorageID)
	files[1].ContentSHA256 = ""
	st.UpsertFiles(ctx, files)
	cp, _ := st.StartCheckpoint(ctx, in.StorageID)
	st.UpdateCheckpoint(ctx, cp, filepath.Join(root, "a"), 1)
	st.CompleteCheckpoint(ctx, cp, "paused_network")
	fullCalls := 0
	svc := NewScanServiceWithHashFunc(st, fingerprint.Quick, func(path string, _ int64) (string, error) { fullCalls++; return fingerprint.Full(path) })
	in.Resume = true
	res, err := svc.Scan(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := st.ListFileMetadata(ctx, in.StorageID)
	if fullCalls != 1 || svc.Progress().Discovered != 1 || res.ResumedCount != 1 || len(after) != 2 || after[0].ContentSHA256 != after[1].ContentSHA256 {
		t.Fatalf("cross-boundary candidate not recovered: calls=%d progress=%+v", fullCalls, svc.Progress())
	}
}

type unreliableResumeStore struct{ ScanStore }

func (s unreliableResumeStore) ListFileMetadata(ctx context.Context, id string) ([]store.FileMeta, error) {
	metas, err := s.ScanStore.ListFileMetadata(ctx, id)
	for i := range metas {
		metas[i].PhysicalReliable = false
	}
	return metas, err
}
func TestResumeFullHashDoesNotTrustUnreliableCachedPeer(t *testing.T) {
	st, in, _ := pausedFullHashFixture(t)
	ctx := context.Background()
	files, _ := st.ListFiles(ctx, in.StorageID)
	files[0].ContentSHA256 = "stale-network-hash"
	st.UpsertFiles(ctx, files)
	calls := 0
	svc := NewScanServiceWithHashFunc(unreliableResumeStore{st}, fingerprint.Quick, func(path string, _ int64) (string, error) { calls++; return fingerprint.Full(path) })
	if _, err := svc.Scan(ctx, in); err != nil {
		t.Fatal(err)
	}
	if calls != 6 {
		t.Fatalf("unreliable peer reused stale hash: calls=%d", calls)
	}
	metas, err := st.ListFileMetadata(ctx, in.StorageID)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range metas {
		want, err := fingerprint.Full(m.Path)
		if err != nil || m.ContentSHA256 != want {
			t.Fatal("unreliable cached hash survived")
		}
	}
}

func TestResumeFullHashFailureExplainedWithoutPrivateEvents(t *testing.T) {
	st, in, _ := pausedFullHashFixture(t)
	ctx := context.Background()
	svc := NewScanServiceWithHashFunc(st, fingerprint.Quick, func(path string, _ int64) (string, error) {
		return "", &os.PathError{Op: "open", Path: path, Err: os.ErrPermission}
	})
	mgr := jobs.New(st)
	runner := NewScanJobRunner(svc, mgr)
	id, res, err := runner.RunScanAsJob(ctx, filepath.Join(in.Root, "private-project-marker"), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.HashFailures) != 6 || svc.Progress().Failed != 6 {
		t.Fatal("non-network failures not terminal/explainable")
	}
	run, err := mgr.Get(ctx, id)
	if err != nil || run.State != jobs.StateCompleted || run.WarningCount != 1 {
		t.Fatalf("unexpected nonfatal outcome: %+v err=%v", run, err)
	}
	events, err := mgr.ListEvents(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		b, err := json.Marshal(e.Payload)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), in.Root) || strings.Contains(string(b), "private-project-marker") || strings.Contains(string(b), "f0") {
			t.Fatal("event leaked recovery path")
		}
		if e.EventType == "job:warning" && e.Payload["full_hash_failures"] != float64(6) {
			t.Fatalf("missing full hash warning: %+v", e.Payload)
		}
	}
}

func TestResumeQuickFailureExplainsFormerCandidateSingleton(t *testing.T) {
	st, in, _ := pausedFullHashFixtureCount(t, 2)
	ctx := context.Background()
	svc := NewScanServiceWithHashFunc(unreliableResumeStore{st}, func(path string, size int64) (string, error) {
		if filepath.Base(path) == "f0" {
			return "", os.ErrPermission
		}
		return fingerprint.Quick(path, size)
	}, func(path string, _ int64) (string, error) { return fingerprint.Full(path) })
	res, err := svc.Scan(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.HashFailures) != 1 || res.HashFailures[0].Stage != "quick" {
		t.Fatalf("lost terminal quick failure: %+v", res.HashFailures)
	}
}

func TestResumeFullHashRetriesTransientFailure(t *testing.T) {
	st, in, _ := pausedFullHashFixtureCount(t, 2)
	in.HashAttempts = 2
	calls := map[string]int{}
	svc := NewScanServiceWithHashFunc(st, fingerprint.Quick, func(path string, _ int64) (string, error) {
		calls[path]++
		if calls[path] == 1 {
			return "", os.ErrPermission
		}
		return fingerprint.Full(path)
	})
	res, err := svc.Scan(context.Background(), in)
	if err != nil || len(res.HashFailures) != 0 {
		t.Fatalf("retry failed: %v", err)
	}
	files, err := st.ListFiles(context.Background(), in.StorageID)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if calls[f.Path] != 2 || f.ContentSHA256 == "" {
			t.Fatal("retry did not persist candidate exactly once")
		}
	}
}
