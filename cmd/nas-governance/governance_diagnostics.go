package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"nas-data-governance/internal/governancediag"
	"nas-data-governance/internal/store"
)

func runDiagnoseGovernance(args []string) error {
	fs := flag.NewFlagSet("diagnose-governance", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database produced by scan/import and analyze (required)")
	out := fs.String("out", "./var/governance-review-private.json", "private P2 review report output")
	storageID := fs.String("storage-id", "", "filter by storage ID (optional)")
	largeMediaMinimum := fs.Int64("large-media-min", 100<<20, "minimum audio/video size for detailed review")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return fmt.Errorf("--db is required")
	}
	if *largeMediaMinimum < 1<<20 {
		return fmt.Errorf("--large-media-min must be at least 1 MiB")
	}
	if err := validatePrivateReportOutput(*dbPath, *out); err != nil {
		return err
	}
	ctx := context.Background()
	st, err := store.OpenReadOnly(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	files, err := st.ListFiles(ctx, *storageID)
	if err != nil {
		return err
	}
	formats, err := st.ListFormats(ctx, *storageID)
	if err != nil {
		return err
	}
	byKey := make(map[string]int, len(files))
	for i := range files {
		byKey[files[i].StorageID+"\x00"+files[i].Path] = i
	}
	for _, record := range formats {
		if i, ok := byKey[record.StorageID+"\x00"+record.Path]; ok {
			files[i].Format = record.Info
		}
	}
	review := governancediag.Build(files, *largeMediaMinimum, time.Now())
	if review.Summary.NonDraftPlans != 0 || review.ExecutionAuthorized {
		return fmt.Errorf("governance diagnostic refused unsafe non-draft result")
	}
	if err := writePrivateJSON(*out, review); err != nil {
		return err
	}
	fmt.Printf("private P2 review created: %d duplicate group(s), %d zero-byte file(s), %d large media file(s), %d missing format row(s); all plans DRAFT, paths omitted\n",
		review.Summary.DuplicateGroups, review.Summary.ZeroByteFiles, review.Summary.LargeMediaFiles, review.Summary.MissingFormatRows)
	return nil
}
