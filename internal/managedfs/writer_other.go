//go:build !linux

package managedfs

import "context"

func Initialize(context.Context, string) (RootProof, error)         { return RootProof{}, ErrUnsupported }
func OpenWriter(context.Context, string, RootProof) (Writer, error) { return nil, ErrUnsupported }
