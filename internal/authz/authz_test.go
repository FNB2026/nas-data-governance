package authz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// --- static Facts source for pure unit tests ---

type staticFacts struct {
	facts Facts
}

func (s *staticFacts) ReadOnlyFacts(ctx context.Context, op OpKind, planID string) (Facts, error) {
	return s.facts, nil
}

func baseFacts() Facts {
	now := time.Now().UTC()
	return Facts{
		PlanID:           "plan-1",
		PlanState:        stateApproved,
		PlanRevoked:      false,
		ExpectedDigest:   strings.Repeat("a", 64),
		DryRunDigest:     strings.Repeat("a", 64),
		DryRunVerifiedAt: &now,
		RetainUntil:      now.Add(-time.Hour),
		ExpectedObjectID: "obj-1",
		ExpectedSHA256:   strings.Repeat("b", 64),
		ExpectedSize:     1024,
		ExpectedPath:     "/q/obj-1",
		ValidCopies:      2,
		RequiredCopies:   2,
		RecoveryLockHeld: false,
		SourcePath:       "/src/obj-1",
	}
}

func baseReq() Request {
	return Request{
		Operation:       OpPurge,
		PlanID:          "plan-1",
		PresentedDigest: strings.Repeat("a", 64),
		ObjectID:        "obj-1",
		ContentSHA256:   strings.Repeat("b", 64),
		Size:            1024,
		MountPath:       "/q/obj-1",
		SourcePeer:      "uid=1000",
		IPCFields:       map[string]string{"op": "PURGE"},
	}
}

// TestVerifier_Positive: a fully bound, approved, elapsed, corroborated,
// matching request is eligible.
func TestVerifier_Positive(t *testing.T) {
	v := NewVerifier()
	src := &staticFacts{facts: baseFacts()}
	el, err := v.Check(context.Background(), src, baseReq())
	if err != nil {
		t.Fatal(err)
	}
	if !el.Eligible || el.Reason != ReasonEligible {
		t.Fatalf("expected eligible, got %s: %s", el.Reason, el.Detail)
	}
}

// TestVerifier_PeerAndDigestAloneNotSufficient proves requirement #3: a
// kernel-attested peer (SO_PEERCRED), complete IPC fields, and a well-formed
// SHA-256 digest together are NOT sufficient when the plan is not an approved
// authorization.
func TestVerifier_PeerAndDigestAloneNotSufficient(t *testing.T) {
	v := NewVerifier()
	f := baseFacts()
	f.PlanState = "DRAFT" // not approved
	src := &staticFacts{facts: f}
	req := baseReq()
	req.SourcePeer = "uid=0" // even root over a kernel-attested peer
	req.IPCFields = map[string]string{
		"op":                   "PURGE",
		"object_id":            req.ObjectID,
		"size":                 "1024",
		"sha256":               req.ContentSHA256,
		"authorization_sha256": req.PresentedDigest,
	}
	el, err := v.Check(context.Background(), src, req)
	if err != nil {
		t.Fatal(err)
	}
	if el.Eligible {
		t.Fatal("peer + full IPC fields + well-formed digest must NOT authorize an unapproved plan")
	}
	if el.Reason != ReasonWrongPlan {
		t.Fatalf("expected wrong_plan, got %s", el.Reason)
	}
}

// TestVerifier_Negative exercises every rejection reason in isolation, mutating
// exactly one binding dimension at a time.
func TestVerifier_Negative(t *testing.T) {
	cases := []struct {
		name string
		mut  func(f *Facts, r *Request)
		want Reason
	}{
		{"expired", func(f *Facts, r *Request) { f.RetainUntil = time.Now().UTC().Add(time.Hour) }, ReasonExpired},
		{"revoked", func(f *Facts, r *Request) { f.PlanState = string(domain.PurgeRolledBack); f.PlanRevoked = true }, ReasonRevoked},
		{"wrong_plan_state", func(f *Facts, r *Request) { f.PlanState = "DRAFT" }, ReasonWrongPlan},
		{"unknown_plan", func(f *Facts, r *Request) { f.PlanID = ""; f.PlanState = "" }, ReasonWrongPlan},
		{"tampered_digest", func(f *Facts, r *Request) { r.PresentedDigest = strings.Repeat("c", 64) }, ReasonTampered},
		{"malformed_digest", func(f *Facts, r *Request) { r.PresentedDigest = "deadbeef" }, ReasonTampered},
		{"insufficient_copies", func(f *Facts, r *Request) { f.ValidCopies = 1 }, ReasonTampered},
		{"wrong_object_id", func(f *Facts, r *Request) { r.ObjectID = "obj-X" }, ReasonWrongObject},
		{"wrong_sha", func(f *Facts, r *Request) { r.ContentSHA256 = strings.Repeat("d", 64) }, ReasonWrongObject},
		{"wrong_size", func(f *Facts, r *Request) { r.Size = 999 }, ReasonWrongObject},
		{"recovery_lock", func(f *Facts, r *Request) { f.RecoveryLockHeld = true }, ReasonRecoveryLock},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := NewVerifier()
			f := baseFacts()
			r := baseReq()
			c.mut(&f, &r)
			src := &staticFacts{facts: f}
			el, err := v.Check(context.Background(), src, r)
			if err != nil {
				t.Fatal(err)
			}
			if el.Eligible {
				t.Fatalf("expected ineligible, got eligible")
			}
			if el.Reason != c.want {
				t.Fatalf("want %s, got %s (%s)", c.want, el.Reason, el.Detail)
			}
		})
	}
}

// --- synthetic-data integration against the real store ---

// seedQuarantineItem materializes one managed quarantine item by replaying the
// execution pipeline (create task -> save plan -> journal -> complete).
func seedQuarantineItem(t *testing.T, st *store.SQLiteStore, sourceRoot, quarantineRoot, name string, quarantinedAt, retainUntil time.Time) domain.QuarantineItem {
	t.Helper()
	ctx := context.Background()
	qPath := filepath.Join(quarantineRoot, name)
	if err := os.WriteFile(qPath, []byte("test-"+name), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := executor.Snapshot(qPath, true)
	if err != nil {
		t.Fatal(err)
	}
	taskID := "task-" + name
	planID := "plan-" + name
	sourcePath := filepath.Join(sourceRoot, name)
	if err := st.CreateTask(ctx, domain.OperationTask{ID: taskID, RootPath: sourceRoot, State: "completed", CreatedAt: quarantinedAt}); err != nil {
		t.Fatal(err)
	}
	action := domain.PlannedAction{
		Path:    sourcePath,
		Action:  domain.OperationQuarantine,
		Context: domain.DirectoryContext{Role: domain.RoleTemporary},
		File:    domain.FileInstance{Path: sourcePath, Size: snapshot.Size, ContentSHA256: snapshot.Hash},
	}
	plan := domain.OperationPlan{ID: planID, TaskID: taskID, State: domain.PlanExecuting, Actions: []domain.PlannedAction{action}}
	if err := st.SavePlans(ctx, taskID, []domain.OperationPlan{plan}); err != nil {
		t.Fatal(err)
	}
	if err := st.BeginJournal(ctx, taskID, planID, plan.Actions); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkJournalDone(ctx, planID, 0, qPath); err != nil {
		t.Fatal(err)
	}
	if err := st.CompletePlanExecution(ctx, plan, domain.PlanVerified, nil, quarantinedAt, retainUntil); err != nil {
		t.Fatal(err)
	}
	all, err := st.ListQuarantineItems(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range all {
		if it.PlanID == planID {
			return it
		}
	}
	t.Fatalf("seeded item not found for plan %s", planID)
	return domain.QuarantineItem{}
}

// approvedPurge builds an APPROVED purge plan (optionally dry-run verified) for
// a freshly seeded quarantine item, and returns the durable objects plus a
// matching Request.
func approvedPurge(t *testing.T, ctx context.Context, st *store.SQLiteStore, sourceRoot, quarantineRoot, name string, itemRetain, planCreated, planRetain time.Time, withDryRun bool) (domain.QuarantineItem, domain.PurgePlan, string, Request) {
	t.Helper()
	item := seedQuarantineItem(t, st, sourceRoot, quarantineRoot, name, time.Now().UTC().Add(-48*time.Hour), itemRetain)
	digest := strings.Repeat("a", 64)
	planID := "purge-" + name
	plan := domain.PurgePlan{
		ID:             planID,
		ItemID:         item.ID,
		State:          domain.PurgeDraft,
		ExpectedPath:   item.QuarantinePath,
		ExpectedSHA256: item.ContentSHA256,
		ExpectedSize:   item.FileSize,
		RetainUntil:    planRetain,
		ApprovalDigest: digest,
		CreatedAt:      planCreated,
	}
	if err := st.SavePurgePlans(ctx, []domain.PurgePlan{plan}); err != nil {
		t.Fatal(err)
	}
	if err := st.ApprovePurgePlan(ctx, planID, digest, time.Now().UTC().Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if withDryRun {
		if err := st.MarkPurgeDryRunVerified(ctx, planID, digest, time.Now().UTC().Add(-20*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	req := Request{
		Operation:       OpPurge,
		PlanID:          planID,
		PresentedDigest: digest,
		ObjectID:        item.ID,
		ContentSHA256:   item.ContentSHA256,
		Size:            item.FileSize,
		MountPath:       item.QuarantinePath,
	}
	return item, plan, digest, req
}

// TestStoreFacts_SyntheticLifecycle validates the contract in an isolated
// synthetic-data environment: positive eligibility, idempotency, restart
// durability, concurrency, and every rejection reason driven by the real store,
// plus an authorization-state-change (revoke) transition.
func TestStoreFacts_SyntheticLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("synthetic lifecycle uses a real store")
	}
	ctx := context.Background()
	dir := t.TempDir()
	sourceRoot := filepath.Join(dir, "source")
	quarantineRoot := filepath.Join(dir, "quarantine")
	for _, d := range []string{sourceRoot, quarantineRoot} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(ctx, filepath.Join(dir, "gov.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	v := NewVerifier()

	now := time.Now().UTC()

	// --- eligible scenario (retention elapsed, dry-run verified) ---
	item, plan, _, req := approvedPurge(t, ctx, st, sourceRoot, quarantineRoot, "alpha",
		now.Add(-24*time.Hour), now.Add(-12*time.Hour), now.Add(-time.Hour), true)
	src := NewStoreFacts(st)

	// Idempotency: repeated checks are stable and read-only.
	for i := 0; i < 5; i++ {
		el, e := v.Check(ctx, src, req)
		if e != nil {
			t.Fatal(e)
		}
		if !el.Eligible {
			t.Fatalf("iter %d: expected eligible, got %s: %s", i, el.Reason, el.Detail)
		}
	}

	// Restart: durable truth survives a store reopen.
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st2, err := store.Open(ctx, filepath.Join(dir, "gov.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	src2 := NewStoreFacts(st2)
	el, e := v.Check(ctx, src2, req)
	if e != nil || !el.Eligible {
		t.Fatalf("after restart: expected eligible, got %v err=%v", el, e)
	}

	// Concurrency: 32 concurrent readers all agree on eligibility.
	var wg sync.WaitGroup
	errCh := make(chan error, 64)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev, evErr := v.Check(ctx, src2, req)
			if evErr != nil {
				errCh <- evErr
				return
			}
			if !ev.Eligible {
				errCh <- fmt.Errorf("concurrent check not eligible: %s", ev.Reason)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for ev := range errCh {
		t.Fatal(ev)
	}

	// Concurrency vs recovery lock: an active execution lock blocks eligibility.
	release, e := st2.AcquireExecutionLock()
	if e != nil {
		t.Fatal(e)
	}
	el, _ = v.Check(ctx, src2, req)
	if el.Eligible {
		t.Fatal("active execution lock must block eligibility")
	}
	if el.Reason != ReasonRecoveryLock {
		t.Fatalf("expected recovery_lock, got %s", el.Reason)
	}
	release()
	el, _ = v.Check(ctx, src2, req)
	if !el.Eligible {
		t.Fatalf("after releasing the lock, expected eligible, got %s", el.Reason)
	}

	// Auth-state-change: rolling the plan back must void eligibility.
	if err := st2.MarkPurgeRolledBack(ctx, plan.ID, item.ID, now); err != nil {
		t.Fatal(err)
	}
	el, _ = v.Check(ctx, src2, req)
	if el.Eligible {
		t.Fatal("rolled-back plan must not be eligible")
	}
	if el.Reason != ReasonRevoked {
		t.Fatalf("expected revoked, got %s", el.Reason)
	}

	// --- wrong plan: unknown plan id ---
	reqBad := req
	reqBad.PlanID = "plan-does-not-exist"
	el, _ = v.Check(ctx, src2, reqBad)
	if el.Eligible || el.Reason != ReasonWrongPlan {
		t.Fatalf("expected wrong_plan for unknown plan, got eligible=%v reason=%s", el.Eligible, el.Reason)
	}

	// --- wrong object: a correct digest but a mismatched object id ---
	reqWrongObj := req
	reqWrongObj.ObjectID = "obj-impostor"
	// Re-approve a fresh scenario so the plan state is valid for this check.
	_, _, _, reqFresh := approvedPurge(t, ctx, st2, sourceRoot, quarantineRoot, "beta",
		now.Add(-24*time.Hour), now.Add(-12*time.Hour), now.Add(-time.Hour), true)
	reqWrongObj.PlanID = reqFresh.PlanID
	reqWrongObj.PresentedDigest = reqFresh.PresentedDigest
	reqWrongObj.ContentSHA256 = reqFresh.ContentSHA256
	reqWrongObj.Size = reqFresh.Size
	reqWrongObj.MountPath = reqFresh.MountPath
	el, _ = v.Check(ctx, src2, reqWrongObj)
	if el.Eligible || el.Reason != ReasonWrongObject {
		t.Fatalf("expected wrong_object, got eligible=%v reason=%s", el.Eligible, el.Reason)
	}

	// --- expired: retention window not elapsed ---
	_, _, _, reqExp := approvedPurge(t, ctx, st2, sourceRoot, quarantineRoot, "gamma",
		now.Add(2*time.Hour), now.Add(3*time.Hour), now.Add(1*time.Hour), true)
	el, _ = v.Check(ctx, src2, reqExp)
	if el.Eligible || el.Reason != ReasonExpired {
		t.Fatalf("expected expired, got eligible=%v reason=%s", el.Eligible, el.Reason)
	}

	// --- tampered: approval not independently corroborated (no dry-run) ---
	_, _, _, reqNoDry := approvedPurge(t, ctx, st2, sourceRoot, quarantineRoot, "delta",
		now.Add(-24*time.Hour), now.Add(-12*time.Hour), now.Add(-time.Hour), false)
	el, _ = v.Check(ctx, src2, reqNoDry)
	if el.Eligible || el.Reason != ReasonTampered {
		t.Fatalf("expected tampered (insufficient copies), got eligible=%v reason=%s", el.Eligible, el.Reason)
	}

	// --- recovery lock via pending journal (recovered purge) ---
	itemR, planR, _, reqR := approvedPurge(t, ctx, st2, sourceRoot, quarantineRoot, "epsilon",
		now.Add(-24*time.Hour), now.Add(-12*time.Hour), now.Add(-time.Hour), true)
	stagingPath := filepath.Join(dir, "staging-epsilon")
	if err := st2.BeginPurge(ctx, planR, itemR, stagingPath, now); err != nil {
		t.Fatal(err)
	}
	el, _ = v.Check(ctx, src2, reqR)
	if el.Eligible || el.Reason != ReasonRecoveryLock {
		t.Fatalf("expected recovery_lock (pending journal), got eligible=%v reason=%s", el.Eligible, el.Reason)
	}
}
