package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/fingerprint"
	idx "nas-data-governance/internal/index"
	"nas-data-governance/internal/planner"
	"nas-data-governance/internal/report"
	"nas-data-governance/internal/scanner"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "scan":
		err = runScan(os.Args[2:])
	case "duplicates":
		err = runDuplicates(os.Args[2:])
	case "plan":
		err = runPlan(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	root := fs.String("root", "", "root directory to scan")
	out := fs.String("out", "./var/index.jsonl", "index output")
	storage := fs.String("storage", "local", "storage identifier")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return fmt.Errorf("--root is required")
	}
	var files []domain.FileInstance
	err := scanner.Scan(scanner.Options{Root: *root, StorageID: *storage, ExcludedNames: scanner.DefaultExclusions()}, func(file domain.FileInstance) error {
		q, err := fingerprint.Quick(file.Path, file.Size)
		if err != nil {
			return err
		}
		file.QuickHash = q
		files = append(files, file)
		return nil
	})
	if err != nil {
		return err
	}
	bySizeQuick := map[string][]int{}
	for i, f := range files {
		bySizeQuick[fmt.Sprintf("%d:%s", f.Size, f.QuickHash)] = append(bySizeQuick[fmt.Sprintf("%d:%s", f.Size, f.QuickHash)], i)
	}
	for _, indexes := range bySizeQuick {
		if len(indexes) < 2 {
			continue
		}
		for _, i := range indexes {
			h, err := fingerprint.Full(files[i].Path)
			if err != nil {
				return err
			}
			files[i].ContentSHA256 = h
		}
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := idx.Write(*out, files); err != nil {
		return err
	}
	fmt.Printf("indexed %d files into %s (read-only scan)\n", len(files), *out)
	return nil
}

func runDuplicates(args []string) error {
	fs := flag.NewFlagSet("duplicates", flag.ContinueOnError)
	path := fs.String("index", "./var/index.jsonl", "index file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files, err := idx.Read(*path)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report.DuplicateGroups(files))
}

func runPlan(args []string) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	path := fs.String("index", "./var/index.jsonl", "index file")
	out := fs.String("out", "./var/plan.json", "draft plan output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files, err := idx.Read(*path)
	if err != nil {
		return err
	}
	plans := planner.Build(report.DuplicateGroups(files))
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(plans); err != nil {
		return err
	}
	fmt.Printf("created %d draft plans in %s (no filesystem operations executed)\n", len(plans), *out)
	return nil
}

func usage() { fmt.Fprintln(os.Stderr, "usage: nas-governance <scan|duplicates|plan> [options]") }
