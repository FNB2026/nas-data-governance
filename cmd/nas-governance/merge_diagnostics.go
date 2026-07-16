package main

import (
	"flag"
	"fmt"
	"time"

	idx "github.com/FNB2026/nas-data-governance/internal/index"
	"github.com/FNB2026/nas-data-governance/internal/merge"
)

func runDiagnoseMerges(args []string) error {
	fs := flag.NewFlagSet("diagnose-merges", flag.ContinueOnError)
	indexPath := fs.String("index", "", "scan index JSONL (required)")
	out := fs.String("out", "./var/merge-review-private.json", "private merge gate report output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *indexPath == "" {
		return fmt.Errorf("--index is required")
	}
	if err := validatePrivateReportOutput(*indexPath, *out); err != nil {
		return err
	}
	files, err := idx.Read(*indexPath)
	if err != nil {
		return err
	}
	report := merge.Diagnose(files, time.Now())
	if report.ExecutionAuthorized {
		return fmt.Errorf("merge diagnostic refused unsafe executable result")
	}
	if err := writePrivateJSON(*out, report); err != nil {
		return err
	}
	fmt.Printf("private merge review created: %d directories, %d name-similar pair(s), %d at >=0.25, %d at current 0.50 threshold; source paths omitted\n",
		report.Summary.Directories, report.Summary.NameSimilarPairs,
		report.Summary.OverlapAtLeast25, report.Summary.OverlapAtLeast50)
	return nil
}
