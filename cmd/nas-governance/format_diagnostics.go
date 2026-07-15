package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"nas-data-governance/internal/formatdiag"
	"nas-data-governance/internal/store"
)

func runDiagnoseFormats(args []string) error {
	fs := flag.NewFlagSet("diagnose-formats", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database produced by scan/import and analyze (required)")
	out := fs.String("out", "./var/format-review-private.json", "private review report output")
	storageID := fs.String("storage-id", "", "filter by storage ID (optional)")
	largeMinimum := fs.Int64("large-unknown-min", 100<<20, "minimum unknown file size for private review")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return fmt.Errorf("--db is required")
	}
	if *largeMinimum < 1<<20 {
		return fmt.Errorf("--large-unknown-min must be at least 1 MiB")
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
	report := formatdiag.Build(files, formats, *largeMinimum, time.Now())
	if err := writePrivateJSON(*out, report); err != nil {
		return err
	}
	fmt.Printf("private format review created: %d large unknown, %d extension mismatch, %d metadata-gap format(s); paths omitted\n",
		report.Summary.LargeUnknown, report.Summary.ExtensionMismatches, report.Summary.FormatsWithMetadataGap)
	return nil
}

func writePrivateJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("private report output must not be a symbolic link")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, privateFileMode)
	if err != nil {
		return err
	}
	if err := f.Chmod(privateFileMode); err != nil {
		_ = f.Close()
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func validatePrivateReportOutput(inputPath, outPath string) error {
	inputAbs, err := filepath.Abs(inputPath)
	if err != nil {
		return err
	}
	outAbs, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	if inputAbs == outAbs {
		return fmt.Errorf("private report output must differ from its input")
	}
	inputInfo, err := os.Lstat(inputPath)
	if err != nil {
		return err
	}
	if inputInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("private report input must not be a symbolic link")
	}
	outInfo, err := os.Lstat(outPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if outInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("private report output must not be a symbolic link")
	}
	if os.SameFile(inputInfo, outInfo) {
		return fmt.Errorf("private report output must not alias its input")
	}
	return nil
}
