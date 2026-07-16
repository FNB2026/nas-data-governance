package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/pathdiag"
)

var legacyHashErrorLine = regexp.MustCompile(`^warning: hash error: open (.*): no such file or directory$`)

func runDiagnosePaths(args []string) error {
	fs := flag.NewFlagSet("diagnose-paths", flag.ContinueOnError)
	root := fs.String("root", "", "read-only task root (required)")
	legacyLog := fs.String("legacy-log", "", "private legacy scan log to inspect (optional)")
	failuresManifest := fs.String("failures-manifest", "", "private hash failure manifest produced by scan (optional)")
	out := fs.String("out", "./var/path-compat-private.json", "private compatibility report output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return fmt.Errorf("--root is required")
	}
	if *legacyLog == "" && *failuresManifest == "" {
		return fmt.Errorf("--legacy-log or --failures-manifest is required")
	}
	if *legacyLog != "" && *failuresManifest != "" {
		return fmt.Errorf("--legacy-log and --failures-manifest are mutually exclusive")
	}
	if *legacyLog != "" {
		if err := validatePrivateReportOutput(*legacyLog, *out); err != nil {
			return err
		}
	}
	if *failuresManifest != "" {
		if err := validatePrivateReportOutput(*failuresManifest, *out); err != nil {
			return err
		}
	}
	var paths []string
	var err error
	if *failuresManifest != "" {
		paths, err = readHashFailureManifestPaths(*failuresManifest)
	} else {
		paths, err = readLegacyHashErrorPaths(*legacyLog)
	}
	if err != nil {
		return err
	}
	report, err := pathdiag.Build(*root, paths, time.Now())
	if err != nil {
		return err
	}
	if report.ExecutionAuthorized {
		return fmt.Errorf("path diagnostic refused unsafe executable result")
	}
	if err := writePrivateJSON(*out, report); err != nil {
		return err
	}
	fmt.Printf("private path compatibility review created: %d candidate(s), %d exact-now, %d normalized variant/match, %d listable-not-openable, %d no-current-match; source paths omitted\n",
		report.Summary.Candidates, report.Summary.ExactNowExists,
		report.Summary.NFCVariantExists+report.Summary.NFDVariantExists+report.Summary.NormalizedSiblingMatches,
		report.Summary.ListableNotOpenable,
		report.Summary.NoCurrentMatch)
	return nil
}

// hashFailureEntry is a subset of the JSONL record written by scan's
// --hash-failures-out manifest. Only the path field is consumed.
type hashFailureEntry struct {
	Path string `json:"path"`
}

// readHashFailureManifestPaths reads the private hash-failure manifest
// produced by scan and returns the unique set of source paths. The manifest
// is 0600; this function does not echo paths to ordinary logs.
func readHashFailureManifestPaths(path string) ([]string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("failures manifest must be an available regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open private failures manifest failed")
	}
	defer f.Close()
	var paths []string
	seen := make(map[string]struct{})
	dec := json.NewDecoder(f)
	for {
		var e hashFailureEntry
		if err := dec.Decode(&e); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("read private failures manifest failed")
		}
		if e.Path == "" {
			continue
		}
		if _, ok := seen[e.Path]; ok {
			continue
		}
		seen[e.Path] = struct{}{}
		paths = append(paths, e.Path)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("failures manifest contains no path entries")
	}
	return paths, nil
}

func readLegacyHashErrorPaths(path string) ([]string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("legacy log must be an available regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open private legacy log failed")
	}
	defer f.Close()
	var paths []string
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for s.Scan() {
		match := legacyHashErrorLine.FindStringSubmatch(s.Text())
		if len(match) == 2 {
			paths = append(paths, match[1])
		}
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("read private legacy log failed")
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("legacy log contains no supported hash-error entries")
	}
	return paths, nil
}
