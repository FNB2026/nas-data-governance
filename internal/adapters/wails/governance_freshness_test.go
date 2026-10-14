package wails

import (
	"context"
	"errors"
	"math"
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/fileaccess"
	"github.com/FNB2026/nas-data-governance/internal/fingerprint"
)

func TestGovernanceFreshnessBudgetBoundaries(t *testing.T) {
	files := make([]domain.FileInstance, governanceFreshnessMaxFiles)
	if governanceFreshnessBudgetExceeded([][]domain.FileInstance{files}) {
		t.Fatal("exact file-count budget should be accepted")
	}
	files = append(files, domain.FileInstance{})
	if !governanceFreshnessBudgetExceeded([][]domain.FileInstance{files}) {
		t.Fatal("file-count budget must fail closed")
	}
	if governanceFreshnessBudgetExceeded([][]domain.FileInstance{{{Size: governanceFreshnessMaxBytes}}}) {
		t.Fatal("exact byte budget should be accepted")
	}
	if !governanceFreshnessBudgetExceeded([][]domain.FileInstance{{{Size: governanceFreshnessMaxBytes}, {Size: 1}}}) {
		t.Fatal("aggregate byte budget must fail closed")
	}
	for _, size := range []int64{-1, governanceFreshnessMaxBytes + 1, math.MaxInt64} {
		if !governanceFreshnessBudgetExceeded([][]domain.FileInstance{{{Size: size}}}) {
			t.Fatalf("invalid or oversized declared length %d was accepted", size)
		}
	}
}

func TestGovernanceFreshnessNetworkClassificationRequiresKnownNetworkEvidence(t *testing.T) {
	networkLease := &fileaccess.Lease{Network: true}
	localLease := &fileaccess.Lease{}
	for _, tc := range []struct {
		name  string
		lease *fileaccess.Lease
		err   error
		want  string
	}{
		{name: "known network disappeared", lease: networkLease, err: os.ErrNotExist, want: "NETWORK_PAUSED"},
		{name: "local source disappeared", lease: localLease, err: os.ErrNotExist, want: "SOURCE_UNAVAILABLE"},
		{name: "network path guard rejection", lease: networkLease, err: errors.New("cross-device path rejected"), want: "SOURCE_UNAVAILABLE"},
		{name: "openat2 unavailable", lease: localLease, err: unix.ENOSYS, want: "SOURCE_UNAVAILABLE"},
		{name: "openat2 rejected mount crossing", lease: networkLease, err: unix.EXDEV, want: "SOURCE_UNAVAILABLE"},
		{name: "platform cannot prove mount boundary", lease: localLease, err: fingerprint.ErrGuardedMountBoundaryUnverified, want: "UNVERIFIED"},
		{name: "authorization failure wins", lease: networkLease, err: syscall.EACCES, want: "AUTHORIZATION_REQUIRED"},
		{name: "network cancellation", lease: networkLease, err: context.Canceled, want: "UNVERIFIED"},
		{name: "network verification deadline", lease: networkLease, err: context.DeadlineExceeded, want: "UNVERIFIED"},
		{name: "local cancellation", lease: localLease, err: context.Canceled, want: "UNVERIFIED"},
		{name: "local verification deadline", lease: localLease, err: context.DeadlineExceeded, want: "UNVERIFIED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := freshnessStatusForLease(tc.lease, tc.err); got != tc.want {
				t.Fatalf("classification = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGovernanceFreshnessCauseUsesTypedEvidenceOnly(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		network bool
		want    string
	}{
		{"method missing", fingerprint.ErrGuardedMountBoundaryUnverified, true, "SOURCE_PLATFORM_PROOF_UNAVAILABLE"},
		{"cancelled", context.Canceled, true, "SOURCE_VERIFICATION_CANCELLED"},
		{"deadline", context.DeadlineExceeded, true, "SOURCE_VERIFICATION_TIMEOUT"},
		{"permission", &os.PathError{Op: "open", Path: "/synthetic-private-marker", Err: syscall.EACCES}, true, "SOURCE_PERMISSION_DENIED"},
		{"mount crossing", syscall.EXDEV, true, "SOURCE_MOUNT_BOUNDARY_REJECTED"},
		{"symlink", syscall.ELOOP, true, "SOURCE_SYMLINK_REJECTED"},
		{"kernel method missing", syscall.ENOSYS, false, "SOURCE_VERIFICATION_API_UNAVAILABLE"},
		{"typed identity or metadata change", fingerprint.ErrGuardedSourceChanged, true, "SOURCE_IDENTITY_OR_METADATA_CHANGED"},
		{"network unavailable", syscall.ENETDOWN, true, "NETWORK_PAUSED"},
		{"local missing", os.ErrNotExist, false, "SOURCE_FILE_UNAVAILABLE"},
		{"network missing is ambiguous", os.ErrNotExist, true, "NETWORK_PAUSED"},
		{"unknown error text must not become evidence", errors.New("ENOENT bookmark expired volume UUID changed /private-marker"), true, "SOURCE_READ_FAILED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := freshnessReasonForRead(&fileaccess.Lease{Network: tc.network}, tc.err); got != tc.want {
				t.Fatalf("cause = %q, want %q", got, tc.want)
			}
		})
	}
}
