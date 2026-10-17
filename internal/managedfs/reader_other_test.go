//go:build !linux

package managedfs

import "testing"

func TestReaderUnsupportedPlatformCannotProduceProof(t *testing.T) {
	reader, err := OpenReader("unused", RootProof{})
	if reader != nil || err != ErrUnsupported {
		t.Fatal("unsupported platform produced a private reader")
	}
}
