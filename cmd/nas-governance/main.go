package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/assets"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/format"
	idx "github.com/FNB2026/nas-data-governance/internal/index"
	"github.com/FNB2026/nas-data-governance/internal/learning"
	"github.com/FNB2026/nas-data-governance/internal/merge"
	"github.com/FNB2026/nas-data-governance/internal/privatefs"
	"github.com/FNB2026/nas-data-governance/internal/relations"
	"github.com/FNB2026/nas-data-governance/internal/runner"
	"github.com/FNB2026/nas-data-governance/internal/scanner"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
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
	case "import-index":
		err = runImportIndex(os.Args[2:])
	case "retry-hashes":
		err = runRetryHashes(os.Args[2:])
	case "duplicates":
		err = runDuplicates(os.Args[2:])
	case "plan":
		err = runPlan(os.Args[2:])
	case "approve":
		err = runApprove(os.Args[2:])
	case "execute":
		err = runExecute(os.Args[2:])
	case "quarantine":
		err = runQuarantine(os.Args[2:])
	case "purge":
		err = runPurge(os.Args[2:])
	case "analyze":
		err = runAnalyze(os.Args[2:])
	case "diagnose-formats":
		err = runDiagnoseFormats(os.Args[2:])
	case "diagnose-governance":
		err = runDiagnoseGovernance(os.Args[2:])
	case "diagnose-paths":
		err = runDiagnosePaths(os.Args[2:])
	case "diagnose-merges":
		err = runDiagnoseMerges(os.Args[2:])
	case "group":
		err = runGroup(os.Args[2:])
	case "relations":
		err = runRelations(os.Args[2:])
	case "merge":
		err = runMerge(os.Args[2:])
	case "learn":
		err = runLearn(os.Args[2:])
	case "rules":
		err = runRules(os.Args[2:])
	case "recover":
		err = runRecover(os.Args[2:])
	case "review":
		err = runReview(os.Args[2:])
	case "next-steps":
		err = runNextSteps(os.Args[2:])
	case "version":
		err = runVersion(os.Args[2:])
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
	dbPath := fs.String("db", "", "SQLite database for file/context persistence (optional)")
	fullScan := fs.Bool("full", false, "force full scan (ignore checkpoint, recompute all hashes)")
	resume := fs.Bool("resume", false, "resume from last checkpoint if available")
	workers := fs.Int("workers", 1, "number of concurrent hash workers (1 = serial, recommended 2-8)")
	hashAttempts := fs.Int("hash-attempts", 3, "maximum read attempts per hash")
	hashRetryDelay := fs.Duration("hash-retry-delay", 250*time.Millisecond, "delay between hash attempts")
	hashFailuresOut := fs.String("hash-failures-out", "", "private hash failure manifest (default: <out>.hash-failures.jsonl)")
	progressOut := fs.String("progress-out", "", "owner-only aggregate progress snapshot (optional)")
	progressInterval := fs.Duration("progress-interval", 15*time.Second, "progress snapshot interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return fmt.Errorf("--root is required")
	}
	if *hashFailuresOut == "" {
		*hashFailuresOut = *out + ".hash-failures.jsonl"
	}
	// signal-aware context so Ctrl+C can abort the scan gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Open store early if --db is provided so we can use incremental mode.
	var st *store.SQLiteStore
	if *dbPath != "" {
		var err error
		st, err = store.Open(ctx, *dbPath)
		if err != nil {
			return err
		}
		defer st.Close()
	}

	// Create scan service. The service encapsulates traversal, incremental
	// hash reuse, two-stage progressive fingerprinting, and DB persistence.
	// When --db is omitted, pass a nil interface (not a nil *SQLiteStore)
	// so the service correctly detects JSONL-only mode.
	// We inject the global quickHash/fullHash variables so tests can
	// override them without reaching into the service internals.
	var scanStore app.ScanStore
	if st != nil {
		scanStore = st
	}
	svc := app.NewScanServiceWithHashFunc(scanStore, app.HashFunc(quickHash), app.HashFunc(fullHash))

	// Start progress reporter that polls the service's Progress() method.
	reporter, err := startProgressReporter(*progressOut, *progressInterval, func() progressSnapshot {
		p := svc.Progress()
		return progressSnapshot{
			Command: "scan", Stage: p.Stage, Status: progressStatus(p.Stage),
			Processed: p.Processed,
			Details:   map[string]int{"discovered": int(p.Discovered), "hash_failures": int(p.Failed)},
		}
	})
	if err != nil {
		return err
	}
	if reporter != nil {
		defer reporter.Stop()
	}

	// Run the scan through the service.
	result, err := svc.Scan(ctx, app.ScanInput{
		Root:           *root,
		StorageID:      *storage,
		FullScan:       *fullScan,
		Resume:         *resume,
		Workers:        *workers,
		HashAttempts:   *hashAttempts,
		HashRetryDelay: *hashRetryDelay,
	})
	if err != nil {
		return err
	}

	if result.ResumedFrom != "" {
		fmt.Printf("resuming from checkpoint %d (%d files scanned; path omitted)\n",
			result.CheckpointID, result.ResumedCount)
	}

	// Write hash failure manifest (convert app.HashFailure to hashFailure).
	cliFailures := make([]hashFailure, len(result.HashFailures))
	for i, f := range result.HashFailures {
		cliFailures[i] = hashFailure(f)
	}
	if err := writeHashFailures(*hashFailuresOut, cliFailures); err != nil {
		return fmt.Errorf("write private hash failure manifest: %w", err)
	}
	if len(result.HashFailures) > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d file(s) could not be fingerprinted; paths omitted\n", len(result.HashFailures))
	}

	// Write JSONL index.
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := idx.Write(*out, result.Files); err != nil {
		return err
	}

	// Print summary.
	if st != nil {
		if !result.FullTraversal && result.Unavailable > 0 {
			fmt.Fprintf(os.Stderr, "warning: partial traversal preserved %d unseen item(s) as unavailable; paths omitted\n", result.Unavailable)
		}
		fmt.Printf("indexed %d files into %s (%d missing, %d unavailable marked; read-only source scan)\n",
			len(result.Files), *out, result.Missing, result.Unavailable)
	} else {
		fmt.Printf("indexed %d files into %s (read-only scan)\n", len(result.Files), *out)
	}
	reportScanErrors(os.Stderr, result.ScanErrors)
	return nil
}

func reportScanErrors(w io.Writer, count int) {
	if count > 0 {
		fmt.Fprintf(w, "warning: %d filesystem entries could not be scanned; paths omitted\n", count)
	}
}

// reportHashErrors prints an aggregate warning for hash failures without
// leaking sensitive source paths. Operators can consult the private hash
// failure manifest for per-file details.
func reportHashErrors(w io.Writer, errs []error) {
	if len(errs) == 0 {
		return
	}
	fmt.Fprintf(w, "warning: %d file(s) could not be fingerprinted; paths omitted\n", len(errs))
}

// canMarkMissing returns true only when the scan completed a full traversal
// without errors. Partial traversals must not reconcile missing state because
// files that were skipped (permission denied, I/O error) would be incorrectly
// marked as missing.
func canMarkMissing(stats scanner.Stats) bool {
	return len(stats.Errors) == 0
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
	svc := app.NewDuplicateService()
	groups := svc.DuplicateGroups(context.Background(), files)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(groups)
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
	svc := app.NewPlanService()
	plans := svc.BuildPlans(context.Background(), files)
	f, err := privatefs.Create(*out)
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

func usage() {
	fmt.Fprintln(os.Stderr, "usage: nas-governance <scan|retry-hashes|import-index|duplicates|plan|approve|execute|quarantine|purge|analyze|diagnose-formats|diagnose-governance|diagnose-paths|diagnose-merges|group|relations|merge|learn|rules|recover|review|next-steps|version> [options]")
}

func runVersion(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("version does not accept arguments")
	}
	fmt.Printf("nas-governance %s\ncommit: %s\nbuilt: %s\n", version, commit, buildTime)
	return nil
}

// ---- approve ----

// stringList accumulates repeated --flag values into a slice.
type stringList []string

func (s *stringList) String() string     { return fmt.Sprintf("%v", *s) }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

func runApprove(args []string) error {
	fs := flag.NewFlagSet("approve", flag.ContinueOnError)
	planPath := fs.String("plan", "./var/plan.json", "draft plan input")
	out := fs.String("out", "./var/approved.json", "approved plan output")
	var planIDs stringList
	fs.Var(&planIDs, "plan-id", "plan ID to approve (repeatable; omit with --all)")
	all := fs.Bool("all", false, "approve all plans in the file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*all && len(planIDs) == 0 {
		return fmt.Errorf("--plan-id or --all is required")
	}
	plans, err := readPlans(*planPath)
	if err != nil {
		return err
	}
	svc := app.NewPlanService()
	result, err := svc.ApprovePlans(context.Background(), app.ApprovePlansInput{
		Plans: plans, IDs: planIDs, All: *all,
	})
	if err != nil {
		return err
	}
	f, err := privatefs.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result.Approved); err != nil {
		return err
	}
	fmt.Printf("approved %d plan(s) into %s\n", len(result.Approved), *out)
	return nil
}

// ---- execute ----

func runExecute(args []string) error {
	fs := flag.NewFlagSet("execute", flag.ContinueOnError)
	planPath := fs.String("plan", "./var/approved.json", "approved plan input")
	out := fs.String("out", "./var/audit.json", "audit report output")
	quarantineRoot := fs.String("quarantine", "", "quarantine root directory (absolute)")
	var sourceRoots stringList
	fs.Var(&sourceRoots, "source-root", "approved task root (repeatable, absolute)")
	dbPath := fs.String("db", "", "SQLite database for mandatory execution journal")
	dryRun := fs.Bool("dry-run", false, "validate plans without executing filesystem actions")
	retention := fs.Duration("retention", 30*24*time.Hour, "managed quarantine retention before PURGE eligibility (minimum 24h)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *quarantineRoot == "" {
		return fmt.Errorf("--quarantine is required")
	}
	if len(sourceRoots) == 0 {
		return fmt.Errorf("--source-root is required (at least one)")
	}
	if !*dryRun && *dbPath == "" {
		return fmt.Errorf("--db is required for non-dry-run execution")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	plans, err := readPlans(*planPath)
	if err != nil {
		return err
	}
	// SQLite is mandatory for real execution and optional for read-only dry-run.
	var st *store.SQLiteStore
	if *dbPath != "" {
		st, err = store.Open(ctx, *dbPath)
		if err != nil {
			return err
		}
		defer st.Close()
	}
	// Delegate execution to the application service. The service handles
	// task creation, plan persistence, journal management, audit logging,
	// and quarantine registration.
	svc := app.NewExecutionService(st)
	summary, err := svc.Execute(ctx, app.ExecutionInput{
		Plans:          plans,
		QuarantineRoot: *quarantineRoot,
		SourceRoots:    sourceRoots,
		DryRun:         *dryRun,
		Retention:      *retention,
	})
	if err != nil {
		return err
	}
	f, err := privatefs.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(summary.Results); err != nil {
		return err
	}
	fmt.Printf("executed %d, skipped %d, failed %d — audit: %s\n", summary.Executed, summary.Skipped, summary.Failed, *out)
	return summary.LifecycleErr
}

// ---- analyze ----

// formatReportEntry is one row in the analyze output.
type formatReportEntry struct {
	Path      string            `json:"path"`
	StorageID string            `json:"storage_id"`
	Format    domain.FormatInfo `json:"format"`
	Error     string            `json:"error,omitempty"`
}

// runAnalyze reads a scan index, runs header-only format analysis (K-006) on
// each file, and writes a JSON report. When --db is provided, results are also
// persisted to SQLite in batches; the file rows must already exist
// (created by a prior scan). Analyze is read-only with respect to user data:
// it only reads file headers and writes its own report/database.
func runAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	index := fs.String("index", "./var/index.jsonl", "index file from scan")
	out := fs.String("out", "./var/formats.json", "format analysis report output")
	dbPath := fs.String("db", "", "SQLite database to persist results (optional)")
	storageID := fs.String("storage-id", "", "filter files by storage ID (optional)")
	limit := fs.Int("limit", 0, "max files to analyze (0 = all)")
	workers := fs.Int("workers", 1, "number of concurrent analysis workers (1 = serial)")
	resume := fs.Bool("resume", false, "reuse completed format rows from --db and skip their NAS reads")
	refreshUnknown := fs.Bool("refresh-unknown", false, "with --resume, re-analyze previously unknown records using current rules")
	refreshMetadata := fs.Bool("refresh-metadata", false, "with --resume, re-analyze supported media records with missing metadata")
	batchSize := fs.Int("batch-size", 500, "format records per SQLite transaction")
	progressEvery := fs.Int("progress-every", 10000, "aggregate progress interval (0 = disabled)")
	progressOut := fs.String("progress-out", "", "owner-only aggregate progress snapshot (optional)")
	progressInterval := fs.Duration("progress-interval", 15*time.Second, "progress snapshot interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *resume && *dbPath == "" {
		return fmt.Errorf("--resume requires --db")
	}
	if *refreshUnknown && !*resume {
		return fmt.Errorf("--refresh-unknown requires --resume")
	}
	if *refreshMetadata && !*resume {
		return fmt.Errorf("--refresh-metadata requires --resume")
	}
	if *workers < 1 || *workers > 64 {
		return fmt.Errorf("--workers must be between 1 and 64")
	}
	if *batchSize < 1 || *batchSize > 10000 {
		return fmt.Errorf("--batch-size must be between 1 and 10000")
	}
	if *progressEvery < 0 {
		return fmt.Errorf("--progress-every cannot be negative")
	}
	files, err := idx.Read(*index)
	if err != nil {
		return err
	}
	if *storageID != "" {
		filtered := make([]domain.FileInstance, 0, len(files))
		for _, f := range files {
			if f.StorageID == *storageID {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}
	if *limit > 0 && len(files) > *limit {
		files = files[:*limit]
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	var st *store.SQLiteStore
	if *dbPath != "" {
		st, err = store.Open(ctx, *dbPath)
		if err != nil {
			return err
		}
		defer st.Close()
	}

	// Resume is based on durable format rows, not a fragile path cursor. This
	// safely handles worker completion order and makes repeated runs idempotent.
	existing := map[string]domain.FormatInfo{}
	if *resume {
		records, listErr := st.ListFormats(ctx, *storageID)
		if listErr != nil {
			return listErr
		}
		for _, record := range records {
			existing[formatRecordKey(record.StorageID, record.Path)] = record.Info
		}
	}

	// Concurrent format analysis via worker pool. Each worker reads file
	// headers (read-only) and writes results to the shared entries slice.
	// A separate local persistence stage batches SQLite writes; it never writes
	// to the NAS and ordinary progress output contains aggregate counts only.
	entries := make([]formatReportEntry, len(files))
	var entriesMu sync.Mutex
	var analyzed, unrecognized, failed int64
	type pendingFile struct {
		index int
		file  domain.FileInstance
	}
	pending := make([]pendingFile, 0, len(files))
	var reused int64
	for i, file := range files {
		if info, ok := existing[formatRecordKey(file.StorageID, file.Path)]; ok &&
			!(*refreshUnknown && isUnknownFormat(info)) &&
			!(*refreshMetadata && needsMetadataRefresh(info)) {
			entries[i] = formatReportEntry{Path: file.Path, StorageID: file.StorageID, Format: info}
			reused++
			if info.Format == "" || info.Format == "unknown" || info.Category == domain.CategoryUnknown {
				unrecognized++
			} else {
				analyzed++
			}
			continue
		}
		pending = append(pending, pendingFile{index: i, file: file})
	}
	var analyzeStage atomic.Value
	analyzeStage.Store("format_analysis")
	var processedNew int64
	reporter, err := startProgressReporter(*progressOut, *progressInterval, func() progressSnapshot {
		stage := analyzeStage.Load().(string)
		return progressSnapshot{
			Command: "analyze", Stage: stage, Status: progressStatus(stage),
			Processed: reused + atomic.LoadInt64(&processedNew), Total: int64(len(files)),
			Failed: atomic.LoadInt64(&failed), Reused: reused,
		}
	})
	if err != nil {
		return err
	}
	if reporter != nil {
		defer reporter.Stop()
	}
	if *resume {
		fmt.Printf("resume: reused %d completed format record(s); source paths omitted\n", reused)
	}

	var persistCh chan store.FormatRecord
	var persistWG sync.WaitGroup
	var persistMu sync.Mutex
	var persistErr error
	var persisted, lookupFailed int64
	if st != nil {
		persistCh = make(chan store.FormatRecord, *batchSize*2)
		persistWG.Add(1)
		go func() {
			defer persistWG.Done()
			batch := make([]store.FormatRecord, 0, *batchSize)
			flush := func() {
				if len(batch) == 0 {
					return
				}
				persistMu.Lock()
				failedAlready := persistErr != nil
				persistMu.Unlock()
				if !failedAlready {
					saved, missing, err := st.SaveFormatsByPath(context.Background(), batch)
					persistMu.Lock()
					persisted += int64(saved)
					lookupFailed += int64(missing)
					if err != nil && persistErr == nil {
						persistErr = err
					}
					persistMu.Unlock()
				}
				batch = batch[:0]
			}
			for record := range persistCh {
				batch = append(batch, record)
				if len(batch) >= *batchSize {
					flush()
				}
			}
			flush()
		}()
	}

	ar := runner.New(*workers)
	var submitErr error
	for _, item := range pending {
		idx := item.index
		file := item.file
		if err := ar.Submit(ctx, func() error {
			info, analyzeErr := format.Analyze(file.Path)
			entry := formatReportEntry{Path: file.Path, StorageID: file.StorageID, Format: info}
			if analyzeErr != nil {
				entry.Error = analyzeErr.Error()
				atomic.AddInt64(&failed, 1)
			} else if info.Format == "" || info.Format == "unknown" || info.Category == domain.CategoryUnknown {
				atomic.AddInt64(&unrecognized, 1)
			} else {
				atomic.AddInt64(&analyzed, 1)
			}
			if persistCh != nil && analyzeErr == nil && info.Format != "" {
				persistCh <- store.FormatRecord{StorageID: file.StorageID, Path: file.Path, Info: info}
			}
			entriesMu.Lock()
			entries[idx] = entry
			entriesMu.Unlock()
			done := reused + atomic.AddInt64(&processedNew, 1)
			if *progressEvery > 0 && done%int64(*progressEvery) == 0 {
				fmt.Fprintf(os.Stderr, "progress: analyzed %d/%d file(s); source paths omitted\n", done, len(files))
			}
			return nil
		}); err != nil {
			submitErr = err
			break
		}
	}
	ar.Wait()
	analyzeStage.Store("persisting")
	if persistCh != nil {
		close(persistCh)
		persistWG.Wait()
	}
	persistMu.Lock()
	batchErr := persistErr
	persistMu.Unlock()
	completed := reused + atomic.LoadInt64(&processedNew)
	if completed != int64(len(files)) {
		fmt.Fprintf(os.Stderr, "analysis interrupted after %d/%d file(s); rerun with --resume; source paths omitted\n", completed, len(files))
		if submitErr != nil {
			return submitErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return fmt.Errorf("analysis incomplete")
	}

	f, err := privatefs.Create(*out)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(entries); err != nil {
		return err
	}
	fmt.Printf("analyzed %d files (%d recognized, %d unrecognized, %d failed) — report: %s\n",
		len(entries), analyzed, unrecognized, failed, *out)
	if st != nil {
		fmt.Printf("persisted %d new format records to %s (%d reused)\n", persisted, *dbPath, reused)
	}
	analyzeStage.Store("completed")
	reportAnalyzePersistenceErrors(os.Stderr, int(lookupFailed), 0)
	if batchErr != nil {
		return batchErr
	}
	return nil
}

func formatRecordKey(storageID, path string) string { return storageID + "\x00" + path }

func isUnknownFormat(info domain.FormatInfo) bool {
	return info.Format == "" || info.Format == "unknown" || info.Category == domain.CategoryUnknown
}

// needsMetadataRefresh is deliberately capability-scoped. Formats whose
// parser cannot currently fill a missing field are not retried forever.
func needsMetadataRefresh(info domain.FormatInfo) bool {
	switch info.Format {
	case "wav", "aiff", "flac", "m4a":
		return info.Duration <= 0
	case "mp4", "mov", "m4v", "avi":
		return info.Duration <= 0 || info.Width <= 0 || info.Height <= 0
	case "mpeg":
		return info.Width <= 0 || info.Height <= 0
	default:
		return false
	}
}

func reportAnalyzePersistenceErrors(w io.Writer, lookupFailed, saveFailed int) {
	if lookupFailed > 0 {
		fmt.Fprintf(w, "warning: %d analyzed format record(s) had no database row; source paths omitted\n", lookupFailed)
	}
	if saveFailed > 0 {
		fmt.Fprintf(w, "warning: %d analyzed format record(s) could not be persisted; source paths omitted\n", saveFailed)
	}
}

// ---- group / relations / merge ----

// runGroup reads a scan index and clusters files into asset groups by
// business anchor or path proximity. Output is JSON to stdout (K-001/K-002).
func runGroup(args []string) error {
	fs := flag.NewFlagSet("group", flag.ContinueOnError)
	path := fs.String("index", "./var/index.jsonl", "index file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files, err := idx.Read(*path)
	if err != nil {
		return err
	}
	groups := assets.Group(files)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(groups); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "identified %d asset groups\n", len(groups))
	return nil
}

// runRelations reads a scan index and detects version + derivative
// relationships between files. All relations are read-only observations;
// none trigger deletion (K-002).
func runRelations(args []string) error {
	fs := flag.NewFlagSet("relations", flag.ContinueOnError)
	path := fs.String("index", "./var/index.jsonl", "index file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files, err := idx.Read(*path)
	if err != nil {
		return err
	}
	rels := relations.Relations(files)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rels); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "identified %d file relations\n", len(rels))
	return nil
}

// runMerge reads a scan index and proposes directory consolidation
// suggestions. Suggestions are read-only; no filesystem action is taken
// (K-008). Each suggestion requires human review before a plan is created.
func runMerge(args []string) error {
	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	path := fs.String("index", "./var/index.jsonl", "index file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files, err := idx.Read(*path)
	if err != nil {
		return err
	}
	suggestions := merge.Suggest(files)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(suggestions); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "identified %d merge suggestions\n", len(suggestions))
	return nil
}

// ---- learn ----

// runLearn runs local learning. Three sources are supported:
//   - --source=stats (L2): reads indexed file metadata from the governance
//     SQLite database. Read-only w.r.t. user data (K-009).
//   - --source=corpus (L3): reads user-provided trusted documents from
//     --corpus-dir (default var/corpus/). Only this directory is read,
//     never the user's NAS data.
//   - --source=feedback (L4): reads historical plan decisions from the
//     governance DB and proposes retention-score weight adjustments
//     (±3 cap) plus rule confidence downgrades. Only structured fields
//     are read, never paths (K-009).
//
// By default it prints a summary (no writes). With --apply it also persists
// directory-signal rule drafts (status=draft) to the rules table for human
// review via the `rules` subcommand. Drafts never override protection rules
// (priority <= 60, K-008).
func runLearn(args []string) error {
	fs := flag.NewFlagSet("learn", flag.ContinueOnError)
	dbPath := fs.String("db", "./var/governance.db", "governance database (for stats source or to persist drafts)")
	source := fs.String("source", "stats", "learning source: stats | corpus | feedback")
	corpusDir := fs.String("corpus-dir", "./var/corpus", "trusted corpus directory (source=corpus only)")
	out := fs.String("out", "", "write full stats JSON to this path (optional)")
	apply := fs.Bool("apply", false, "persist generated rule drafts to the database")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch *source {
	case "stats":
		return runLearnStats(args, *dbPath, *out, *apply)
	case "corpus":
		return runLearnCorpus(args, *dbPath, *corpusDir, *out, *apply)
	case "feedback":
		return runLearnFeedback(args, *dbPath, *out, *apply)
	default:
		return fmt.Errorf("learn: --source %q not supported (use 'stats', 'corpus', or 'feedback')", *source)
	}
}

func runLearnFeedback(args []string, dbPath, outPath string, apply bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	stats, err := learning.LearnFromFeedback(ctx, st, learning.FeedbackOptions{})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "feedback: analyzed %d plans (%d rejected)\n", stats.PlansAnalyzed, stats.PlansRejected)
	fmt.Fprintf(os.Stderr, "weight adjustments: %d\n", len(stats.WeightAdjustments))
	for _, adj := range stats.WeightAdjustments {
		fmt.Fprintf(os.Stderr, "  %-12s delta=%-3d  %s\n", adj.Component, adj.Delta, adj.Reason)
	}
	fmt.Fprintf(os.Stderr, "confidence downgrades: %d\n", len(stats.ConfidenceDowngrades))
	for _, dg := range stats.ConfidenceDowngrades {
		fmt.Fprintf(os.Stderr, "  %-28s rejection=%.0f%%  delta=%.2f  samples=%d\n", dg.RuleID, dg.ObservedRejection*100, dg.SuggestedDelta, dg.Samples)
	}

	if outPath != "" {
		if err := writeJSONFile(outPath, stats); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "stats written to %s\n", outPath)
	}
	if !apply {
		fmt.Fprintln(os.Stderr, "dry run: no drafts persisted (pass --apply to write drafts)")
		return nil
	}
	batchID := learning.NewBatchID("feedback", time.Now().UTC())
	rules, err := learning.GenerateFeedbackDrafts(ctx, st, stats, batchID)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "persisted %d draft rules (batch %s) — review with `rules list --status draft`\n", len(rules), batchID)
	return nil
}

func runLearnStats(args []string, dbPath, outPath string, apply bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	stats, err := learning.Learn(ctx, st)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "learned from %d files (%d sensitive skipped, K-009)\n", stats.TotalFiles, stats.SensitiveSkipped)
	fmt.Fprintf(os.Stderr, "dir stats above threshold: %d names\n", len(stats.DirStats))
	fmt.Fprintf(os.Stderr, "project code patterns: %d\n", len(stats.ProjectCodes))
	for _, ds := range stats.DirStats {
		fmt.Fprintf(os.Stderr, "  %-24s dirs=%-3d files=%-4d role=%s\n", ds.Name, ds.DirCount, ds.FileCount, ds.SuggestedRole)
	}

	if outPath != "" {
		if err := writeJSONFile(outPath, stats); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "stats written to %s\n", outPath)
	}
	if !apply {
		fmt.Fprintln(os.Stderr, "dry run: no drafts persisted (pass --apply to write drafts)")
		return nil
	}
	batchID := learning.NewBatchID("stats", time.Now().UTC())
	rules, err := learning.GenerateDrafts(ctx, st, stats, batchID)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "persisted %d draft rules (batch %s) — review with `rules list --status draft`\n", len(rules), batchID)
	return nil
}

func runLearnCorpus(args []string, dbPath, corpusDir, outPath string, apply bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	stats, err := learning.LearnFromCorpus(ctx, learning.CorpusOptions{Dir: corpusDir})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "corpus: read %d files (%d skipped)\n", stats.FilesRead, stats.FilesSkipped)
	fmt.Fprintf(os.Stderr, "role candidates: %d\n", len(stats.Terms))
	for _, t := range stats.Terms {
		fmt.Fprintf(os.Stderr, "  %-20s count=%-3d role=%s\n", t.Term, t.Count, t.SuggestedRole)
	}
	fmt.Fprintf(os.Stderr, "sensitive candidates: %d\n", len(stats.SensitiveCandidates))
	for _, t := range stats.SensitiveCandidates {
		fmt.Fprintf(os.Stderr, "  %-20s count=%-3d (sensitive)\n", t.Term, t.Count)
	}

	if outPath != "" {
		if err := writeJSONFile(outPath, stats); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "stats written to %s\n", outPath)
	}
	if !apply {
		fmt.Fprintln(os.Stderr, "dry run: no drafts persisted (pass --apply to write drafts)")
		return nil
	}
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	batchID := learning.NewBatchID("corpus", time.Now().UTC())
	rules, err := learning.GenerateCorpusDrafts(ctx, st, stats, batchID)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "persisted %d draft rules (batch %s) — review with `rules list --status draft`\n", len(rules), batchID)
	return nil
}

// writeJSONFile writes v as indented JSON to path, creating parent dirs.
func writeJSONFile(path string, v any) error {
	f, err := privatefs.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// ---- rules ----

// runRules manages the rule lifecycle: list drafts/approved/disabled,
// approve a draft (entering probation), reject a draft, or disable a
// whole batch. All operations are on the project's SQLite database only.
func runRules(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("rules requires a subcommand: list|approve|reject|disable")
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "list":
		return runRulesList(rest)
	case "approve":
		return runRulesApprove(rest)
	case "reject":
		return runRulesReject(rest)
	case "disable":
		return runRulesDisable(rest)
	default:
		return fmt.Errorf("unknown rules subcommand: %s", sub)
	}
}

func runRulesList(args []string) error {
	fs := flag.NewFlagSet("rules list", flag.ContinueOnError)
	dbPath := fs.String("db", "./var/governance.db", "database path")
	status := fs.String("status", "", "filter by status (draft|probation|approved|disabled|rejected)")
	source := fs.String("source", "", "filter by source (builtin|learned|user)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	rules, err := st.ListRules(ctx, domain.RuleSource(*source), domain.RuleStatus(*status))
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rules); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%d rules\n", len(rules))
	return nil
}

func runRulesApprove(args []string) error {
	fs := flag.NewFlagSet("rules approve", flag.ContinueOnError)
	dbPath := fs.String("db", "./var/governance.db", "database path")
	batch := fs.String("batch", "", "approve all drafts in a batch")
	id := fs.String("id", "", "approve a single rule by ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *batch == "" && *id == "" {
		return fmt.Errorf("rules approve requires --batch or --id")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	now := time.Now().UTC()
	if *id != "" {
		if err := st.UpdateRuleStatus(ctx, *id, domain.RuleProbation, &now); err != nil {
			return err
		}
		fmt.Printf("rule %s → probation\n", *id)
		return nil
	}
	// Approve all drafts in the batch.
	rules, err := st.ListRules(ctx, domain.RuleSourceLearned, domain.RuleDraft)
	if err != nil {
		return err
	}
	count := 0
	for _, r := range rules {
		if r.BatchID != *batch {
			continue
		}
		if err := st.UpdateRuleStatus(ctx, r.ID, domain.RuleProbation, &now); err != nil {
			return err
		}
		count++
	}
	fmt.Printf("approved %d rules in batch %s → probation\n", count, *batch)
	return nil
}

func runRulesReject(args []string) error {
	fs := flag.NewFlagSet("rules reject", flag.ContinueOnError)
	dbPath := fs.String("db", "./var/governance.db", "database path")
	id := fs.String("id", "", "reject a single rule by ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return fmt.Errorf("rules reject requires --id")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.UpdateRuleStatus(ctx, *id, domain.RuleRejected, nil); err != nil {
		return err
	}
	fmt.Printf("rule %s → rejected\n", *id)
	return nil
}

func runRulesDisable(args []string) error {
	fs := flag.NewFlagSet("rules disable", flag.ContinueOnError)
	dbPath := fs.String("db", "./var/governance.db", "database path")
	batch := fs.String("batch", "", "disable all rules in a batch")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *batch == "" {
		return fmt.Errorf("rules disable requires --batch")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.DisableBatch(ctx, *batch); err != nil {
		return err
	}
	fmt.Printf("disabled all rules in batch %s\n", *batch)
	return nil
}

// ---- helpers ----

func readPlans(path string) ([]domain.OperationPlan, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var plans []domain.OperationPlan
	dec := json.NewDecoder(f)
	if err := dec.Decode(&plans); err != nil {
		return nil, err
	}
	return plans, nil
}

// persistAudit writes each audit step as an operation_log row. Failures are
// reported to stderr but do not stop execution — the audit JSON file is the
// primary record, and the database is a secondary index for querying.
func persistAudit(ctx context.Context, st *store.SQLiteStore, result executor.Result) {
	if len(result.Steps) == 0 && result.Err != nil {
		_ = st.AppendLog(ctx, result.PlanID, "pipeline_error", map[string]any{
			"status": "failed", "final_state": string(result.FinalState), "error_type": result.ErrorType,
		})
		return
	}
	for _, step := range result.Steps {
		detail := make(map[string]any)
		for k, v := range step.Detail {
			detail[k] = v
		}
		detail["status"] = string(step.Status)
		detail["final_state"] = string(result.FinalState)
		if result.Err != nil {
			detail["error_type"] = result.ErrorType
		}
		if err := st.AppendLog(ctx, result.PlanID, step.Name, detail); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write audit log for plan %s: %v\n", result.PlanID, err)
		}
	}
}

// ---- recover ----

func runRecover(args []string) error {
	fs := flag.NewFlagSet("recover", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database (required)")
	out := fs.String("out", "", "write recovery results to file (default: stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return fmt.Errorf("--db is required")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	svc := app.NewRecoveryService(st)
	results, err := svc.Recover(ctx)
	if err != nil {
		return err
	}

	rolledBack, reset, errors := 0, 0, 0
	for _, r := range results {
		switch r.Action {
		case executor.RecoveryRolledBack:
			rolledBack++
		case executor.RecoveryResetToApproved:
			reset++
		}
		if len(r.Errors) > 0 {
			errors++
		}
	}

	enc := json.NewEncoder(os.Stdout)
	if *out != "" {
		f, err := privatefs.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		enc = json.NewEncoder(f)
	}
	enc.SetIndent("", "  ")
	if err := enc.Encode(results); err != nil {
		return err
	}
	fmt.Printf("recovery: %d rolled back, %d reset to approved, %d with errors\n", rolledBack, reset, errors)
	return nil
}
