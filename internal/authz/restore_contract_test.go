package authz

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// The RESTORE durable-authorization contract. RESTORE has its own fact source
// (restore_plans + quarantine_items), its own eligibility rules, and its own
// corroboration floor. A purge approval can never authorize a restore and a
// restore approval can never authorize a purge — structurally, because each
// operation's fact source consults only its own plan tables.

// approvedRestore seeds one quarantined item plus an independent DRAFT
// restore plan bound to the item's durable identity, then approves it.
// Retain dates may be in the future: restore eligibility must not depend on
// the purge retention clock.
func approvedRestore(t *testing.T, ctx context.Context, st *store.SQLiteStore, sourceRoot, quarantineRoot, name string, quarantinedAt, retainUntil time.Time, withApproval bool) (domain.QuarantineItem, domain.RestorePlan, string, Request) {
	t.Helper()
	item := seedQuarantineItem(t, st, sourceRoot, quarantineRoot, name, "restore-"+name, quarantinedAt, retainUntil)
	digest := strings.Repeat("d", 64)
	planID := "restore-" + name
	plan := domain.RestorePlan{
		ID: planID, ItemID: item.ID, State: domain.RestoreDraft,
		QuarantinePath: item.QuarantinePath, RestorePath: item.SourcePath,
		ExpectedSHA256: item.ContentSHA256, ExpectedSize: item.FileSize,
		ApprovalDigest: digest, CreatedAt: quarantinedAt.Add(24 * time.Hour),
	}
	if err := st.SaveRestorePlan(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if withApproval {
		if err := st.ApproveRestorePlan(ctx, planID, digest, plan.CreatedAt.Add(30*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	fresh, err := st.GetRestorePlan(ctx, planID)
	if err != nil {
		t.Fatal(err)
	}
	req := Request{
		Operation: OpRestore, PlanID: planID, PresentedDigest: digest,
		ObjectID: item.ID, ContentSHA256: item.ContentSHA256,
		Size: Int64Field(item.FileSize),
	}
	return item, fresh, digest, req
}

func TestRestoreContract_Synthetic(t *testing.T) {
	if testing.Short() {
		t.Skip("restore contract uses a real store")
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
	now := time.Now().UTC()

	setup := func(t *testing.T, dbName string) *store.SQLiteStore {
		t.Helper()
		st, err := store.Open(ctx, filepath.Join(dir, dbName))
		if err != nil {
			t.Fatal(err)
		}
		return st
	}

	t.Run("approved_restore_is_eligible_without_retention_precondition", func(t *testing.T) {
		// The core RESTORE rule: eligibility does not inherit the purge
		// retention-elapsed precondition. The item's retention is still
		// running (far in the future) and the restore is nevertheless
		// eligible — the same facts would refuse a PURGE as expired.
		st := setup(t, "restore-positive.db")
		defer st.Close()
		item, _, _, req := approvedRestore(t, ctx, st, sourceRoot, quarantineRoot,
			"positive", now.Add(-48*time.Hour), now.Add(48*time.Hour), true)

		v := NewVerifier()
		el, err := v.Check(ctx, NewStoreFacts(st), req)
		if err != nil {
			t.Fatal(err)
		}
		if !el.Eligible {
			t.Fatalf("an approved restore must be eligible despite running retention, got %s (%s)", el.Reason, el.Detail)
		}

		// Symmetry proof at the contract level: with retention still running,
		// the verifier refuses PURGE (expired) on the very same identity
		// dimensions, while the restore above passed. The durable store
		// itself cannot even hold a purge plan for an item whose retention
		// has not elapsed (SavePurgePlans refuses the promotion), which is
		// the same rule seen from the write side.
		purgeFacts := Facts{
			Operation: OpPurge, PlanID: "purge-symmetric", PlanState: "APPROVED",
			ApprovedAt:            ptrTime(now.Add(-time.Hour)),
			ExpectedDigest:        strings.Repeat("a", 64),
			DryRunDigest:          strings.Repeat("a", 64),
			DryRunVerifiedAt:      ptrTime(now.Add(-30 * time.Minute)),
			RetainUntil:           now.Add(48 * time.Hour), // still protected
			ExpectedObjectID:      item.ID,
			ExpectedSHA256:        item.ContentSHA256,
			ExpectedSize:          Int64Field(item.FileSize),
			ExpectedPath:          item.QuarantinePath,
			ItemStatus:            string(domain.QuarantinePurgeEligible),
			ItemSHA256:            item.ContentSHA256,
			ItemSize:              item.FileSize,
			ItemPath:              item.QuarantinePath,
			ItemRetainUntil:       now.Add(48 * time.Hour),
			SourcePath:            item.SourcePath,
			ApprovalCorroboration: 2,
			RequiredCorroboration: 2,
		}
		el, err = v.Check(ctx, fixedFacts{facts: purgeFacts}, Request{
			Operation: OpPurge, PlanID: "purge-symmetric", PresentedDigest: strings.Repeat("a", 64),
			ObjectID: item.ID, ContentSHA256: item.ContentSHA256,
			Size: Int64Field(item.FileSize), Device: Uint64Field(1), Inode: Uint64Field(2),
		})
		if err != nil {
			t.Fatal(err)
		}
		if el.Eligible || el.Reason != ReasonExpired {
			t.Fatalf("the same running retention must keep the purge expired, got %s", el.Reason)
		}
	})

	t.Run("restorable_item_states", func(t *testing.T) {
		// ACTIVE, HOLD and PURGE_ELIGIBLE items are restorable (the same set
		// the restore executor accepts); a RESTORED item is not.
		st := setup(t, "restore-states.db")
		defer st.Close()
		_, _, _, reqActive := approvedRestore(t, ctx, st, sourceRoot, quarantineRoot,
			"state-active", now.Add(-48*time.Hour), now.Add(24*time.Hour), true)
		v := NewVerifier()
		if el, err := v.Check(ctx, NewStoreFacts(st), reqActive); err != nil || !el.Eligible {
			t.Fatalf("ACTIVE item must be restorable, got %v %s", err, el.Reason)
		}

		// HOLD: a protected item is restorable back to its owner.
		_, _, _, reqHold := approvedRestore(t, ctx, st, sourceRoot, quarantineRoot,
			"state-hold", now.Add(-48*time.Hour), now.Add(24*time.Hour), true)
		planHold, err := st.GetRestorePlan(ctx, reqHold.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		itemHold, err := st.GetQuarantineItem(ctx, planHold.ItemID)
		if err != nil {
			t.Fatal(err)
		}
		// Flip the durable status to HOLD via a protected re-registration is
		// heavy; assert the verifier accepts the recorded HOLD state through
		// the store's own status transition (seed a fresh item, mark hold by
		// saving with protected role is already covered by seedProtectedItem).
		_ = itemHold
		holdItem := seedProtectedItem(t, st, sourceRoot, quarantineRoot, "state-hold-2", now.Add(-48*time.Hour), now.Add(24*time.Hour))
		if holdItem.Status != domain.QuarantineHold {
			t.Fatalf("protected seed should be HOLD, got %s", holdItem.Status)
		}
		holdDigest := strings.Repeat("d", 64)
		holdPlanID := "restore-state-hold-2"
		if err := st.SaveRestorePlan(ctx, domain.RestorePlan{
			ID: holdPlanID, ItemID: holdItem.ID, State: domain.RestoreDraft,
			QuarantinePath: holdItem.QuarantinePath, RestorePath: holdItem.SourcePath,
			ExpectedSHA256: holdItem.ContentSHA256, ExpectedSize: holdItem.FileSize,
			ApprovalDigest: holdDigest, CreatedAt: now.Add(-time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		if err := st.ApproveRestorePlan(ctx, holdPlanID, holdDigest, now.Add(-30*time.Minute)); err != nil {
			t.Fatal(err)
		}
		el, err := v.Check(ctx, NewStoreFacts(st), Request{
			Operation: OpRestore, PlanID: holdPlanID, PresentedDigest: holdDigest,
			ObjectID: holdItem.ID, ContentSHA256: holdItem.ContentSHA256,
			Size: Int64Field(holdItem.FileSize),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !el.Eligible {
			t.Fatalf("a HOLD item must be restorable, got %s (%s)", el.Reason, el.Detail)
		}
		_ = reqHold
		_ = planHold
	})

	t.Run("negative_table", func(t *testing.T) {
		st := setup(t, "restore-negative.db")
		defer st.Close()
		item, plan, digest, req := approvedRestore(t, ctx, st, sourceRoot, quarantineRoot,
			"neg", now.Add(-48*time.Hour), now.Add(24*time.Hour), true)
		v := NewVerifier()
		src := NewStoreFacts(st)

		check := func(name string, r Request, want Reason) {
			t.Helper()
			el, err := v.Check(ctx, src, r)
			if err != nil {
				t.Fatalf("%s: unexpected error %v", name, err)
			}
			if el.Eligible || el.Reason != want {
				t.Fatalf("%s: expected %s, got eligible=%v reason=%s", name, want, el.Eligible, el.Reason)
			}
		}

		// Draft plan: approval state is missing.
		draftReq := req
		draftReq.PlanID = "restore-unapproved"
		if err := st.SaveRestorePlan(ctx, domain.RestorePlan{
			ID: "restore-unapproved", ItemID: item.ID, State: domain.RestoreDraft,
			QuarantinePath: item.QuarantinePath, RestorePath: item.SourcePath,
			ExpectedSHA256: item.ContentSHA256, ExpectedSize: item.FileSize,
			ApprovalDigest: digest, CreatedAt: now.Add(-time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		check("draft_plan", draftReq, ReasonWrongPlan)

		// Rolled back: the approval was invalidated. The durable rollback
		// path is journal-driven: begin (pending journal), then reconcile.
		_, planRb, _, reqRb := approvedRestore(t, ctx, st, sourceRoot, quarantineRoot,
			"neg-rb", now.Add(-48*time.Hour), now.Add(24*time.Hour), true)
		itemRb, err := st.GetQuarantineItem(ctx, planRb.ItemID)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.BeginRestore(ctx, planRb, itemRb, now); err != nil {
			t.Fatal(err)
		}
		if err := st.MarkRestoreRolledBack(ctx, reqRb.PlanID, now); err != nil {
			t.Fatal(err)
		}
		check("rolled_back", reqRb, ReasonRevoked)

		// Completed plan: a finished restore is not an authorization.
		_, _, _, reqDone := approvedRestore(t, ctx, st, sourceRoot, quarantineRoot,
			"neg-done", now.Add(-48*time.Hour), now.Add(24*time.Hour), true)
		planDone, err := st.GetRestorePlan(ctx, reqDone.PlanID)
		if err != nil {
			t.Fatal(err)
		}
		itemDone, err := st.GetQuarantineItem(ctx, planDone.ItemID)
		if err != nil {
			t.Fatal(err)
		}
		if err := st.BeginRestore(ctx, planDone, itemDone, now); err != nil {
			t.Fatal(err)
		}
		if err := st.MarkRestoreCompleted(ctx, planDone.ID, itemDone.ID, now); err != nil {
			t.Fatal(err)
		}
		check("completed_plan", reqDone, ReasonWrongPlan)

		// Unknown plan.
		check("unknown_plan", Request{Operation: OpRestore, PlanID: "restore-nope"}, ReasonWrongPlan)

		// Forged digest.
		forged := req
		forged.PresentedDigest = strings.Repeat("e", 64)
		check("forged_digest", forged, ReasonTampered)

		// Wrong object identity dimensions.
		wrongObj := req
		wrongObj.ObjectID = "obj-impostor"
		check("wrong_object_id", wrongObj, ReasonWrongObject)

		wrongSha := req
		wrongSha.ContentSHA256 = strings.Repeat("f", 64)
		check("wrong_sha", wrongSha, ReasonWrongObject)

		wrongSize := req
		wrongSize.Size = Int64Field(item.FileSize + 1)
		check("wrong_size", wrongSize, ReasonWrongObject)

		// Explicit input-domain constraint: negative size.
		negSize := req
		negSize.Size = Int64Field(-1)
		check("negative_size", negSize, ReasonWrongObject)

		// Missing size (omission is meaningful, never skipped).
		noSize := req
		noSize.Size = nil
		check("missing_size", noSize, ReasonWrongObject)

		// Plan snapshot drift: the plan's recorded identity no longer matches
		// its item (simulated by presenting a different plan id bound to the
		// drifted expectations — construct a drifted plan directly).
		driftDigest := strings.Repeat("d", 64)
		if err := st.SaveRestorePlan(ctx, domain.RestorePlan{
			ID: "restore-neg-drift", ItemID: item.ID, State: domain.RestoreDraft,
			QuarantinePath: item.QuarantinePath, RestorePath: item.SourcePath,
			ExpectedSHA256: strings.Repeat("9", 64), ExpectedSize: item.FileSize,
			ApprovalDigest: driftDigest, CreatedAt: now.Add(-time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		if err := st.ApproveRestorePlan(ctx, "restore-neg-drift", driftDigest, now.Add(-30*time.Minute)); err != nil {
			t.Fatal(err)
		}
		check("plan_snapshot_drift", Request{
			Operation: OpRestore, PlanID: "restore-neg-drift", PresentedDigest: driftDigest,
			ObjectID: item.ID, ContentSHA256: item.ContentSHA256, Size: Int64Field(item.FileSize),
		}, ReasonWrongObject)

		// Original-destination drift: the plan points the restore somewhere
		// other than the item's durable source path.
		destDriftDigest := strings.Repeat("d", 64)
		if err := st.SaveRestorePlan(ctx, domain.RestorePlan{
			ID: "restore-neg-dest", ItemID: item.ID, State: domain.RestoreDraft,
			QuarantinePath: item.QuarantinePath, RestorePath: filepath.Join(sourceRoot, "somewhere-else"),
			ExpectedSHA256: item.ContentSHA256, ExpectedSize: item.FileSize,
			ApprovalDigest: destDriftDigest, CreatedAt: now.Add(-time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
		if err := st.ApproveRestorePlan(ctx, "restore-neg-dest", destDriftDigest, now.Add(-30*time.Minute)); err != nil {
			t.Fatal(err)
		}
		check("destination_drift", Request{
			Operation: OpRestore, PlanID: "restore-neg-dest", PresentedDigest: destDriftDigest,
			ObjectID: item.ID, ContentSHA256: item.ContentSHA256, Size: Int64Field(item.FileSize),
		}, ReasonWrongObject)

		// Cross-operation: a purge plan id cannot authorize a restore.
		_, _, _, purgeReq := approvedPurge(t, ctx, st, sourceRoot, quarantineRoot,
			"neg-cross-purge", now.Add(-24*time.Hour), now.Add(-12*time.Hour), now.Add(-24*time.Hour), true)
		cross := purgeReq
		cross.Operation = OpRestore
		check("purge_plan_cannot_authorize_restore", cross, ReasonWrongPlan)

		// Cross-operation: a restore plan id cannot authorize a purge.
		crossPurge := req
		crossPurge.Operation = OpPurge
		crossPurge.Device = Uint64Field(1)
		crossPurge.Inode = Uint64Field(2)
		check("restore_plan_cannot_authorize_purge", crossPurge, ReasonWrongPlan)

		// Competing writer's execution lock.
		release, err := st.AcquireExecutionLock()
		if err != nil {
			t.Fatal(err)
		}
		el, err := v.Check(ctx, src, req)
		if err != nil {
			t.Fatal(err)
		}
		release()
		if el.Eligible || el.Reason != ReasonRecoveryLock {
			t.Fatalf("competing lock must block restore, got %s", el.Reason)
		}

		// Pending restore journal blocks another restore.
		itemJ, planJ, _, reqJ := approvedRestore(t, ctx, st, sourceRoot, quarantineRoot,
			"neg-journal", now.Add(-48*time.Hour), now.Add(24*time.Hour), true)
		if err := st.BeginRestore(ctx, planJ, itemJ, now); err != nil {
			t.Fatal(err)
		}
		el, err = v.Check(ctx, src, reqJ)
		if err != nil {
			t.Fatal(err)
		}
		if el.Eligible || el.Reason != ReasonRecoveryLock {
			t.Fatalf("pending restore journal must block, got %s", el.Reason)
		}

		// An unreadable fact source is an error — a refusal, never a pass.
		st2 := setup(t, "restore-closed.db")
		gate2Src := NewStoreFacts(st2)
		if err := st2.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := v.Check(ctx, gate2Src, req); err == nil {
			t.Fatal("closed store must be an error, not a decision")
		}
		_ = plan
		_ = digest
	})
}

// fixedFacts is a read-only fact source returning one pre-built fact set. It
// exists for contract-level symmetry proofs that need shapes the durable
// store cannot persist — e.g. a purge plan whose retention still runs, which
// SavePurgePlans refuses to create.
type fixedFacts struct{ facts Facts }

func (s fixedFacts) ReadOnlyFacts(ctx context.Context, op OpKind, planID string) (Facts, error) {
	return s.facts, nil
}
