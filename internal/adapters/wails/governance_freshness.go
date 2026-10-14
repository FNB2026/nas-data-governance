package wails

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/fileaccess"
	"github.com/FNB2026/nas-data-governance/internal/fingerprint"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
)

const (
	governanceFreshnessMaxGroups = 100
	governanceFreshnessMaxFiles  = 256
	governanceFreshnessMaxBytes  = int64(128 << 20)
)

type governanceFreshnessReader func(context.Context, string, domain.FileInstance) (string, error)

// verifyGovernanceSources re-reads only the selected duplicate-group members.
// Roots and member paths come from the open project's store. A MAS source
// lease must prove matching bookmark identity before GuardedContext opens a
// member. This is an observation, never file authority.
// It is deliberately not a method on the Wails-bound API, so the frontend can
// invoke it only through the governance preview/commit workflows.
func (a *API) verifyGovernanceSources(ctx context.Context, refs []domain.GovernanceSubject) map[string]domain.SourceFreshness {
	return a.verifyGovernanceSourcesWithReader(ctx, refs, fingerprint.GuardedContext)
}

func (a *API) verifyGovernanceSourcesWithReader(ctx context.Context, refs []domain.GovernanceSubject, read governanceFreshnessReader) map[string]domain.SourceFreshness {
	if read == nil {
		read = fingerprint.GuardedContext
	}
	checks := make(map[string]domain.SourceFreshness, len(refs))
	set := func(id, status string) {
		checks[id] = domain.SourceFreshness{Status: status, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	}
	setReason := func(id, status, reason string) {
		set(id, status)
		check := checks[id]
		check.ReasonCode = reason
		checks[id] = check
	}
	if !governanceFreshnessMountProofAvailable() {
		for _, ref := range refs {
			setReason(ref.GroupID, "UNVERIFIED", "SOURCE_PLATFORM_PROOF_UNAVAILABLE")
		}
		return checks
	}
	if len(refs) == 0 || len(refs) > governanceFreshnessMaxGroups || a.store == nil {
		for _, ref := range refs {
			set(ref.GroupID, "LIMIT_EXCEEDED")
		}
		return checks
	}
	storages, err := a.store.ListStorages(ctx)
	if err != nil {
		for _, ref := range refs {
			setReason(ref.GroupID, "UNVERIFIED", "SOURCE_CATALOG_UNAVAILABLE")
		}
		return checks
	}
	storageByID := make(map[string]domain.Storage, len(storages))
	for _, storage := range storages {
		storageByID[storage.ID] = storage
	}
	type selected struct {
		ref     domain.GovernanceSubject
		storage domain.Storage
		files   []domain.FileInstance
	}
	selectedGroups := make([]selected, 0, len(refs))
	for _, ref := range refs {
		storage, ok := storageByID[ref.StorageID]
		if !ok || storage.RootPath == "" {
			setReason(ref.GroupID, "SOURCE_UNAVAILABLE", "SOURCE_REGISTRATION_UNAVAILABLE")
			continue
		}
		files, truncated, err := a.store.GovernanceSourceMembers(ctx, ref, governanceFreshnessMaxFiles)
		if err != nil {
			setReason(ref.GroupID, "UNVERIFIED", "SOURCE_CATALOG_UNAVAILABLE")
			continue
		}
		if truncated {
			set(ref.GroupID, "LIMIT_EXCEEDED")
			continue
		}
		selectedGroups = append(selectedGroups, selected{ref: ref, storage: storage, files: files})
	}
	fileGroups := make([][]domain.FileInstance, 0, len(selectedGroups))
	for _, group := range selectedGroups {
		fileGroups = append(fileGroups, group.files)
	}
	if governanceFreshnessBudgetExceeded(fileGroups) {
		for _, group := range selectedGroups {
			set(group.ref.GroupID, "LIMIT_EXCEEDED")
		}
		return checks
	}

	leases := make(map[string]*fileaccess.Lease)
	defer func() {
		for _, lease := range leases {
			lease.Release()
		}
	}()
	for _, group := range selectedGroups {
		if _, done := checks[group.ref.GroupID]; done {
			continue
		}
		root := filepath.Clean(group.storage.RootPath)
		if !filepath.IsAbs(root) {
			setReason(group.ref.GroupID, "SOURCE_UNAVAILABLE", "SOURCE_REGISTRATION_UNAVAILABLE")
			continue
		}
		lease := leases[group.ref.StorageID]
		if lease == nil {
			lease, err = a.sourceLeaseContext(ctx, root)
			if err != nil {
				set(group.ref.GroupID, freshnessStatusForLeaseError(err))
				continue
			}
			if filepath.Clean(lease.Root) != root {
				lease.Release()
				setReason(group.ref.GroupID, "AUTHORIZATION_REQUIRED", "SOURCE_AUTHORIZATION_MISMATCH")
				continue
			}
			leases[group.ref.StorageID] = lease
		}
		status := "MATCH"
		reason := ""
		for _, file := range group.files {
			if err := ctx.Err(); err != nil {
				status = freshnessStatusForLease(lease, err)
				reason = freshnessReasonForRead(lease, err)
				break
			}
			got, readErr := read(ctx, lease.Root, file)
			if readErr != nil {
				reason = freshnessReasonForRead(lease, readErr)
				if errors.Is(readErr, fingerprint.ErrGuardedSourceChanged) {
					status = "SOURCE_CHANGED"
				} else {
					status = freshnessStatusForLease(lease, readErr)
				}
				break
			}
			if got != file.ContentSHA256 {
				status = "SOURCE_CHANGED"
				reason = "SOURCE_CONTENT_CHANGED"
				break
			}
		}
		setReason(group.ref.GroupID, status, reason)
	}
	return checks
}

// governanceSourceVerifier is injected into the application service at the
// composition root. It is not bound to Wails, so raw verification cannot be
// called as a frontend API outside the governed preview/commit flow.
type governanceSourceVerifier struct {
	api  *API
	read governanceFreshnessReader // nil in production; injectable only by package tests
}

func (v governanceSourceVerifier) VerifyGovernanceSources(ctx context.Context, refs []domain.GovernanceSubject) map[string]domain.SourceFreshness {
	if v.api == nil {
		checks := make(map[string]domain.SourceFreshness, len(refs))
		for _, ref := range refs {
			checks[ref.GroupID] = domain.SourceFreshness{Status: "UNVERIFIED", CheckedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		}
		return checks
	}
	if v.read == nil {
		return v.api.verifyGovernanceSources(ctx, refs)
	}
	return v.api.verifyGovernanceSourcesWithReader(ctx, refs, v.read)
}

func governanceFreshnessBudgetExceeded(groups [][]domain.FileInstance) bool {
	totalFiles := 0
	totalBytes := int64(0)
	for _, group := range groups {
		for _, file := range group {
			totalFiles++
			if totalFiles > governanceFreshnessMaxFiles || file.Size < 0 || file.Size > governanceFreshnessMaxBytes {
				return true
			}
			if totalBytes > governanceFreshnessMaxBytes-file.Size {
				return true
			}
			totalBytes += file.Size
		}
	}
	return false
}

func freshnessStatusForLeaseError(err error) string {
	switch {
	case errors.Is(err, fileaccess.ErrAuthorizationRequired):
		return "AUTHORIZATION_REQUIRED"
	case errors.Is(err, fileaccess.ErrNetworkPaused):
		return "NETWORK_PAUSED"
	case errors.Is(err, fileaccess.ErrSourceUnavailable):
		return "SOURCE_UNAVAILABLE"
	default:
		return "UNVERIFIED"
	}
}

func freshnessStatusForLease(lease *fileaccess.Lease, err error) string {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// A caller cancellation or our bounded verification deadline says the
		// observation did not finish. It does not establish that the source or
		// network is unavailable, so fail closed without asserting either.
		return "UNVERIFIED"
	case errors.Is(err, fingerprint.ErrGuardedMountBoundaryUnverified):
		return "UNVERIFIED"
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return "AUTHORIZATION_REQUIRED"
	case lease != nil && lease.Network && scanner.IsNetworkUnavailableError(err):
		return "NETWORK_PAUSED"
	case errors.Is(err, os.ErrNotExist):
		return "SOURCE_UNAVAILABLE"
	default:
		return "SOURCE_UNAVAILABLE"
	}
}

// Classification is based on typed evidence, never string matching. In
// particular, permission/guard failures on a network lease are not outages.
func freshnessReasonForRead(lease *fileaccess.Lease, err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "SOURCE_VERIFICATION_CANCELLED"
	case errors.Is(err, context.DeadlineExceeded):
		return "SOURCE_VERIFICATION_TIMEOUT"
	case errors.Is(err, fingerprint.ErrGuardedMountBoundaryUnverified):
		return "SOURCE_PLATFORM_PROOF_UNAVAILABLE"
	case errors.Is(err, fingerprint.ErrGuardedSourceChanged):
		// The reader does not separately expose inode vs size/time changes.
		return "SOURCE_IDENTITY_OR_METADATA_CHANGED"
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return "SOURCE_PERMISSION_DENIED"
	case errors.Is(err, syscall.EXDEV):
		return "SOURCE_MOUNT_BOUNDARY_REJECTED"
	case errors.Is(err, syscall.ELOOP):
		return "SOURCE_SYMLINK_REJECTED"
	case errors.Is(err, syscall.ENOSYS):
		return "SOURCE_VERIFICATION_API_UNAVAILABLE"
	case lease != nil && lease.Network && scanner.IsNetworkUnavailableError(err):
		return "NETWORK_PAUSED"
	case errors.Is(err, os.ErrNotExist):
		return "SOURCE_FILE_UNAVAILABLE"
	default:
		return "SOURCE_READ_FAILED"
	}
}
