//go:build darwin && cgo

// A disposable research executable, deliberately outside the production reader.
// It accepts no source path and never accesses an existing NDG project.
package main

/*
#include <fcntl.h>
#ifdef O_RESOLVE_BENEATH
#define NDG_HAS_BENEATH 1
#define NDG_BENEATH_FLAG O_RESOLVE_BENEATH
#else
#define NDG_HAS_BENEATH 0
#define NDG_BENEATH_FLAG 0
#endif
*/
import "C"

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const flags = unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW_ANY | int(C.NDG_BENEATH_FLAG) | unix.O_NONBLOCK

var errPrimitiveUnavailable = errors.New("native research primitive unavailable")

func main() {
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "probe accepts no source arguments")
		os.Exit(2)
	}
	result, err := probe()
	if errors.Is(err, errPrimitiveUnavailable) {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]bool{"primitive_available": false, "mount_boundary_proved": false})
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "disposable probe failed; private path omitted")
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
}

func probe() (map[string]bool, error) {
	// Older SDKs may not declare the flag. Never run a weaker lookup in that case.
	if C.NDG_HAS_BENEATH == 0 {
		return nil, errPrimitiveUnavailable
	}
	fixture, err := os.MkdirTemp("", "ndg-darwin-proof-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(fixture)
	fixture, err = filepath.EvalSymlinks(fixture)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(fixture, "root")
	if err := os.MkdirAll(filepath.Join(root, "child"), 0700); err != nil {
		return nil, err
	}
	data := []byte("synthetic-native-proof\n")
	if err := os.WriteFile(filepath.Join(root, "child", "member"), data, 0600); err != nil {
		return nil, err
	}
	if err := os.Symlink("child/member", filepath.Join(root, "leaf")); err != nil {
		return nil, err
	}
	if err := os.Symlink("child", filepath.Join(root, "intermediate")); err != nil {
		return nil, err
	}
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW_ANY, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	open := func(base int, path string) (int, error) { return unix.Openat(base, path, flags, 0) }
	result := map[string]bool{}
	alias := filepath.Join(fixture, "root-alias")
	if err := os.Symlink("root", alias); err != nil {
		return nil, err
	}
	for label, path := range map[string]string{"root_symlink_rejected": alias, "root_ancestor_symlink_rejected": filepath.Join(alias, "child")} {
		candidate, e := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW_ANY, 0)
		if e == nil {
			_ = unix.Close(candidate)
		}
		result[label] = e != nil
	}
	member, err := open(fd, "child/member")
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.ENOTSUP) {
		return nil, errPrimitiveUnavailable
	}
	if err != nil {
		return nil, err
	}
	buf := make([]byte, len(data))
	n, err := unix.Read(member, buf)
	_ = unix.Close(member)
	if err != nil || n != len(data) || string(buf) != string(data) {
		return nil, errors.New("read mismatch")
	}
	result["ordinary_primitive_read"] = true
	for label, path := range map[string]string{"leaf_symlink_rejected": "leaf", "intermediate_symlink_rejected": "intermediate/member", "dotdot_escape_rejected": "../root/child/member", "absolute_path_rejected": filepath.Join(root, "child/member")} {
		candidate, e := open(fd, path)
		if e == nil {
			_ = unix.Close(candidate)
		}
		result[label] = e != nil
	}
	// A held child descriptor alone does not prove current ancestry beneath root.
	child, err := unix.Openat(fd, "child", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW_ANY, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(child)
	if err := os.Rename(filepath.Join(root, "child"), filepath.Join(fixture, "moved-child")); err != nil {
		return nil, err
	}
	member, err = open(child, "member")
	result["child_fd_still_opens_after_move_outside_root"] = err == nil
	if err == nil {
		_ = unix.Close(member)
	}
	member, err = open(fd, "child/member")
	result["whole_root_lookup_rejects_moved_child"] = err != nil
	if err == nil {
		_ = unix.Close(member)
	}
	if err := os.Rename(filepath.Join(fixture, "moved-child"), filepath.Join(root, "child")); err != nil {
		return nil, err
	}
	before, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(root, filepath.Join(fixture, "original-root")); err != nil {
		return nil, err
	}
	if err := os.Mkdir(root, 0700); err != nil {
		return nil, err
	}
	during, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	member, err = open(fd, "child/member")
	result["held_root_reads_original_after_path_replacement"] = err == nil
	if err == nil {
		_ = unix.Close(member)
	}
	result["root_replacement_identity_detectable"] = !os.SameFile(before, during)
	if err := os.Rename(root, filepath.Join(fixture, "replacement-root")); err != nil {
		return nil, err
	}
	if err := os.Rename(filepath.Join(fixture, "original-root"), root); err != nil {
		return nil, err
	}
	after, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	result["root_aba_endpoint_identity_equal"] = os.SameFile(before, after)
	// No mount is created by this executable; an absent same-device test is not PASS.
	result["mount_boundary_proved"] = false
	result["same_device_nested_mount_tested"] = false
	return result, nil
}
