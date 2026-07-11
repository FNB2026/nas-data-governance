package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"nas-data-governance/internal/domain"
	"nas-data-governance/internal/executor"
	"nas-data-governance/internal/fingerprint"
	"nas-data-governance/internal/format"
	idx "nas-data-governance/internal/index"
	"nas-data-governance/internal/planner"
	"nas-data-governance/internal/report"
	"nas-data-governance/internal/scanner"
	"nas-data-governance/internal/store"
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
	case "approve":
		err = runApprove(os.Args[2:])
	case "execute":
		err = runExecute(os.Args[2:])
	case "analyze":
		err = runAnalyze(os.Args[2:])
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

func usage() {
	fmt.Fprintln(os.Stderr, "usage: nas-governance <scan|duplicates|plan|approve|execute|analyze> [options]")
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
	want := make(map[string]bool)
	for _, id := range planIDs {
		want[id] = true
	}
	approved := make([]domain.OperationPlan, 0, len(plans))
	for _, p := range plans {
		if *all || want[p.ID] {
			if p.State != domain.PlanDraft {
				return fmt.Errorf("plan %s is in state %s, expected DRAFT", p.ID, p.State)
			}
			if err := executor.Transition(&p, domain.PlanApproved); err != nil {
				return fmt.Errorf("plan %s: %w", p.ID, err)
			}
			approved = append(approved, p)
		}
	}
	if len(approved) == 0 {
		return fmt.Errorf("no plans matched the selection")
	}
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
	if err := enc.Encode(approved); err != nil {
		return err
	}
	fmt.Printf("approved %d plan(s) into %s\n", len(approved), *out)
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
	dbPath := fs.String("db", "", "SQLite database for audit logs (optional)")
	dryRun := fs.Bool("dry-run", false, "validate plans without executing filesystem actions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *quarantineRoot == "" {
		return fmt.Errorf("--quarantine is required")
	}
	if len(sourceRoots) == 0 {
		return fmt.Errorf("--source-root is required (at least one)")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	plans, err := readPlans(*planPath)
	if err != nil {
		return err
	}
	// Optional SQLite store for audit log persistence.
	var st *store.SQLiteStore
	if *dbPath != "" {
		st, err = store.Open(ctx, *dbPath)
		if err != nil {
			return err
		}
		defer st.Close()
		// Create a task and save plans so operation_logs FK is satisfied.
		taskID := fmt.Sprintf("task-%d", time.Now().Unix())
		if err := st.CreateTask(ctx, domain.OperationTask{
			ID: taskID, RootPath: sourceRoots[0], State: "executing", CreatedAt: time.Now(),
		}); err != nil {
			return fmt.Errorf("create task: %w", err)
		}
		if err := st.SavePlans(ctx, taskID, plans); err != nil {
			return fmt.Errorf("save plans: %w", err)
		}
	}
	qCfg := executor.QuarantineConfig{
		Root:        *quarantineRoot,
		Structure:   executor.QuarantineFlat,
		SourceRoots: sourceRoots,
	}
	exec, err := executor.New(qCfg)
	if err != nil {
		return err
	}
	results := make([]executor.Result, 0, len(plans))
	executed, skipped, failed := 0, 0, 0
	for i := range plans {
		p := &plans[i]
		if p.State != domain.PlanApproved {
			skipped++
			results = append(results, executor.Result{
				PlanID:     p.ID,
				FinalState: p.State,
				Steps:      []executor.AuditStep{{Name: "skip", Status: executor.StepSkipped, Detail: map[string]any{"reason": "not approved"}}},
			})
			continue
		}
		if *dryRun {
			skipped++
			results = append(results, executor.Result{
				PlanID:     p.ID,
				FinalState: p.State,
				Steps:      []executor.AuditStep{{Name: "dry_run", Status: executor.StepSkipped, Detail: map[string]any{"reason": "dry-run mode"}}},
			})
			continue
		}
		result := exec.Execute(ctx, p)
		results = append(results, result)
		if result.Err != nil {
			failed++
		} else {
			executed++
		}
		// Persist audit steps to SQLite when a database is configured.
		if st != nil {
			persistAudit(ctx, st, result)
		}
	}
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
	if err := enc.Encode(results); err != nil {
		return err
	}
	fmt.Printf("executed %d, skipped %d, failed %d — audit: %s\n", executed, skipped, failed, *out)
	return nil
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
// persisted to SQLite via store.SaveFormat; the file rows must already exist
// (created by a prior scan). Analyze is read-only with respect to user data:
// it only reads file headers and writes its own report/database.
func runAnalyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ContinueOnError)
	index := fs.String("index", "./var/index.jsonl", "index file from scan")
	out := fs.String("out", "./var/formats.json", "format analysis report output")
	dbPath := fs.String("db", "", "SQLite database to persist results (optional)")
	storageID := fs.String("storage-id", "", "filter files by storage ID (optional)")
	limit := fs.Int("limit", 0, "max files to analyze (0 = all)")
	if err := fs.Parse(args); err != nil {
		return err
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
	entries := make([]formatReportEntry, 0, len(files))
	analyzed, unrecognized, failed, persisted := 0, 0, 0, 0
	for _, f := range files {
		info, analyzeErr := format.Analyze(f.Path)
		entry := formatReportEntry{Path: f.Path, StorageID: f.StorageID, Format: info}
		if analyzeErr != nil {
			failed++
			entry.Error = analyzeErr.Error()
		} else if info.Format == "" {
			unrecognized++
		} else {
			analyzed++
		}
		// Persist to SQLite when configured. The file row must already exist
		// (created by a prior scan with --db); missing rows are reported as
		// warnings but do not stop analysis.
		if st != nil && analyzeErr == nil && info.Format != "" {
			fileID, lookupErr := st.FileID(ctx, f.StorageID, f.Path)
			if lookupErr != nil {
				fmt.Fprintf(os.Stderr, "warning: no file row for %s: %v\n", f.Path, lookupErr)
			} else if saveErr := st.SaveFormat(ctx, fileID, info); saveErr != nil {
				fmt.Fprintf(os.Stderr, "warning: save format for %s: %v\n", f.Path, saveErr)
			} else {
				persisted++
			}
		}
		entries = append(entries, entry)
	}
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
	if err := enc.Encode(entries); err != nil {
		return err
	}
	fmt.Printf("analyzed %d files (%d recognized, %d unrecognized, %d failed) — report: %s\n",
		len(entries), analyzed, unrecognized, failed, *out)
	if st != nil {
		fmt.Printf("persisted %d format records to %s\n", persisted, *dbPath)
	}
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
	for _, step := range result.Steps {
		detail := make(map[string]any)
		for k, v := range step.Detail {
			detail[k] = v
		}
		detail["status"] = string(step.Status)
		detail["final_state"] = string(result.FinalState)
		if result.Err != nil {
			detail["error"] = result.Err.Error()
		}
		if err := st.AppendLog(ctx, result.PlanID, step.Name, detail); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to write audit log for plan %s: %v\n", result.PlanID, err)
		}
	}
}
