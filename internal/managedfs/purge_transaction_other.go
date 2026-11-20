//go:build !linux

package managedfs

import "context"

func observePurgeTarget(context.Context, string, string) (PurgeTarget, error) {
	return PurgeTarget{}, ErrUnsupported
}

func executePurgeTransaction(context.Context, PurgeTransaction, func(string) error) (PurgeReceipt, error) {
	return PurgeReceipt{}, ErrUnsupported
}

func recoverPurgeTransaction(context.Context, PurgeTransaction) (PurgeReceipt, error) {
	return PurgeReceipt{}, ErrUnsupported
}
