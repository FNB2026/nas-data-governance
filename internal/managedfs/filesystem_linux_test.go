//go:build linux

package managedfs

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestWriterFilesystemRequiresExactMountTypeNotSharedMagic(t *testing.T) {
	for _, fixture := range []struct {
		kind  string
		magic uint64
		valid bool
	}{
		{"ext4", 0xef53, true}, {"btrfs", 0x9123683e, true}, {"xfs", 0x58465342, true},
		{"ext2", 0xef53, false}, {"ext3", 0xef53, false}, {"ext4", 0x9123683e, false},
		{"btrfs", 0xef53, false}, {"xfs", 0xef53, false}, {"overlay", 0x794c7630, false},
	} {
		t.Run(fmt.Sprintf("%s/%x", fixture.kind, fixture.magic), func(t *testing.T) {
			raw := []byte("47 1 8:1 / / rw shared:2 future:field - " + fixture.kind + " /dev/test rw\n")
			kind, err := mountProfile(raw, 47, 8, 1, fixture.magic)
			if fixture.valid {
				if kind != fixture.kind || err != nil {
					t.Fatal("valid filesystem type rejected")
				}
			} else if kind != "" || !errors.Is(err, ErrUnsupported) {
				t.Fatal("unproved filesystem type accepted")
			}
		})
	}
}

func TestWriterFilesystemRejectsAmbiguousOrTruncatedKernelMetadata(t *testing.T) {
	valid := "47 1 8:1 / / rw - ext4 /dev/test rw\n"
	for _, raw := range []string{"", strings.TrimSuffix(valid, "\n"), valid + valid,
		"48 1 8:1 / / rw - ext4 /dev/test rw\n", strings.Replace(valid, "8:1", "8:2", 1),
		strings.Replace(valid, "47 ", "047 ", 1), strings.Replace(valid, "1 8:1", "bad 8:1", 1),
		strings.Replace(valid, "8:1", "8:bad", 1), strings.Replace(valid, " - ", " - - ", 1),
		strings.Replace(valid, " - ", " ", 1), strings.Replace(valid, " / / ", " relative / ", 1),
		strings.Replace(valid, " rw\n", " rw extra\n", 1), valid + "\n", strings.Repeat("x", maxMountInfoBytes+1) + "\n"} {
		if kind, err := mountProfile([]byte(raw), 47, 8, 1, 0xef53); kind != "" || !errors.Is(err, ErrUnsupported) {
			t.Fatal("malformed mount metadata accepted")
		}
	}
	if kind, err := mountProfile([]byte(valid), 0, 8, 1, 0xef53); kind != "" || !errors.Is(err, ErrUnsupported) {
		t.Fatal("missing mount ID accepted")
	}
}
