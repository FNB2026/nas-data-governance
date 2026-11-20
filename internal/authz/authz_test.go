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
	retainUntil := now.Add(-time.Hour)
	size := int64(1024)
	return Facts{
		Operation:        OpPurge,
		PlanID:           "plan-1",
		PlanState:        stateApproved,
		PlanRevoked:      false,
		ApprovedAt:       ptrTime(now.Add(-3 * time.Hour)),
		ExpectedDigest:   strings.Repeat("a", 64),
		DryRunDigest:     strings.Repeat("a", 64),
		DryRunVerifiedAt: ptrTime(now.Add(-2 * time.Hour)),
		RetainUntil:      retainUntil,
		// Plan-side expectations: what the approval was granted against.
		ExpectedObjectID: "obj-1",
		ExpectedSHA256:   strings.Repeat("b", 64),
		ExpectedSize:     &size,
		ExpectedPath:     "/q/obj-1",
		// Item-side durable state, consistent with the plan snapshot.
		ItemStatus:      string(domain.QuarantinePurgeEligible),
		ItemHoldReason:  "",
		ItemSHA256:      strings.Repeat("b", 64),
		ItemSize:        1024,
		ItemPath:        "/q/obj-1",
		ItemRetainUntil: retainUntil,
		SourcePath:      "/src/obj-1",
		// Approval corroboration: two durable authorization records.
		ApprovalCorroboration: 2,
		RequiredCorroboration: 2,
		RecoveryLockHeld:      false,
	}
}

func baseReq() Request {
	return Request{
		Operation:       OpPurge,
		PlanID:          "plan-1",
		PresentedDigest: strings.Repeat("a", 64),
		ObjectID:        "obj-1",
		ContentSHA256:   strings.Repeat("b", 64),
		Size:            Int64Field(1024),
		Device:          Uint64Field(1),
		Inode:           Uint64Field(2),
		SourcePeer:      "uid=1000",
		IPCFields:       map[string]string{"op": "PURGE"},
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

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

// TestVerifier_ZeroByteObject: a zero-byte object is a first-class citizen —
// the size dimension is verified (0 matches only 0), never skipped.
func TestVerifier_ZeroByteObject(t *testing.T) {
	v := NewVerifier()
	f := baseFacts()
	zero := int64(0)
	emptySHA := strings.Repeat("e", 64)
	f.ExpectedSize = &zero
	f.ItemSize = 0
	f.ItemSHA256 = emptySHA
	f.ExpectedSHA256 = emptySHA
	src := &staticFacts{facts: f}

	req := baseReq()
	req.ContentSHA256 = emptySHA
	req.Size = Int64Field(0)
	el, err := v.Check(context.Background(), src, req)
	if err != nil {
		t.Fatal(err)
	}
	if !el.Eligible {
		t.Fatalf("zero-byte object must be eligible, got %s: %s", el.Reason, el.Detail)
	}

	// A non-zero claim against a zero-byte object is a real mismatch.
	req.Size = Int64Field(1)
	el, err = v.Check(context.Background(), src, req)
	if err != nil {
		t.Fatal(err)
	}
	if el.Eligible || el.Reason != ReasonWrongObject {
		t.Fatalf("expected wrong_object for size 1 vs zero-byte object, got %s", el.Reason)
	}
}

// TestVerifier_PeerAndDigestAloneNotSufficient: a kernel-attested peer
// (SO_PEERCRED), complete IPC fields, and a well-formed SHA-256 digest
// together are NOT sufficient when the plan is not an approved authorization.
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
// exactly one binding dimension at a time. Missing fields and zero-valued
// dimensions are refused, never skipped.
func TestVerifier_Negative(t *testing.T) {
	otherSHA := strings.Repeat("d", 64)
	future := time.Now().UTC().Add(time.Hour)
	cases := []struct {
		name string
		mut  func(f *Facts, r *Request)
		want Reason
	}{
		// --- environment ---
		{"recovery_lock", func(f *Facts, r *Request) { f.RecoveryLockHeld = true }, ReasonRecoveryLock},
		// --- plan identity + operation binding ---
		{"plan_id_mismatch", func(f *Facts, r *Request) { r.PlanID = "plan-other" }, ReasonWrongPlan},
		{"fact_plan_empty", func(f *Facts, r *Request) { f.PlanID = "" }, ReasonWrongPlan},
		{"operation_mismatch", func(f *Facts, r *Request) { r.Operation = OpSeal }, ReasonWrongPlan},
		{"fact_operation_mismatch", func(f *Facts, r *Request) { f.Operation = OpRestore }, ReasonWrongPlan},
		// --- approval state ---
		{"expired", func(f *Facts, r *Request) { f.RetainUntil = future; f.ItemRetainUntil = future }, ReasonExpired},
		{"revoked", func(f *Facts, r *Request) { f.PlanState = string(domain.PurgeRolledBack); f.PlanRevoked = true }, ReasonRevoked},
		{"wrong_plan_state", func(f *Facts, r *Request) { f.PlanState = "DRAFT" }, ReasonWrongPlan},
		// --- approval record integrity / timing ---
		{"missing_approval_time", func(f *Facts, r *Request) { f.ApprovedAt = nil }, ReasonTampered},
		{"future_approval_time", func(f *Facts, r *Request) { f.ApprovedAt = ptrTime(future) }, ReasonTampered},
		// --- item durable state ---
		{"hold_item", func(f *Facts, r *Request) {
			f.ItemStatus = string(domain.QuarantineHold)
			f.ItemHoldReason = "protected_context"
		}, ReasonWrongObject},
		{"restored_item", func(f *Facts, r *Request) { f.ItemStatus = string(domain.QuarantineRestored) }, ReasonWrongObject},
		{"purged_item", func(f *Facts, r *Request) { f.ItemStatus = string(domain.QuarantinePurged) }, ReasonWrongObject},
		{"active_item", func(f *Facts, r *Request) { f.ItemStatus = string(domain.QuarantineActive) }, ReasonWrongObject},
		{"missing_item", func(f *Facts, r *Request) { f.ItemStatus = ""; f.SourcePath = "" }, ReasonWrongObject},
		// --- plan↔item binding ---
		{"item_sha_drift", func(f *Facts, r *Request) { f.ItemSHA256 = otherSHA }, ReasonWrongObject},
		{"item_size_drift", func(f *Facts, r *Request) { f.ItemSize = 2048 }, ReasonWrongObject},
		{"item_path_drift", func(f *Facts, r *Request) { f.ItemPath = "/q/elsewhere" }, ReasonWrongObject},
		{"item_retention_drift", func(f *Facts, r *Request) { f.ItemRetainUntil = future }, ReasonWrongObject},
		{"item_missing_source", func(f *Facts, r *Request) { f.SourcePath = "" }, ReasonWrongObject},
		// --- facts-side identity completeness ---
		{"facts_missing_size", func(f *Facts, r *Request) { f.ExpectedSize = nil }, ReasonWrongObject},
		{"facts_missing_path", func(f *Facts, r *Request) { f.ExpectedPath = "" }, ReasonWrongObject},
		{"facts_missing_object", func(f *Facts, r *Request) { f.ExpectedObjectID = "" }, ReasonWrongObject},
		{"facts_missing_sha", func(f *Facts, r *Request) { f.ExpectedSHA256 = "" }, ReasonWrongObject},
		// --- request completeness: omission refused, never skipped ---
		{"request_missing_size", func(f *Facts, r *Request) { r.Size = nil }, ReasonWrongObject},
		{"request_missing_object", func(f *Facts, r *Request) { r.ObjectID = "" }, ReasonWrongObject},
		{"request_missing_sha", func(f *Facts, r *Request) { r.ContentSHA256 = "" }, ReasonWrongObject},
		{"purge_missing_device", func(f *Facts, r *Request) { r.Device = nil }, ReasonWrongObject},
		{"purge_missing_inode", func(f *Facts, r *Request) { r.Inode = nil }, ReasonWrongObject},
		// --- request↔facts identity ---
		{"wrong_object_id", func(f *Facts, r *Request) { r.ObjectID = "obj-X" }, ReasonWrongObject},
		{"wrong_sha", func(f *Facts, r *Request) { r.ContentSHA256 = otherSHA }, ReasonWrongObject},
		{"wrong_size", func(f *Facts, r *Request) { r.Size = Int64Field(999) }, ReasonWrongObject},
		// --- digest binding ---
		{"tampered_digest", func(f *Facts, r *Request) { r.PresentedDigest = strings.Repeat("c", 64) }, ReasonTampered},
		{"malformed_digest", func(f *Facts, r *Request) { r.PresentedDigest = "deadbeef" }, ReasonTampered},
		// --- approval corroboration / dry-run integrity ---
		{"insufficient_corroboration", func(f *Facts, r *Request) { f.ApprovalCorroboration = 1 }, ReasonTampered},
		{"weak_required_corroboration", func(f *Facts, r *Request) {
			f.RequiredCorroboration = 1
			f.ApprovalCorroboration = 1
		}, ReasonTampered},
		{"dryrun_digest_mismatch", func(f *Facts, r *Request) { f.DryRunDigest = strings.Repeat("f", 64) }, ReasonTampered},
		{"dryrun_digest_malformed", func(f *Facts, r *Request) { f.DryRunDigest = "zz" }, ReasonTampered},
		{"dryrun_missing_record", func(f *Facts, r *Request) { f.DryRunVerifiedAt = nil; f.DryRunDigest = "" }, ReasonTampered},
		{"dryrun_before_approval", func(f *Facts, r *Request) {
			f.DryRunVerifiedAt = ptrTime(f.ApprovedAt.Add(-time.Minute))
		}, ReasonTampered},
		{"dryrun_in_future", func(f *Facts, r *Request) { f.DryRunVerifiedAt = ptrTime(future) }, ReasonTampered},
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
func seedQuarantineItem(t *testing.T, st *store.SQLiteStore, sourceRoot, quarantineRoot, name, content string, quarantinedAt, retainUntil time.Time) domain.QuarantineItem {
	t.Helper()
	ctx := context.Background()
	qPath := filepath.Join(quarantineRoot, name)
	if err := os.WriteFile(qPath, []byte(content), 0o600); err != nil {
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
// a freshly seeded quarantine item. planRetain must equal the item's retention
// (the executor and the verifier both bind plan↔item retention); pass
// planRetainLater to construct the deliberate retention-drift negative.
func approvedPurge(t *testing.T, ctx context.Context, st *store.SQLiteStore, sourceRoot, quarantineRoot, name string, itemRetain, planCreated, planRetain time.Time, withDryRun bool) (domain.QuarantineItem, domain.PurgePlan, string, Request) {
	t.Helper()
	item := seedQuarantineItem(t, st, sourceRoot, quarantineRoot, name, "test-"+name, time.Now().UTC().Add(-48*time.Hour), itemRetain)
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
		Size:            Int64Field(item.FileSize),
		Device:          Uint64Field(1),
		Inode:           Uint64Field(2),
	}
	return item, plan, digest, req
}

// TestStoreFacts_SyntheticLifecycle validates the contract in an isolated
// synthetic-data environment: positive eligibility, idempotency, restart
// durability, concurrency, own-lock re-verification, every rejection reason
// driven by the real store, store-failure refusal, an inaccessible lock file,
// and authorization-state changes.
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
	v := NewVerifier()

	now := time.Now().UTC()
	elapsed := now.Add(-24 * time.Hour) // item/plan retention, already elapsed

	// --- eligible scenario (retention elapsed, dry-run verified) ---
	item, plan, _, req := approvedPurge(t, ctx, st, sourceRoot, quarantineRoot, "alpha",
		elapsed, now.Add(-12*time.Hour), elapsed, true)
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

	// Concurrency vs recovery lock: a competing writer's execution lock blocks
	// eligibility.
	release, e := st2.AcquireExecutionLock()
	if e != nil {
		t.Fatal(e)
	}
	el, _ = v.Check(ctx, src2, req)
	if el.Eligible {
		t.Fatal("a competing writer's execution lock must block eligibility")
	}
	if el.Reason != ReasonRecoveryLock {
		t.Fatalf("expected recovery_lock, got %s", el.Reason)
	}
	release()
	el, _ = v.Check(ctx, src2, req)
	if !el.Eligible {
		t.Fatalf("after releasing the lock, expected eligible, got %s", el.Reason)
	}

	// Re-verification under our own lock (the chained contract's second gate):
	// our own reservation is not a competing writer's. Ownership comes from
	// the real scope, never a caller-supplied predicate.
	scopeOwn, e := AcquireExecutionLockScope(st2)
	if e != nil {
		t.Fatal(e)
	}
	el, _ = v.Check(ctx, NewStoreFactsWithScope(st2, scopeOwn), req)
	scopeOwn.Release()
	if !el.Eligible {
		t.Fatalf("our own lock reservation must not block re-verification, got %s", el.Reason)
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
	_, _, _, reqFresh := approvedPurge(t, ctx, st2, sourceRoot, quarantineRoot, "beta",
		elapsed, now.Add(-12*time.Hour), elapsed, true)
	reqWrongObj := reqFresh
	reqWrongObj.ObjectID = "obj-impostor"
	el, _ = v.Check(ctx, src2, reqWrongObj)
	if el.Eligible || el.Reason != ReasonWrongObject {
		t.Fatalf("expected wrong_object, got eligible=%v reason=%s", el.Eligible, el.Reason)
	}

	// --- expired: retention window not elapsed ---
	_, _, _, reqExp := approvedPurge(t, ctx, st2, sourceRoot, quarantineRoot, "gamma",
		now.Add(2*time.Hour), now.Add(3*time.Hour), now.Add(2*time.Hour), true)
	el, _ = v.Check(ctx, src2, reqExp)
	if el.Eligible || el.Reason != ReasonExpired {
		t.Fatalf("expected expired, got eligible=%v reason=%s", el.Eligible, el.Reason)
	}

	// --- tampered: approval not independently corroborated (no dry-run) ---
	_, _, _, reqNoDry := approvedPurge(t, ctx, st2, sourceRoot, quarantineRoot, "delta",
		elapsed, now.Add(-12*time.Hour), elapsed, false)
	el, _ = v.Check(ctx, src2, reqNoDry)
	if el.Eligible || el.Reason != ReasonTampered {
		t.Fatalf("expected tampered (insufficient corroboration), got eligible=%v reason=%s", el.Eligible, el.Reason)
	}

	// --- plan↔item retention drift: the plan snapshot drifted from its item ---
	_, _, _, reqDrift := approvedPurge(t, ctx, st2, sourceRoot, quarantineRoot, "zeta",
		elapsed, now.Add(-12*time.Hour), elapsed.Add(23*time.Hour), true)
	el, _ = v.Check(ctx, src2, reqDrift)
	if el.Eligible || el.Reason != ReasonWrongObject {
		t.Fatalf("expected wrong_object (retention drift), got eligible=%v reason=%s", el.Eligible, el.Reason)
	}

	// --- recovery lock via pending journal (recovered purge) ---
	itemR, planR, _, reqR := approvedPurge(t, ctx, st2, sourceRoot, quarantineRoot, "epsilon",
		elapsed, now.Add(-12*time.Hour), elapsed, true)
	stagingPath := filepath.Join(dir, "staging-epsilon")
	if err := st2.BeginPurge(ctx, planR, itemR, stagingPath, now); err != nil {
		t.Fatal(err)
	}
	el, _ = v.Check(ctx, src2, reqR)
	if el.Eligible || el.Reason != ReasonRecoveryLock {
		t.Fatalf("expected recovery_lock (pending journal), got eligible=%v reason=%s", el.Eligible, el.Reason)
	}

	// --- store failure must refuse, never pass silently ---
	st4, err := store.Open(ctx, filepath.Join(dir, "gov3.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, req4 := approvedPurge(t, ctx, st4, sourceRoot, quarantineRoot, "theta",
		elapsed, now.Add(-12*time.Hour), elapsed, true)
	if err := st4.Close(); err != nil {
		t.Fatal(err)
	}
	el, err = v.Check(ctx, NewStoreFacts(st4), req4)
	if err == nil {
		t.Fatal("a failed fact-source read must return an error, never a silent verdict")
	}
	if el.Eligible {
		t.Fatal("a failed fact-source read must never be eligible")
	}

	// --- inaccessible lock file must refuse, never read as free ---
	dbPath2 := filepath.Join(dir, "gov2.db")
	st3, err := store.Open(ctx, dbPath2)
	if err != nil {
		t.Fatal(err)
	}
	defer st3.Close()
	_, _, _, req3 := approvedPurge(t, ctx, st3, sourceRoot, quarantineRoot, "eta",
		elapsed, now.Add(-12*time.Hour), elapsed, true)
	// A directory at the lock path makes the probe's state unknown (EISDIR).
	if err := os.Mkdir(dbPath2+".execution.lock", 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := st3.ExecutionLockHeld(); err == nil {
		t.Fatal("an inaccessible lock file must be reported as unknown, not free")
	}
	el, err = v.Check(ctx, NewStoreFacts(st3), req3)
	if err == nil {
		t.Fatal("unknown lock state must surface as a refusal error")
	}
	if el.Eligible {
		t.Fatal("unknown lock state must never be eligible")
	}

	// --- HOLD items can never enter the purge pipeline at all ---
	protectedRoot := filepath.Join(dir, "quarantine-protected")
	if err := os.MkdirAll(protectedRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	itemH := seedProtectedItem(t, st3, sourceRoot, protectedRoot, "hold-me", now.Add(-24*time.Hour), now.Add(-time.Hour))
	if itemH.Status != domain.QuarantineHold {
		t.Fatalf("expected HOLD item, got %s", itemH.Status)
	}
	err = st3.SavePurgePlans(ctx, []domain.PurgePlan{{
		ID: "purge-hold", ItemID: itemH.ID, State: domain.PurgeDraft,
		ExpectedPath:   itemH.QuarantinePath,
		ExpectedSHA256: itemH.ContentSHA256, ExpectedSize: itemH.FileSize,
		RetainUntil: itemH.RetainUntil, ApprovalDigest: strings.Repeat("a", 64),
		CreatedAt: now.Add(-12 * time.Hour),
	}})
	if err == nil {
		t.Fatal("a HOLD item must never become purge eligible")
	}
}

// seedProtectedItem seeds a quarantine item from a protected context, which
// lands in HOLD and can never become a purge candidate.
func seedProtectedItem(t *testing.T, st *store.SQLiteStore, sourceRoot, quarantineRoot, name string, quarantinedAt, retainUntil time.Time) domain.QuarantineItem {
	t.Helper()
	ctx := context.Background()
	qPath := filepath.Join(quarantineRoot, name)
	if err := os.WriteFile(qPath, []byte("protected-"+name), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := executor.Snapshot(qPath, true)
	if err != nil {
		t.Fatal(err)
	}
	taskID := "task-hold-" + name
	planID := "plan-hold-" + name
	sourcePath := filepath.Join(sourceRoot, name)
	if err := st.CreateTask(ctx, domain.OperationTask{ID: taskID, RootPath: sourceRoot, State: "completed", CreatedAt: quarantinedAt}); err != nil {
		t.Fatal(err)
	}
	action := domain.PlannedAction{
		Path:    sourcePath,
		Action:  domain.OperationQuarantine,
		Context: domain.DirectoryContext{Role: domain.RoleSensitive},
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
	t.Fatalf("seeded protected item not found for plan %s", planID)
	return domain.QuarantineItem{}
}
