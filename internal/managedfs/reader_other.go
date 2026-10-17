//go:build !linux

package managedfs

func OpenReader(string, RootProof) (Reader, error) { return nil, ErrUnsupported }
