//go:build darwin && cgo

package main

import (
	"errors"
	"testing"
)

func TestDarwinNativePrimitiveAndReplacementObservations(t *testing.T) {
	got, err := probe()
	if errors.Is(err, errPrimitiveUnavailable) {
		t.Skip("SDK/kernel does not provide the research primitive; no proof claimed")
	}
	if err != nil {
		t.Fatal("disposable native probe failed")
	}
	for name, value := range got {
		if name == "mount_boundary_proved" || name == "same_device_nested_mount_tested" {
			if value {
				t.Fatal("research must not claim untested mount proof")
			}
			continue
		}
		if !value {
			t.Errorf("native observation failed: %s", name)
		}
	}
}
