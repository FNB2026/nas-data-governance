//go:build linux

package managedworker_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/govweb"
	"github.com/FNB2026/nas-data-governance/internal/managedfs"
	"github.com/FNB2026/nas-data-governance/internal/managedworker"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

type quarantinePurgeFixture struct {
	base, root, path, sentinel, database string
	raw                                  []byte
	store                                *store.SQLiteStore
	service                              *managedworker.QuarantinePurge
	itemID                               string
}

func newQuarantinePurgeFixture(t *testing.T) quarantinePurgeFixture {
	t.Helper()
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Skip("worker runs as a non-root account")
	}
	ctx := context.Background()
	base := t.TempDir()
	root := filepath.Join(base, "q")
	for _, dir := range []string{root, filepath.Join(root, ".ndg-purge-stage")} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	raw := []byte("synthetic quarantine object")
	hash := sha256.Sum256(raw)
	path := filepath.Join(root, "synthetic-file")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(base, "outside")
	if err := os.WriteFile(sentinel, raw, 0600); err != nil {
		t.Fatal(err)
	}

	database := filepath.Join(base, "test.db")
	st, err := store.Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	now := time.Now().UTC()
	task := domain.OperationTask{ID: "synthetic-task", RootPath: base, State: "completed", CreatedAt: now}
	plan := domain.OperationPlan{ID: "synthetic-quarantine", State: domain.PlanVerified, Actions: []domain.PlannedAction{{
		Path: filepath.Join(base, "original"), Action: domain.OperationQuarantine, TargetPath: path,
		File: domain.FileInstance{Size: int64(len(raw)), ContentSHA256: hex.EncodeToString(hash[:])},
	}}}
	for _, action := range []func() error{
		func() error { return st.CreateTask(ctx, task) },
		func() error { return st.SavePlans(ctx, task.ID, []domain.OperationPlan{plan}) },
		func() error { return st.BeginJournal(ctx, task.ID, plan.ID, plan.Actions) },
		func() error { return st.MarkJournalDone(ctx, plan.ID, 0, path) },
	} {
		if err := action(); err != nil {
			t.Fatal(err)
		}
	}
	items, err := st.RegisterQuarantinesFromJournal(ctx, plan.ID, now.Add(-31*24*time.Hour), now.Add(-24*time.Hour))
	if err != nil || len(items) != 1 {
		t.Fatal("fixture quarantine registration failed", err)
	}
	svc, err := managedworker.NewQuarantinePurge(st, root)
	if err != nil {
		t.Fatal(err)
	}
	return quarantinePurgeFixture{base, root, path, sentinel, database, raw, st, svc, items[0].ID}
}

func TestQuarantinePurgeDurableWorkerFlow(t *testing.T) {
	fixture := newQuarantinePurgeFixture(t)
	base, path, sentinel, database := fixture.base, fixture.path, fixture.sentinel, fixture.database
	raw, st, svc := fixture.raw, fixture.store, fixture.service
	ctx := context.Background()
	runDir := filepath.Join(base, "run")
	stateDir := filepath.Join(base, "state")
	for _, dir := range []string{runDir, stateDir} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	subject := managedworker.Subject{UID: uint32(os.Geteuid()), GID: uint32(os.Getegid())}
	listener, err := managedworker.Listen(managedworker.Layout{Run: runDir, State: stateDir}, subject, []managedworker.Subject{subject})
	if err != nil {
		t.Fatal("IPC listener", err)
	}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			_ = c.ServeQuarantinePurge(ctx, svc)
		}
	}()
	defer func() {
		_ = listener.Close()
		<-workerDone
	}()
	token, err := govweb.RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	credential := filepath.Join(base, "test-credential")
	if err := os.WriteFile(credential, []byte(token+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	web, err := govweb.New(govweb.Config{
		Addr: "127.0.0.1:8443", CredentialPath: credential, RequireTLS: true, Backing: govweb.ClosedBacking{},
		QuarantinePurge: managedworker.QuarantineClient{RunDir: runDir, Worker: subject},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(web.Handler())
	defer server.Close()
	post := func(payload map[string]string, expected int) managedworker.QuarantinePurgeResult {
		t.Helper()
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, server.URL+"/gov/quarantine-purge", strings.NewReader(string(raw)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != expected {
			t.Fatalf("%s status %d, want %d", payload["op"], resp.StatusCode, expected)
		}
		var result managedworker.QuarantinePurgeResult
		if expected == http.StatusOK {
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	p := post(map[string]string{"op": "QUARANTINE_PURGE_PLAN", "item_id": fixture.itemID}, http.StatusOK)
	if p.State != "DRAFT" {
		t.Fatal("plan did not start in draft")
	}
	step := func(op, confirmation string, expected int) managedworker.QuarantinePurgeResult {
		t.Helper()
		fields := map[string]string{"op": op, "plan_id": p.ID, "digest": p.Digest}
		if confirmation != "" {
			fields["confirmation"] = confirmation
		}
		return post(fields, expected)
	}
	step("QUARANTINE_PURGE_EXECUTE", "永久清理 "+p.ID, http.StatusConflict)
	step("QUARANTINE_PURGE_APPROVE", "", http.StatusOK)
	step("QUARANTINE_PURGE_DRY_RUN", "", http.StatusOK)
	step("QUARANTINE_PURGE_EXECUTE", "", http.StatusConflict)
	result := step("QUARANTINE_PURGE_EXECUTE", "永久清理 "+p.ID, http.StatusOK)
	if result.State != "PURGED" {
		t.Fatal("commit did not produce a receipt")
	}
	if receipt := step("QUARANTINE_PURGE_RECEIPT", "", http.StatusOK); receipt.State != "PURGED" {
		t.Fatal("receipt could not be read through the Worker")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("selected item survived")
	}
	b, err := os.ReadFile(sentinel)
	if err != nil || string(b) != string(raw) {
		t.Fatal("outside sentinel changed")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	receipt, err := reopened.ReadDualPurgePlan(ctx, p.ID)
	if err != nil || receipt.State != "PURGED" || receipt.Digest != p.Digest {
		t.Fatal("persisted plan did not confirm deletion", err)
	}
	item, err := reopened.GetQuarantineItem(ctx, fixture.itemID)
	if err != nil || item.Status != domain.QuarantinePurged {
		t.Fatal("quarantine item did not transition", err)
	}
}

// This invokes the committed store and filesystem operations in a disposable
// child process and reopens the SQLite files under the same dedicated user.
func TestQuarantinePurgePersistsAcrossDatabaseProcessKill(t *testing.T) {
	for _, cut := range []string{"before_claim", "after_claim", "after_filesystem_commit", "after_database_commit"} {
		t.Run(cut, func(t *testing.T) {
			f := newQuarantinePurgeFixture(t)
			ctx := context.Background()
			p, err := f.service.Plan(ctx, f.itemID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.Approve(ctx, p.ID, p.Digest); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.DryRun(ctx, p.ID, p.Digest); err != nil {
				t.Fatal(err)
			}
			if err := f.store.Close(); err != nil {
				t.Fatal(err)
			}
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestQuarantinePurgeDatabaseKillChild$")
			cmd.Env = append(os.Environ(), "NDG_TEST_PURGE_CUT="+cut, "NDG_TEST_PURGE_DB="+f.database,
				"NDG_TEST_PURGE_PLAN="+p.ID)
			out, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
				t.Fatalf("expected real SIGKILL, got %v %s", err, out)
			}
			st, err := store.Open(ctx, f.database)
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			plan, err := st.ReadDualPurgePlan(ctx, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			item, err := st.GetQuarantineItem(ctx, f.itemID)
			if err != nil {
				t.Fatal(err)
			}
			caps, err := st.GovernanceCapabilities(ctx)
			if err != nil {
				t.Fatal(err)
			}
			wantState := "EXECUTING"
			if cut == "before_claim" {
				wantState = "DRY_RUN"
			}
			if cut == "after_database_commit" {
				wantState = "PURGED"
			}
			if plan.State != wantState || plan.Digest != p.Digest ||
				(item.Status == domain.QuarantinePurged) != (wantState == "PURGED") ||
				caps.RecoveryLockActive != (wantState == "EXECUTING") {
				t.Fatal("inconsistent restored SQLite lifecycle")
			}
			removed := cut == "after_filesystem_commit" || cut == "after_database_commit"
			for file, absent := range map[string]bool{f.path: removed,
				filepath.Join(f.root, ".ndg-purge-stage", "receipt-"+p.ID): !removed} {
				_, err := os.Lstat(file)
				if (absent && !errors.Is(err, os.ErrNotExist)) || (!absent && err != nil) {
					t.Fatalf("filesystem did not reflect interrupted phase: %v", err)
				}
			}
			if got, err := os.ReadFile(f.sentinel); err != nil || string(got) != string(f.raw) {
				t.Fatal("outside sentinel changed")
			}
		})
	}
}

func TestQuarantinePurgeDatabaseKillChild(t *testing.T) {
	cut := os.Getenv("NDG_TEST_PURGE_CUT")
	if cut == "" {
		return
	}
	database := os.Getenv("NDG_TEST_PURGE_DB")
	if !strings.HasPrefix(database, filepath.Join(os.TempDir(), "TestQuarantinePurgePersistsAcrossDatabaseProcessKill")) ||
		filepath.Base(database) != "test.db" {
		t.Fatal("subprocess requires its dedicated synthetic fixture")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, database)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	plan, err := st.ReadDualPurgePlan(ctx, os.Getenv("NDG_TEST_PURGE_PLAN"))
	if err != nil {
		t.Fatal(err)
	}
	item, err := st.GetQuarantineItem(ctx, plan.SubjectID)
	if err != nil {
		t.Fatal(err)
	}
	stop := func(phase string) {
		if phase != cut {
			return
		}
		if err := syscall.Kill(os.Getpid(), syscall.SIGKILL); err != nil {
			t.Fatal(err)
		}
	}
	release, err := st.AcquireExecutionLock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	stop("before_claim")
	if err := st.AdvanceQuarantinePurge(ctx, plan, item, "EXECUTING", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	stop("after_claim")
	var proof struct{ Target managedfs.PurgeTarget }
	root := filepath.Join(filepath.Dir(database), "q")
	if json.Unmarshal([]byte(plan.EvidenceJSON), &proof) != nil || proof.Target.Relative != "synthetic-file" {
		t.Fatal("missing synthetic target")
	}
	result, err := managedfs.ExecutePurgeTransaction(ctx, managedfs.PurgeTransaction{
		ID: plan.ID, ApprovalSHA256: plan.Digest, RootPath: root, Target: proof.Target,
	})
	if err != nil || result.State != "PURGED" {
		t.Fatal("synthetic filesystem commit failed", err)
	}
	stop("after_filesystem_commit")
	if err := st.FinishQuarantinePurge(ctx, plan, item, result.State, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	stop("after_database_commit")
	t.Fatal("subprocess did not reach kill point")
}
