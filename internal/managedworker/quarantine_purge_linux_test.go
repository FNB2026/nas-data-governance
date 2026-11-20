//go:build linux

package managedworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func TestQuarantinePurgeDurableWorkerFlow(t *testing.T) {
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
	svc, err := NewQuarantinePurge(st, root)
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.Plan(ctx, items[0].ID)
	if err != nil || p.State != "DRAFT" {
		t.Fatal("plan failed", err)
	}
	if _, err := svc.Execute(ctx, p.ID, p.Digest, true); !errors.Is(err, ErrAuthorization) {
		t.Fatal("execution without approval admitted", err)
	}
	if _, err := svc.Approve(ctx, p.ID, p.Digest); err != nil {
		t.Fatal("approve failed", err)
	}
	if _, err := svc.DryRun(ctx, p.ID, p.Digest); err != nil {
		t.Fatal("dry run failed", err)
	}
	if _, err := svc.Execute(ctx, p.ID, p.Digest, false); !errors.Is(err, ErrAuthorization) {
		t.Fatal("missing confirmation admitted", err)
	}
	result, err := svc.Execute(ctx, p.ID, p.Digest, true)
	if err != nil || result.State != "PURGED" {
		t.Fatal("commit failed", err)
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
	item, err := reopened.GetQuarantineItem(ctx, items[0].ID)
	if err != nil || item.Status != domain.QuarantinePurged {
		t.Fatal("quarantine item did not transition", err)
	}
}
