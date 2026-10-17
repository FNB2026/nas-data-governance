package managedworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/authz"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// seedApprovedPurge creates the durable authorization truth for the gate
// tests: one quarantined synthetic object with an APPROVED, dry-run-verified
// purge plan, a second quarantined object whose plan has been rolled back
// (for the revoked denial — an item can carry only one non-terminal purge
// plan), and an APPROVED restore plan for the first object (restore is the
// reversible direction, allowed for PURGE_ELIGIBLE items). The plan ids are
// caller-chosen so the kernel driver can pre-compute deterministic item
// identities against the very same ids. Returns the identifiers the
// envelopes must carry; the restore digest is the fixed 64×'d'.
func seedApprovedPurge(t *testing.T, ctx context.Context, st *store.SQLiteStore, dir, eligiblePlanID, revokedPlanID, restorePlanID string) (itemID, planID, revokedPlan, restorePlan, digest, objectPath, contentHash string, size int64) {
	t.Helper()
	planID = eligiblePlanID
	restorePlan = restorePlanID
	sourceRoot := filepath.Join(dir, "source")
	quarantineRoot := filepath.Join(dir, "quarantine")
	for _, d := range []string{sourceRoot, quarantineRoot} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	retainUntil := now.Add(-24 * time.Hour) // retention already elapsed

	// quarantineObject runs one real quarantine registration: plan execution
	// journal + completion, exactly the durable path the app uses.
	quarantineObject := func(name, content, planID string) (string, string, string, int64) {
		objectPath := filepath.Join(quarantineRoot, name)
		if err := os.WriteFile(objectPath, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		contentHash := sha256Hex([]byte(content))
		size := int64(len(content))
		taskID := "task-" + name
		plan := domain.OperationPlan{
			ID:     planID,
			TaskID: taskID,
			State:  domain.PlanExecuting,
			Actions: []domain.PlannedAction{{
				Path:    filepath.Join(sourceRoot, name),
				Action:  domain.OperationQuarantine,
				Context: domain.DirectoryContext{Role: domain.RoleTemporary},
				File:    domain.FileInstance{Path: filepath.Join(sourceRoot, name), Size: size, ContentSHA256: contentHash},
			}},
		}
		if err := st.CreateTask(ctx, domain.OperationTask{ID: taskID, RootPath: sourceRoot, State: "completed", CreatedAt: now.Add(-48 * time.Hour)}); err != nil {
			t.Fatal(err)
		}
		if err := st.SavePlans(ctx, taskID, []domain.OperationPlan{plan}); err != nil {
			t.Fatal(err)
		}
		if err := st.BeginJournal(ctx, taskID, planID, plan.Actions); err != nil {
			t.Fatal(err)
		}
		if err := st.MarkJournalDone(ctx, planID, 0, objectPath); err != nil {
			t.Fatal(err)
		}
		if err := st.CompletePlanExecution(ctx, plan, domain.PlanVerified, nil, now.Add(-48*time.Hour), retainUntil); err != nil {
			t.Fatal(err)
		}
		items, err := st.ListQuarantineItems(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range items {
			if it.PlanID == planID {
				return it.ID, objectPath, contentHash, size
			}
		}
		t.Fatalf("seeded quarantine item not found for %s", planID)
		return "", "", "", 0
	}

	itemID, objectPath, contentHash, size = quarantineObject("gate-object", "gate-object-payload", planID)
	revokedItemID, _, _, _ := quarantineObject("gate-object-revoked", "gate-object-revoked-payload", revokedPlanID)

	digest = strings.Repeat("a", 64)
	if err := st.SavePurgePlans(ctx, []domain.PurgePlan{{
		ID: planID, ItemID: itemID, State: domain.PurgeDraft,
		ExpectedPath: objectPath, ExpectedSHA256: contentHash,
		ExpectedSize: size, RetainUntil: retainUntil,
		ApprovalDigest: digest, CreatedAt: now.Add(-12 * time.Hour),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.ApprovePurgePlan(ctx, planID, digest, now.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkPurgeDryRunVerified(ctx, planID, digest, now.Add(-20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	revokedDigest := strings.Repeat("c", 64)
	if err := st.SavePurgePlans(ctx, []domain.PurgePlan{{
		ID: revokedPlanID, ItemID: revokedItemID, State: domain.PurgeDraft,
		ExpectedPath:   filepath.Join(quarantineRoot, "gate-object-revoked"),
		ExpectedSHA256: sha256Hex([]byte("gate-object-revoked-payload")),
		ExpectedSize:   int64(len("gate-object-revoked-payload")), RetainUntil: retainUntil,
		ApprovalDigest: revokedDigest, CreatedAt: now.Add(-12 * time.Hour),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.ApprovePurgePlan(ctx, revokedPlanID, revokedDigest, now.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkPurgeDryRunVerified(ctx, revokedPlanID, revokedDigest, now.Add(-20*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkPurgeRolledBack(ctx, revokedPlanID, revokedItemID, now); err != nil {
		t.Fatal(err)
	}

	// An approved restore plan for the eligible object: restore is bound to
	// the same item identity and the original source destination. The item
	// is PURGE_ELIGIBLE after its purge plan, which is a restorable state.
	restoreDigest := strings.Repeat("d", 64)
	if err := st.SaveRestorePlan(ctx, domain.RestorePlan{
		ID: restorePlanID, ItemID: itemID, State: domain.RestoreDraft,
		QuarantinePath: objectPath, RestorePath: filepath.Join(sourceRoot, "gate-object"),
		ExpectedSHA256: contentHash, ExpectedSize: size,
		ApprovalDigest: restoreDigest, CreatedAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.ApproveRestorePlan(ctx, restorePlanID, restoreDigest, now.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return itemID, planID, revokedPlanID, restorePlanID, digest, objectPath, contentHash, size
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func gateFrame(t *testing.T, op string, fields map[string]any) Request {
	t.Helper()
	body := map[string]any{"op": op}
	for k, v := range fields {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request, err := DecodeRequest(raw)
	if err != nil {
		t.Fatalf("frame for %s refused: %v", op, err)
	}
	return request
}

// TestDispatchAuthorized_PING_STATUS_Unchanged proves the harmless surface is
// byte-identical with and without the gate.
func TestDispatchAuthorized_PING_STATUS_Unchanged(t *testing.T) {
	ctx := context.Background()
	for _, gate := range []*AuthorizationGate{nil, {}} {
		pong, err := DispatchAuthorized(ctx, gateFrame(t, "PING", nil), Subject{UID: 1, GID: 1}, gate)
		if err != nil {
			t.Fatal("PING refused", err)
		}
		if m, ok := pong.(map[string]any); !ok || m["ok"] != true {
			t.Fatalf("PING shape changed: %#v", pong)
		}
		status, err := DispatchAuthorized(ctx, gateFrame(t, "STATUS", nil), Subject{UID: 1, GID: 1}, gate)
		if err != nil {
			t.Fatal("STATUS refused", err)
		}
		s, ok := status.(Status)
		if !ok || !s.Ready || s.PurgeReleaseAllowed || s.Executable != 2 {
			t.Fatalf("STATUS shape changed: %#v", status)
		}
	}
}

// TestDispatchAuthorized_NilGateFailClosed proves the ungated path refuses
// dangerous operations exactly as Dispatch does — wiring the gate is the only
// way a dangerous request gets past the authorization line.
func TestDispatchAuthorized_NilGateFailClosed(t *testing.T) {
	ctx := context.Background()
	for _, op := range []string{"SEAL", "PURGE", "INSPECT"} {
		req := gateFrame(t, op, map[string]any{"object_id": strings.Repeat("a", 64), "size": 1, "sha256": strings.Repeat("b", 64)})
		_, err := DispatchAuthorized(ctx, req, Subject{UID: 1, GID: 1}, nil)
		if !errors.Is(err, ErrAuthorization) {
			t.Fatalf("%s without a gate must be authorization_refused, got %v", op, err)
		}
	}
}

// TestDispatchAuthorized_BusinessGate exercises the real gate against a real
// durable store: the legal envelope reaches an eligible verdict (and nothing
// executes), every defective envelope gets its typed refusal, and protocol
// violations stay protocol violations.
func TestDispatchAuthorized_BusinessGate(t *testing.T) {
	if testing.Short() {
		t.Skip("business gate test uses a real store")
	}
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "gate.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	itemID, planID, revokedPlanID, _, digest, objectPath, contentHash, size := seedApprovedPurge(t, ctx, st, dir, "purge-gate-eligible", "purge-gate-revoked", "restore-gate-eligible")
	gate := NewStoreAuthorizationGate(st)
	peer := Subject{UID: 24001, GID: 24001}

	before, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("legal_envelope_reaches_eligible_verdict", func(t *testing.T) {
		req := gateFrame(t, "PURGE", map[string]any{
			"plan_id": planID, "object_id": itemID, "sha256": contentHash,
			"authorization_sha256": digest, "size": size, "device": 1, "inode": 2,
		})
		resp, err := DispatchAuthorized(ctx, req, peer, gate)
		if err != nil {
			t.Fatalf("legal envelope refused: %v", err)
		}
		verdict, ok := resp.(Verdict)
		if !ok || !verdict.Eligible {
			t.Fatalf("expected eligible verdict, got %#v", resp)
		}
		if verdict.PlanID != planID || verdict.ObjectID != itemID {
			t.Fatalf("verdict identity mismatch: %#v", verdict)
		}
		if verdict.Reason != string(authz.ReasonEligible) {
			t.Fatalf("verdict reason: %s", verdict.Reason)
		}
	})

	t.Run("verdict_executes_nothing", func(t *testing.T) {
		after, err := os.ReadFile(objectPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatal("the eligible verdict must not touch the object")
		}
	})

	t.Run("typed_denials", func(t *testing.T) {
		cases := []struct {
			name   string
			fields map[string]any
			reason authz.Reason
		}{
			{"forged_digest", map[string]any{
				"plan_id": planID, "object_id": itemID, "sha256": contentHash,
				"authorization_sha256": strings.Repeat("b", 64), "size": size, "device": 1, "inode": 2,
			}, authz.ReasonTampered},
			{"wrong_size", map[string]any{
				"plan_id": planID, "object_id": itemID, "sha256": contentHash,
				"authorization_sha256": digest, "size": size + 1, "device": 1, "inode": 2,
			}, authz.ReasonWrongObject},
			{"wrong_object", map[string]any{
				"plan_id": planID, "object_id": strings.Repeat("e", 64), "sha256": contentHash,
				"authorization_sha256": digest, "size": size, "device": 1, "inode": 2,
			}, authz.ReasonWrongObject},
			{"missing_plan_id", map[string]any{
				"object_id": itemID, "sha256": contentHash,
				"authorization_sha256": digest, "size": size, "device": 1, "inode": 2,
			}, authz.ReasonWrongPlan},
			{"unknown_plan", map[string]any{
				"plan_id": "purge-does-not-exist", "object_id": itemID, "sha256": contentHash,
				"authorization_sha256": digest, "size": size, "device": 1, "inode": 2,
			}, authz.ReasonWrongPlan},
			{"revoked_plan", map[string]any{
				"plan_id": revokedPlanID, "object_id": itemID, "sha256": contentHash,
				"authorization_sha256": digest, "size": size, "device": 1, "inode": 2,
			}, authz.ReasonRevoked},
			{"missing_device", map[string]any{
				"plan_id": planID, "object_id": itemID, "sha256": contentHash,
				"authorization_sha256": digest, "size": size, "inode": 2,
			}, authz.ReasonWrongObject},
		}
		for _, tc := range cases {
			req := gateFrame(t, "PURGE", tc.fields)
			_, err := DispatchAuthorized(ctx, req, peer, gate)
			if !errors.Is(err, ErrAuthorization) {
				t.Fatalf("%s: expected authorization denial, got %v", tc.name, err)
			}
			var denied *AuthorizationDenied
			if !errors.As(err, &denied) {
				t.Fatalf("%s: denial is not typed: %v", tc.name, err)
			}
			if denied.Reason != tc.reason {
				t.Fatalf("%s: expected %s, got %s (%s)", tc.name, tc.reason, denied.Reason, denied.Detail)
			}
		}
	})

	t.Run("protocol_violations_stay_protocol", func(t *testing.T) {
		// A string size is a frame-contract violation, not an omission.
		raw, _ := json.Marshal(map[string]any{"op": "PURGE", "plan_id": planID, "object_id": itemID,
			"sha256": contentHash, "authorization_sha256": digest, "size": "7", "device": 1, "inode": 2})
		req, err := DecodeRequest(raw)
		if err != nil {
			t.Fatalf("string size must decode (key is allowed), got %v", err)
		}
		if _, err := DispatchAuthorized(ctx, req, peer, gate); !errors.Is(err, ErrProtocol) {
			t.Fatalf("string size must be a protocol refusal, got %v", err)
		}
		// A negative device number is not a valid device identity.
		req = gateFrame(t, "PURGE", map[string]any{
			"plan_id": planID, "object_id": itemID, "sha256": contentHash,
			"authorization_sha256": digest, "size": size, "device": -1, "inode": 2,
		})
		if _, err := DispatchAuthorized(ctx, req, peer, gate); !errors.Is(err, ErrProtocol) {
			t.Fatalf("negative device must be a protocol refusal, got %v", err)
		}
		// RESTORE is protocol-recognized since R4, but it still has no
		// escape hatch: an empty RESTORE frame is a typed business refusal,
		// not a pass.
		req = Request{Op: "RESTORE", Fields: map[string]json.RawMessage{}}
		_, err = DispatchAuthorized(ctx, req, peer, gate)
		var denied *AuthorizationDenied
		if !errors.As(err, &denied) || denied.Reason != authz.ReasonWrongPlan {
			t.Fatalf("empty RESTORE frame must be wrong_plan under the gate, got %v", err)
		}
	})

	t.Run("fact_source_failure_is_refusal", func(t *testing.T) {
		st2, err := store.Open(ctx, filepath.Join(dir, "gate-closed.db"))
		if err != nil {
			t.Fatal(err)
		}
		gate2 := NewStoreAuthorizationGate(st2)
		if err := st2.Close(); err != nil {
			t.Fatal(err)
		}
		req := gateFrame(t, "PURGE", map[string]any{
			"plan_id": planID, "object_id": itemID, "sha256": contentHash,
			"authorization_sha256": digest, "size": size, "device": 1, "inode": 2,
		})
		_, err = DispatchAuthorized(ctx, req, peer, gate2)
		if err == nil {
			t.Fatal("an unreadable fact source must be a refusal")
		}
		if !errors.Is(err, ErrAuthorization) {
			t.Fatalf("fact-source refusal must carry the authorization sentinel, got %v", err)
		}
		var denied *AuthorizationDenied
		if errors.As(err, &denied) {
			t.Fatal("a fact-source failure must not pretend to be a typed business reason")
		}
	})

	t.Run("purge_plan_cannot_authorize_restore", func(t *testing.T) {
		// R4: RESTORE has its own durable fact source (restore_plans). A
		// purge plan id presented to a RESTORE request is structurally
		// unknown to the restore fact source, so the request keeps its
		// wrong_plan rejection — one plan kind never authorizes the other.
		v := authz.NewVerifier()
		el, err := v.Check(ctx, authz.NewStoreFacts(st), authz.Request{
			Operation: authz.OpRestore, PlanID: planID,
			PresentedDigest: digest, ObjectID: itemID,
			ContentSHA256: contentHash, Size: authz.Int64Field(size),
		})
		if err != nil {
			t.Fatal(err)
		}
		if el.Eligible || el.Reason != authz.ReasonWrongPlan {
			t.Fatalf("a purge plan must not authorize RESTORE, got %s", el.Reason)
		}
	})

	t.Run("restore_without_own_plan_stays_refused", func(t *testing.T) {
		// A RESTORE envelope with no restore plan behind it is refused even
		// though the operation is now protocol-recognized: protocol admission
		// is not business authorization.
		req := gateFrame(t, "RESTORE", map[string]any{
			"object_id": itemID, "sha256": contentHash,
			"authorization_sha256": digest, "size": size,
		})
		_, err := DispatchAuthorized(ctx, req, peer, gate)
		var denied *AuthorizationDenied
		if !errors.As(err, &denied) || denied.Reason != authz.ReasonWrongPlan {
			t.Fatalf("RESTORE without a restore plan must be wrong_plan, got %v", err)
		}
		// And without a gate the op is refused exactly like the others.
		if _, err := DispatchAuthorized(ctx, req, peer, nil); !errors.Is(err, ErrAuthorization) {
			t.Fatalf("RESTORE without a gate must be authorization_refused, got %v", err)
		}
	})

	t.Run("negative_size_is_a_protocol_violation", func(t *testing.T) {
		// A8: explicit input-domain constraints — a negative size is not an
		// identity dimension and is refused by the frame contract itself,
		// never by equality substitution.
		raw, _ := json.Marshal(map[string]any{"op": "PURGE", "plan_id": planID, "object_id": itemID,
			"sha256": contentHash, "authorization_sha256": digest, "size": -5, "device": 1, "inode": 2})
		req, err := DecodeRequest(raw)
		if err != nil {
			t.Fatalf("negative size must decode (key is allowed), got %v", err)
		}
		if _, err := DispatchAuthorized(ctx, req, peer, gate); !errors.Is(err, ErrProtocol) {
			t.Fatalf("negative size must be a protocol refusal, got %v", err)
		}
	})
}
