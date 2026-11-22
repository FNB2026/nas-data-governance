//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	idx "github.com/FNB2026/nas-data-governance/internal/index"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func TestAnalyzeLinuxDefaultBoundary(t *testing.T) {
	for _, scenario := range []string{"valid", "no_root", "explicit", "explicit_registered", "explicit_mismatch", "file_changed", "unproven_identity", "outside_index_entry", "ancestor_symlink", "root_replaced", "same_device_nested_bind"} {
		t.Run(scenario, func(t *testing.T) {
			temp := t.TempDir()
			root, outside := filepath.Join(temp, "source"), filepath.Join(temp, "outside")
			nested := filepath.Join(root, "nested", "member")
			for _, dir := range []string{nested, outside} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			insideName := filepath.Join(nested, "fixture.pdf")
			sentinel := filepath.Join(outside, "fixture.pdf")
			contents := []byte("%PDF-1.4\n synthetic file to verify analyzer boundaries\n")
			for _, file := range []string{insideName, sentinel} {
				if err := os.WriteFile(file, contents, 0600); err != nil {
					t.Fatal(err)
				}
			}
			index, db, report := filepath.Join(temp, "scan.jsonl"), filepath.Join(temp, "test.db"), filepath.Join(temp, "report.json")
			if err := runScan([]string{"--root", root, "--db", db, "--out", index, "--storage", "synth"}); err != nil {
				t.Fatal(err)
			}
			files, err := idx.Read(index)
			if err != nil || len(files) != 1 || files[0].Path != insideName {
				t.Fatal("unexpected synthetic index", err)
			}
			args := []string{"--index", index, "--out", report}
			switch scenario {
			case "no_root":
				if err := runAnalyze(args); err == nil {
					t.Fatal("unmanaged index selected its own task root")
				}
				if _, err := os.Stat(report); !os.IsNotExist(err) {
					t.Fatal("unexpected report for unproven root", err)
				}
				return
			case "explicit":
				args = append(args, "--confined-root", root)
			case "explicit_registered":
				args = append(args, "--db", db, "--confined-root", root)
			case "explicit_mismatch":
				args = append(args, "--db", db, "--confined-root", outside)
			default:
				args = append(args, "--db", db)
			}
			allowed := scenario == "valid" || scenario == "explicit" || scenario == "explicit_registered"

			watch, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
			if err != nil {
				t.Fatal(err)
			}
			defer unix.Close(watch)
			if _, err := unix.InotifyAddWatch(watch, sentinel, unix.IN_OPEN|unix.IN_ACCESS); err != nil {
				t.Fatal(err)
			}
			sentinelBefore, err := os.Stat(sentinel)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "file_changed":
				if err := os.WriteFile(insideName, []byte("changed size and contents"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unproven_identity":
				files[0].Physical.Reliable = false
				files[0].Device, files[0].Inode = 0, 0
				if err := idx.Write(index, files); err != nil {
					t.Fatal(err)
				}
			case "outside_index_entry":
				files[0].Path = sentinel
				if err := idx.Write(index, files); err != nil {
					t.Fatal(err)
				}
			case "ancestor_symlink":
				if err := os.Rename(nested, filepath.Join(temp, "old")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, nested); err != nil {
					t.Fatal(err)
				}
			case "root_replaced":
				if err := os.Rename(root, filepath.Join(temp, "old")); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(nested, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(sentinel, insideName); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(insideName, files[0].ModifiedAt, files[0].ModifiedAt); err != nil {
					t.Fatal(err)
				}
			case "same_device_nested_bind":
				if os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("RUNNER_ENVIRONMENT") != "github-hosted" {
					t.Skip("mount fixture requires a disposable GitHub runner")
				}
				if out, err := exec.Command("sudo", "-n", "mount", "--bind", "--", outside, nested).CombinedOutput(); err != nil {
					t.Fatal("fixture bind mount failed", err, string(out))
				}
				t.Cleanup(func() {
					if out, err := exec.Command("sudo", "-n", "umount", "--", nested).CombinedOutput(); err != nil {
						t.Error("fixture unmount failed", err, string(out))
					}
				})
				var parent, member unix.Stat_t
				if unix.Stat(root, &parent) != nil || unix.Stat(nested, &member) != nil || parent.Dev != member.Dev {
					t.Fatal("fixture did not establish a same-device nested mount")
				}
				secondIndex := filepath.Join(temp, "check.jsonl")
				if err := runScan([]string{"--root", root, "--out", secondIndex, "--storage", "mount-check", "--full"}); err != nil {
					t.Fatal(err)
				}
				if scanned, err := idx.Read(secondIndex); err != nil || len(scanned) != 0 {
					t.Fatal("nested fixture entered rescan index", err)
				}
			}

			if err := runAnalyze(args); scenario == "explicit_mismatch" {
				if err == nil {
					t.Fatal("explicit root overrode the project storage")
				}
				if _, err := os.Stat(report); !os.IsNotExist(err) {
					t.Fatal("unexpected report for mismatched project root", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if scenario != "explicit_mismatch" {
				var entries []formatReportEntry
				readJSONFile(t, report, &entries)
				if len(entries) != 1 {
					t.Fatalf("unexpected report cardinality %d", len(entries))
				}
				if allowed {
					if entries[0].Error != "" || entries[0].Format.Format != "pdf" {
						t.Fatal("in-scope format analysis failed")
					}
				} else if entries[0].Path != "" || entries[0].Format.Format != "" || entries[0].Error == "" {
					t.Fatal("refused entry carried file data")
				}
			}
			var event [1024]byte
			if n, err := unix.Read(watch, event[:]); n > 0 || !errors.Is(err, unix.EAGAIN) {
				t.Fatal("out-of-scope sentinel received an open/access event", n, err)
			}
			if raw, err := os.ReadFile(sentinel); err != nil || string(raw) != string(contents) {
				t.Fatal("outside sentinel bytes changed", err)
			}
			if info, err := os.Stat(sentinel); err != nil || !os.SameFile(sentinelBefore, info) {
				t.Fatal("outside sentinel identity changed", err)
			}
			if !allowed {
				project, err := store.Open(context.Background(), db)
				if err != nil {
					t.Fatal(err)
				}
				defer project.Close()
				records, err := project.ListFormats(context.Background(), "synth")
				if err != nil || len(records) != 0 {
					t.Fatal("refused analysis entered the project", err)
				}
			}
		})
	}
}
