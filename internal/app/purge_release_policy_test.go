package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPurgeReleasePolicyDeniesBeforeSideEffects(t *testing.T) {
	t.Setenv("NDG_ALLOW_PURGE", "true")
	ctx := context.Background()
	st := openTestStore(t)
	root := t.TempDir()
	source, quarantine := filepath.Join(root, "source"), filepath.Join(root, "quarantine")
	for _, p := range []string{source, quarantine} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	item := seedQuarantineItem(t, st, source, quarantine, "control", now.Add(-time.Hour), now.Add(-time.Minute))
	before, err := os.ReadFile(item.QuarantinePath)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewPurgeService(st)
	_, createErr := svc.CreatePlans(ctx)
	approveErr := svc.ApprovePlan(ctx, "private-path-canary", "digest")
	for _, dry := range []bool{true, false} {
		_, err := svc.ExecutePurge(ctx, PurgeExecuteInput{PlanID: "private-path-canary", Digest: "digest", QuarantineRoot: quarantine, DryRun: dry, Confirmation: "anything"})
		if err == nil || err.Error() != "purge disabled by release policy" {
			t.Errorf("dry=%v: missing fixed policy refusal: %v", dry, err)
		}
	}
	for _, err := range []error{createErr, approveErr} {
		if err == nil || err.Error() != "purge disabled by release policy" {
			t.Errorf("missing policy refusal: %v", err)
		}
	}
	plans, err := st.ListPurgePlans(ctx)
	if err != nil || len(plans) != 0 {
		t.Errorf("policy wrote plans: %d %v", len(plans), err)
	}
	after, err := os.ReadFile(item.QuarantinePath)
	if err != nil || string(before) != string(after) {
		t.Fatal("quarantine bytes changed")
	}
}

func TestPurgePolicyPreservesAmbiguousHistoricalRecovery(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	root := t.TempDir()
	source, q := filepath.Join(root, "source"), filepath.Join(root, "quarantine")
	for _, p := range []string{source, q} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	item := seedQuarantineItem(t, st, source, q, "historical", now.Add(-time.Hour), now.Add(-time.Minute))
	fixture := newPurgeLifecycleFixture(st)
	plans, err := fixture.CreatePlans(ctx)
	if err != nil || len(plans) != 1 {
		t.Fatalf("seed plan: %v", err)
	}
	p := plans[0]
	if err := fixture.ApprovePlan(ctx, p.ID, p.ApprovalDigest); err != nil {
		t.Fatal(err)
	}
	p, err = st.GetPurgePlan(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(q, "ambiguous-stage")
	if err := os.WriteFile(stage, []byte("externally-created"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := st.BeginPurge(ctx, p, item, stage, now); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(item.QuarantinePath)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewPurgeService(st)
	if plans, err := svc.ListPlans(ctx); err != nil || len(plans) != 1 {
		t.Fatal("history not readable")
	}
	for range 2 {
		results, err := svc.RecoverPurges(ctx, q)
		if err != nil || len(results) != 1 || results[0].ErrorType != "ambiguous_recovery_state" {
			t.Fatalf("historical recovery: %v %+v", err, results)
		}
		if err := requireNoRecovery(ctx, st); err == nil {
			t.Fatal("ambiguous lock cleared")
		}
		entries, err := st.ListRecoverablePurges(ctx)
		if err != nil || len(entries) != 1 || entries[0].Status != "pending" {
			t.Fatal("journal changed")
		}
	}
	after, err := os.ReadFile(item.QuarantinePath)
	if err != nil || string(after) != string(before) {
		t.Fatal("quarantine changed")
	}
	got, err := os.ReadFile(stage)
	if err != nil || string(got) != "externally-created" {
		t.Fatal("unknown stage changed")
	}
}

func TestPurgePolicyPrecedesAnyStoreAccess(t *testing.T) {
	svc := NewPurgeService(nil)
	if _, err := svc.CreatePlans(context.Background()); err != ErrPurgeReleaseDisabled {
		t.Fatal("create touched store")
	}
	if err := svc.ApprovePlan(context.Background(), "", ""); err != ErrPurgeReleaseDisabled {
		t.Fatal("approve touched store")
	}
	for _, dry := range []bool{false, true} {
		if _, err := svc.ExecutePurge(context.Background(), PurgeExecuteInput{DryRun: dry}); err != ErrPurgeReleaseDisabled {
			t.Fatal("execute touched store")
		}
	}
}
