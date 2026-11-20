package managedfs

import "context"

// PurgeIdentity is the physical identity observed through a confined descriptor.
type PurgeIdentity struct {
	Device uint64
	Inode  uint64
	Mount  uint64
}

// PurgeTarget binds a plan to the opened root, parent and exact file contents.
type PurgeTarget struct {
	Root, Parent, File PurgeIdentity
	Relative           string
	Size               uint64
	Mode               uint16
	Links              uint32
	MTimeSec           int64
	MTimeNsec          uint32
	CTimeSec           int64
	CTimeNsec          uint32
	SHA256             string
}

// PurgeTransaction is supplied by the worker after durable, per-file approval.
// RootPath is worker configuration, not a caller-provided execution pathname.
type PurgeTransaction struct {
	ID, ApprovalSHA256, RootPath string
	Target                       PurgeTarget
	Keeper                       *PurgeTarget
}

type PurgeReceipt struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

// ObservePurgeTarget reads an exact regular file under a confined root.
func ObservePurgeTarget(ctx context.Context, root, relative string) (PurgeTarget, error) {
	return observePurgeTarget(ctx, root, relative)
}

// ExecutePurgeTransaction is the filesystem layer. The worker must enforce
// approval, Dry Run and operation-specific policy before invoking it.
func ExecutePurgeTransaction(ctx context.Context, plan PurgeTransaction) (PurgeReceipt, error) {
	return executePurgeTransaction(ctx, plan, nil)
}

// RecoverPurgeTransaction restores an identity-verified object before commit.
func RecoverPurgeTransaction(ctx context.Context, plan PurgeTransaction) (PurgeReceipt, error) {
	return recoverPurgeTransaction(ctx, plan)
}

// DryRunPurgeTransaction checks all present filesystem prerequisites without
// changing the source, staging directory, or journal.
func DryRunPurgeTransaction(ctx context.Context, plan PurgeTransaction) error {
	return dryRunPurgeTransaction(ctx, plan)
}
