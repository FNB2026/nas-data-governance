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

// VerifyGovernanceSources re-reads only the selected duplicate-group members.
// Roots and member paths come from the open project's store; a MAS source lease
// must still prove a matching bookmark identity before fingerprint.Guarded is
// allowed to open any member. This is an observation, never file authority.
func (a *API) VerifyGovernanceSources(ctx context.Context, refs []domain.GovernanceSubject) map[string]domain.SourceFreshness {
	checks := make(map[string]domain.SourceFreshness, len(refs))
	set := func(id, status string) {
		checks[id] = domain.SourceFreshness{Status: status, CheckedAt: time.Now().UTC().Format(time.RFC3339Nano)}
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
			set(ref.GroupID, "UNVERIFIED")
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
			set(ref.GroupID, "SOURCE_UNAVAILABLE")
			continue
		}
		files, truncated, err := a.store.GovernanceSourceMembers(ctx, ref, governanceFreshnessMaxFiles)
		if err != nil {
			set(ref.GroupID, "UNVERIFIED")
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
			set(group.ref.GroupID, "SOURCE_UNAVAILABLE")
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
				set(group.ref.GroupID, "AUTHORIZATION_REQUIRED")
				continue
			}
			leases[group.ref.StorageID] = lease
		}
		status := "MATCH"
		for _, file := range group.files {
			if err := ctx.Err(); err != nil {
				status = freshnessStatusForLease(lease, err)
				break
			}
			got, readErr := fingerprint.GuardedContext(ctx, lease.Root, file)
			if readErr != nil {
				if errors.Is(readErr, fingerprint.ErrGuardedSourceChanged) {
					status = "SOURCE_CHANGED"
				} else {
					status = freshnessStatusForLease(lease, readErr)
				}
				break
			}
			if got != file.ContentSHA256 {
				status = "SOURCE_CHANGED"
				break
			}
		}
		set(group.ref.GroupID, status)
	}
	return checks
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
