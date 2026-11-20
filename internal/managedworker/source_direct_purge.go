package managedworker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/dircontext"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/managedfs"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// SourcePurge admits only an isolated, explicitly mounted local source.
type SourcePurge struct {
	project *store.SQLiteStore
	root    string
}

func NewSourcePurge(project *store.SQLiteStore, root string) (*SourcePurge, error) {
	if runtime.GOOS != "linux" {
		return nil, ErrUnsupported
	}
	if project == nil || root == "/" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, ErrIsolation
	}
	return &SourcePurge{project: project, root: root}, nil
}

type sourcePurgeEvidence struct {
	ID, Project    string
	Facts          store.SourcePurgeFacts
	Target, Keeper managedfs.PurgeTarget
	Role           domain.DirectoryRole
	ExpiresAt      time.Time
}

type SourcePurgeResult struct {
	ID            string                    `json:"id"`
	StorageID     string                    `json:"storage_id"`
	Digest        string                    `json:"digest"`
	State         string                    `json:"state"`
	TargetPath    string                    `json:"target_path"`
	KeeperPath    string                    `json:"keeper_path"`
	ContentSHA256 string                    `json:"sha256"`
	TargetID      int64                     `json:"target_id,string"`
	KeeperID      int64                     `json:"keeper_id,string"`
	Size          int64                     `json:"size"`
	Role          domain.DirectoryRole      `json:"role"`
	ExpiresAt     time.Time                 `json:"expires_at"`
	UpdatedAt     time.Time                 `json:"updated_at"`
	Choices       []store.SourcePurgeChoice `json:"choices,omitempty"`
}

func sourcePurgeResult(p store.DualPurgePlan, proof sourcePurgeEvidence) SourcePurgeResult {
	return SourcePurgeResult{
		ID: p.ID, StorageID: proof.Facts.StorageID, Digest: p.Digest, State: p.State,
		TargetPath: proof.Target.Relative, KeeperPath: proof.Keeper.Relative, ContentSHA256: proof.Target.SHA256,
		TargetID: proof.Facts.Target.ID, KeeperID: proof.Facts.Keeper.ID, Size: proof.Facts.Target.Size, Role: proof.Role,
		ExpiresAt: p.ExpiresAt, UpdatedAt: p.UpdatedAt,
	}
}

func sourceFile(ctx context.Context, root string, f store.SourcePurgeFile) (managedfs.PurgeTarget, error) {
	relative, err := filepath.Rel(root, f.Path)
	if err != nil || !filepath.IsAbs(f.Path) || filepath.Clean(f.Path) != f.Path || relative == "." ||
		relative == ".." || strings.HasPrefix(relative, "../") {
		return managedfs.PurgeTarget{}, ErrAuthorization
	}
	target, err := managedfs.ObservePurgeTarget(ctx, root, relative)
	modified, timeErr := time.Parse(time.RFC3339Nano, f.MTime)
	if err != nil || timeErr != nil || target.File.Device != f.Device || target.File.Inode != f.Inode ||
		target.SHA256 != f.SHA256 || target.Size != uint64(f.Size) || uint32(target.Mode)&0777 != f.Mode&0777 ||
		!modified.Equal(time.Unix(target.MTimeSec, int64(target.MTimeNsec))) {
		return managedfs.PurgeTarget{}, ErrAuthorization
	}
	return target, nil
}

func (s *SourcePurge) Plan(ctx context.Context, storageID string, targetID, keeperID int64) (SourcePurgeResult, error) {
	var empty SourcePurgeResult
	release, err := s.project.AcquireExecutionLock()
	if err != nil {
		return empty, ErrBusy
	}
	defer release()
	caps, err := s.project.GovernanceCapabilities(ctx)
	if err != nil || caps.RecoveryLockActive || caps.ScanActive {
		return empty, ErrAuthorization
	}
	facts, err := s.project.SourcePurgeSnapshot(ctx, storageID, targetID, keeperID)
	if err != nil || facts.RootPath != s.root {
		return empty, ErrAuthorization
	}
	role, err := facts.SourcePurgeDecision(dircontext.RuleVersion())
	if err != nil {
		return empty, ErrAuthorization
	}
	target, err := sourceFile(ctx, s.root, facts.Target)
	if err != nil {
		return empty, err
	}
	keeper, err := sourceFile(ctx, s.root, facts.Keeper)
	if err != nil || keeper.File == target.File || keeper.Root != target.Root {
		return empty, ErrAuthorization
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return empty, ErrIsolation
	}
	now := time.Now().UTC()
	p := store.DualPurgePlan{
		ID: hex.EncodeToString(nonce[:]), Operation: store.SourceDirectPurge, SubjectID: strconv.FormatInt(targetID, 10),
		State: "DRAFT", UpdatedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	}
	proof := sourcePurgeEvidence{p.ID, caps.ProjectScope, facts, target, keeper, role, p.ExpiresAt}
	raw, err := json.Marshal(proof)
	if err != nil {
		return empty, ErrIsolation
	}
	p.EvidenceJSON, p.Digest = string(raw), evidenceDigest(raw)
	if err := s.project.InsertDualPurgePlan(ctx, p); err != nil {
		return empty, ErrIsolation
	}
	return sourcePurgeResult(p, proof), nil
}

func (s *SourcePurge) read(ctx context.Context, id, digest string) (store.DualPurgePlan, sourcePurgeEvidence, error) {
	var proof sourcePurgeEvidence
	if !opaqueID(id) || !opaqueID(digest) {
		return store.DualPurgePlan{}, proof, ErrAuthorization
	}
	p, err := s.project.ReadDualPurgePlan(ctx, id)
	if err != nil || p.Operation != store.SourceDirectPurge || p.Digest != digest ||
		evidenceDigest([]byte(p.EvidenceJSON)) != digest || json.Unmarshal([]byte(p.EvidenceJSON), &proof) != nil ||
		proof.ID != id || proof.Facts.RootPath != s.root || !proof.ExpiresAt.Equal(p.ExpiresAt) ||
		p.SubjectID != strconv.FormatInt(proof.Facts.Target.ID, 10) {
		return p, proof, ErrAuthorization
	}
	return p, proof, nil
}

// Advance uses a single stored proof for each authorization. No operation
// applies to a collection or implicitly approves a different target.
func (s *SourcePurge) Advance(ctx context.Context, id, digest, op, confirmation string) (SourcePurgeResult, error) {
	var empty SourcePurgeResult
	release, err := s.project.AcquireExecutionLock()
	if err != nil {
		return empty, ErrBusy
	}
	defer release()
	p, proof, err := s.read(ctx, id, digest)
	if err != nil {
		return empty, err
	}
	caps, err := s.project.GovernanceCapabilities(ctx)
	if err != nil || caps.ProjectScope != proof.Project {
		return empty, ErrAuthorization
	}
	if op == "RECEIPT" {
		return sourcePurgeResult(p, proof), nil
	}
	now := time.Now().UTC()
	txn := managedfs.PurgeTransaction{ID: id, ApprovalSHA256: digest, RootPath: s.root, Target: proof.Target, Keeper: &proof.Keeper}
	if op == "RECOVER" {
		if p.State != "EXECUTING" {
			return empty, ErrAuthorization
		}
		receipt, err := managedfs.RecoverPurgeTransaction(ctx, txn)
		if err != nil {
			return empty, err
		}
		return s.finish(ctx, p, proof, receipt.State)
	}
	next := ""
	switch op {
	case "APPROVE":
		next = "APPROVED"
	case "DRY_RUN":
		next = "DRY_RUN"
	case "EXECUTE":
		if confirmation != "永久删除 "+id || p.State != "DRY_RUN" {
			return empty, ErrAuthorization
		}
		next = "EXECUTING"
	case "CANCEL":
		next = "CANCELLED"
	default:
		return empty, ErrProtocol
	}
	if next != "CANCELLED" {
		if _, err := proof.Facts.SourcePurgeDecision(dircontext.RuleVersion()); err != nil {
			return empty, ErrAuthorization
		}
		if next != "APPROVED" {
			if err := managedfs.DryRunPurgeTransaction(ctx, txn); err != nil {
				return empty, err
			}
		}
	}
	if err := s.project.AdvanceSourcePurge(ctx, p, proof.Facts, next, now); err != nil {
		return empty, ErrAuthorization
	}
	p.State, p.UpdatedAt = next, now
	if next != "EXECUTING" {
		return sourcePurgeResult(p, proof), nil
	}
	receipt, err := managedfs.ExecutePurgeTransaction(ctx, txn)
	if err != nil {
		return sourcePurgeResult(p, proof), managedfs.ErrRecoveryRequired
	}
	return s.finish(ctx, p, proof, receipt.State)
}

func (s *SourcePurge) finish(ctx context.Context, p store.DualPurgePlan, proof sourcePurgeEvidence, state string) (SourcePurgeResult, error) {
	now := time.Now().UTC()
	if err := s.project.FinishSourcePurge(context.WithoutCancel(ctx), p, proof.Facts, state, now); err != nil {
		return sourcePurgeResult(p, proof), managedfs.ErrRecoveryRequired
	}
	p.State, p.UpdatedAt = state, now
	return sourcePurgeResult(p, proof), nil
}

func sourcePurgeOp(op string) bool {
	switch op {
	case "SOURCE_DIRECT_PURGE_LIST", "SOURCE_DIRECT_PURGE_PLAN", "SOURCE_DIRECT_PURGE_APPROVE", "SOURCE_DIRECT_PURGE_DRY_RUN",
		"SOURCE_DIRECT_PURGE_EXECUTE", "SOURCE_DIRECT_PURGE_CANCEL", "SOURCE_DIRECT_PURGE_RECOVER", "SOURCE_DIRECT_PURGE_RECEIPT":
		return true
	}
	return false
}

func (s *SourcePurge) dispatch(ctx context.Context, req Request) (SourcePurgeResult, error) {
	var empty SourcePurgeResult
	if !sourcePurgeOp(req.Op) {
		return empty, ErrProtocol
	}
	if req.Op == "SOURCE_DIRECT_PURGE_LIST" {
		choices, err := s.project.SourcePurgeCandidates(ctx, s.root)
		return SourcePurgeResult{Choices: choices}, err
	}
	if req.Op == "SOURCE_DIRECT_PURGE_PLAN" {
		storageID, storageErr := req.String("storage_id")
		target, targetErr := req.String("target_id")
		keeper, keeperErr := req.String("keeper_id")
		targetID, targetParse := strconv.ParseInt(target, 10, 64)
		keeperID, keeperParse := strconv.ParseInt(keeper, 10, 64)
		if storageErr != nil || targetErr != nil || keeperErr != nil || targetParse != nil || keeperParse != nil || pathLike(storageID) {
			return empty, ErrProtocol
		}
		return s.Plan(ctx, storageID, targetID, keeperID)
	}
	id, idErr := req.OpaqueID("plan_id")
	digest, digestErr := req.OpaqueID("digest")
	if idErr != nil || digestErr != nil {
		return empty, ErrProtocol
	}
	confirmation := ""
	if req.Op == "SOURCE_DIRECT_PURGE_EXECUTE" {
		var err error
		confirmation, err = req.String("confirmation")
		if err != nil {
			return empty, ErrProtocol
		}
	}
	return s.Advance(ctx, id, digest, strings.TrimPrefix(req.Op, "SOURCE_DIRECT_PURGE_"), confirmation)
}
