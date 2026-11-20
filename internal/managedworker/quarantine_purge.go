package managedworker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/managedfs"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// QuarantinePurge owns the worker-side business flow for an explicitly
// configured quarantine root. Only durable item IDs may produce plans.
type QuarantinePurge struct {
	project *store.SQLiteStore
	root    string
}

func NewQuarantinePurge(project *store.SQLiteStore, root string) (*QuarantinePurge, error) {
	if runtime.GOOS != "linux" {
		return nil, ErrUnsupported
	}
	if project == nil || root == "/" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, ErrIsolation
	}
	return &QuarantinePurge{project: project, root: root}, nil
}

type quarantinePurgeEvidence struct {
	Operation   string
	PlanID      string
	Project     string
	ItemID      string
	RetainUntil time.Time
	ExpiresAt   time.Time
	Target      managedfs.PurgeTarget
}

type QuarantinePurgeResult struct {
	ID        string    `json:"id"`
	ItemID    string    `json:"item_id"`
	Digest    string    `json:"digest"`
	State     string    `json:"state"`
	ExpiresAt time.Time `json:"expires_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func quarantineResult(p store.DualPurgePlan) QuarantinePurgeResult {
	return QuarantinePurgeResult{p.ID, p.SubjectID, p.Digest, p.State, p.ExpiresAt, p.UpdatedAt}
}

func (s *QuarantinePurge) subject(ctx context.Context, itemID string, now time.Time) (domain.QuarantineItem, string, error) {
	item, err := s.project.GetQuarantineItem(ctx, itemID)
	if err != nil || item.HoldReason != "" || now.Before(item.RetainUntil) ||
		(item.Status != domain.QuarantineActive && item.Status != domain.QuarantinePurgeEligible) {
		return item, "", ErrAuthorization
	}
	if !filepath.IsAbs(item.QuarantinePath) || filepath.Clean(item.QuarantinePath) != item.QuarantinePath {
		return item, "", ErrAuthorization
	}
	relative, err := filepath.Rel(s.root, item.QuarantinePath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		return item, "", ErrAuthorization
	}
	return item, relative, nil
}

func evidenceDigest(evidence []byte) string {
	sum := sha256.Sum256(evidence)
	return hex.EncodeToString(sum[:])
}

func (s *QuarantinePurge) Plan(ctx context.Context, itemID string) (QuarantinePurgeResult, error) {
	empty := QuarantinePurgeResult{}
	if itemID == "" || len(itemID) > 128 || pathLike(itemID) {
		return empty, ErrProtocol
	}
	release, err := s.project.AcquireExecutionLock()
	if err != nil {
		return empty, ErrBusy
	}
	defer release()
	caps, err := s.project.GovernanceCapabilities(ctx)
	if err != nil || caps.RecoveryLockActive || caps.ScanActive {
		return empty, ErrAuthorization
	}
	now := time.Now().UTC()
	item, relative, err := s.subject(ctx, itemID, now)
	if err != nil {
		return empty, err
	}
	target, err := managedfs.ObservePurgeTarget(ctx, s.root, relative)
	if err != nil || item.FileSize < 0 || target.Size != uint64(item.FileSize) || target.SHA256 != item.ContentSHA256 {
		return empty, ErrAuthorization
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return empty, ErrIsolation
	}
	plan := store.DualPurgePlan{
		ID: hex.EncodeToString(nonce[:]), Operation: store.QuarantinePurge, SubjectID: itemID,
		State: "DRAFT", UpdatedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	}
	raw, err := json.Marshal(quarantinePurgeEvidence{
		Operation: plan.Operation, PlanID: plan.ID, Project: caps.ProjectScope, ItemID: item.ID,
		RetainUntil: item.RetainUntil, ExpiresAt: plan.ExpiresAt, Target: target,
	})
	if err != nil {
		return empty, ErrIsolation
	}
	plan.EvidenceJSON, plan.Digest = string(raw), evidenceDigest(raw)
	if err := s.project.InsertDualPurgePlan(ctx, plan); err != nil {
		return empty, ErrIsolation
	}
	return quarantineResult(plan), nil
}

func (s *QuarantinePurge) advance(ctx context.Context, id, digest, from, to string, confirmation bool) (QuarantinePurgeResult, error) {
	empty := QuarantinePurgeResult{}
	if !opaqueID(id) || !opaqueID(digest) || (to == "EXECUTING" && !confirmation) {
		return empty, ErrAuthorization
	}
	release, err := s.project.AcquireExecutionLock()
	if err != nil {
		return empty, ErrBusy
	}
	defer release()
	p, err := s.project.ReadDualPurgePlan(ctx, id)
	if err != nil || p.Operation != store.QuarantinePurge || p.State != from || p.Digest != digest ||
		evidenceDigest([]byte(p.EvidenceJSON)) != digest {
		return empty, ErrAuthorization
	}
	var proof quarantinePurgeEvidence
	if err := json.Unmarshal([]byte(p.EvidenceJSON), &proof); err != nil || proof.PlanID != id ||
		proof.Operation != p.Operation || proof.ItemID != p.SubjectID || !proof.ExpiresAt.Equal(p.ExpiresAt) {
		return empty, ErrAuthorization
	}
	now := time.Now().UTC()
	item, relative, err := s.subject(ctx, p.SubjectID, now)
	if err != nil || relative != proof.Target.Relative || !item.RetainUntil.Equal(proof.RetainUntil) ||
		item.FileSize < 0 || uint64(item.FileSize) != proof.Target.Size || item.ContentSHA256 != proof.Target.SHA256 {
		return empty, ErrAuthorization
	}
	caps, err := s.project.GovernanceCapabilities(ctx)
	if err != nil || caps.ProjectScope != proof.Project {
		return empty, ErrAuthorization
	}
	transaction := managedfs.PurgeTransaction{ID: id, ApprovalSHA256: digest, RootPath: s.root, Target: proof.Target}
	if to == "DRY_RUN" || to == "EXECUTING" {
		if err := managedfs.DryRunPurgeTransaction(ctx, transaction); err != nil {
			return empty, err
		}
	}
	if to == "RECOVER" {
		receipt, err := managedfs.RecoverPurgeTransaction(ctx, transaction)
		if err != nil {
			return empty, err
		}
		if err := s.project.FinishQuarantinePurge(context.WithoutCancel(ctx), p, item, receipt.State, time.Now().UTC()); err != nil {
			return empty, managedfs.ErrRecoveryRequired
		}
		p.State, p.UpdatedAt = receipt.State, time.Now().UTC()
		return quarantineResult(p), nil
	}
	// Filesystem verification may itself be lengthy. The approval window
	// applies to the durable execution claim, using a fresh service clock.
	now = time.Now().UTC()
	if err := s.project.AdvanceQuarantinePurge(ctx, p, item, to, now); err != nil {
		return empty, ErrAuthorization
	}
	p.State, p.UpdatedAt = to, now
	if to == "EXECUTING" {
		receipt, err := managedfs.ExecutePurgeTransaction(ctx, transaction)
		if err != nil {
			// An execution claim always remains visible if its outcome is
			// incomplete, even when an error arrived before staging.
			return quarantineResult(p), managedfs.ErrRecoveryRequired
		}
		if receipt.State != "PURGED" {
			return quarantineResult(p), managedfs.ErrRecoveryRequired
		}
		p.State, p.UpdatedAt = receipt.State, time.Now().UTC()
		if err := s.project.FinishQuarantinePurge(context.WithoutCancel(ctx), p, item, receipt.State, p.UpdatedAt); err != nil {
			p.State = "EXECUTING"
			return quarantineResult(p), managedfs.ErrRecoveryRequired
		}
	}
	return quarantineResult(p), nil
}

func (s *QuarantinePurge) Approve(ctx context.Context, id, digest string) (QuarantinePurgeResult, error) {
	return s.advance(ctx, id, digest, "DRAFT", "APPROVED", false)
}

func (s *QuarantinePurge) DryRun(ctx context.Context, id, digest string) (QuarantinePurgeResult, error) {
	return s.advance(ctx, id, digest, "APPROVED", "DRY_RUN", false)
}

func (s *QuarantinePurge) Execute(ctx context.Context, id, digest string, confirmed bool) (QuarantinePurgeResult, error) {
	return s.advance(ctx, id, digest, "DRY_RUN", "EXECUTING", confirmed)
}

func (s *QuarantinePurge) Cancel(ctx context.Context, id, digest string) (QuarantinePurgeResult, error) {
	if !opaqueID(id) || !opaqueID(digest) {
		return QuarantinePurgeResult{}, ErrAuthorization
	}
	release, err := s.project.AcquireExecutionLock()
	if err != nil {
		return QuarantinePurgeResult{}, ErrBusy
	}
	defer release()
	p, err := s.project.ReadDualPurgePlan(ctx, id)
	if err != nil || p.Operation != store.QuarantinePurge ||
		p.Digest != digest || evidenceDigest([]byte(p.EvidenceJSON)) != digest {
		return QuarantinePurgeResult{}, ErrAuthorization
	}
	now := time.Now().UTC()
	if err := s.project.AdvanceQuarantinePurge(ctx, p, domain.QuarantineItem{ID: p.SubjectID}, "CANCELLED", now); err != nil {
		return QuarantinePurgeResult{}, ErrAuthorization
	}
	p.State, p.UpdatedAt = "CANCELLED", now
	return quarantineResult(p), nil
}

func (s *QuarantinePurge) Recover(ctx context.Context, id, digest string) (QuarantinePurgeResult, error) {
	return s.advance(ctx, id, digest, "EXECUTING", "RECOVER", false)
}

func (s *QuarantinePurge) Receipt(ctx context.Context, id, digest string) (QuarantinePurgeResult, error) {
	if !opaqueID(id) || !opaqueID(digest) {
		return QuarantinePurgeResult{}, ErrAuthorization
	}
	p, err := s.project.ReadDualPurgePlan(ctx, id)
	if err != nil || p.Operation != store.QuarantinePurge || p.Digest != digest ||
		evidenceDigest([]byte(p.EvidenceJSON)) != digest {
		return QuarantinePurgeResult{}, ErrAuthorization
	}
	return quarantineResult(p), nil
}

func (s *QuarantinePurge) dispatch(ctx context.Context, req Request) (QuarantinePurgeResult, error) {
	if req.Op == "QUARANTINE_PURGE_PLAN" {
		item, err := req.String("item_id")
		if err != nil {
			return QuarantinePurgeResult{}, err
		}
		return s.Plan(ctx, item)
	}
	id, err := req.OpaqueID("plan_id")
	if err != nil {
		return QuarantinePurgeResult{}, err
	}
	digest, err := req.OpaqueID("digest")
	if err != nil {
		return QuarantinePurgeResult{}, err
	}
	switch req.Op {
	case "QUARANTINE_PURGE_APPROVE":
		return s.Approve(ctx, id, digest)
	case "QUARANTINE_PURGE_DRY_RUN":
		return s.DryRun(ctx, id, digest)
	case "QUARANTINE_PURGE_EXECUTE":
		confirmation, err := req.String("confirmation")
		if err != nil || confirmation != "永久清理 "+id {
			return QuarantinePurgeResult{}, ErrAuthorization
		}
		return s.Execute(ctx, id, digest, true)
	case "QUARANTINE_PURGE_CANCEL":
		return s.Cancel(ctx, id, digest)
	case "QUARANTINE_PURGE_RECOVER":
		return s.Recover(ctx, id, digest)
	case "QUARANTINE_PURGE_RECEIPT":
		return s.Receipt(ctx, id, digest)
	}
	return QuarantinePurgeResult{}, ErrProtocol
}
