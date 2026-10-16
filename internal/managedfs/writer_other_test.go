//go:build !linux

package managedfs

import (
	"context"
	"errors"
	"testing"
)

func TestWriterUnsupportedPlatformHasNoBootstrap(t *testing.T) {
	path := t.TempDir() + "/uncreated"
	if proof, err := Initialize(context.Background(), path); proof != (RootProof{}) || !errors.Is(err, ErrUnsupported) {
		t.Fatal("unsupported initialization did not refuse")
	}
	if w, err := OpenWriter(context.Background(), path, RootProof{}); w != nil || !errors.Is(err, ErrUnsupported) {
		t.Fatal("unsupported writer did not refuse")
	}
}
