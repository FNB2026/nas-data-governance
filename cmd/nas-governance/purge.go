package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// ============================================================================
// runQuarantine — delegates to app.QuarantineService
//
// The CLI layer handles: flag parsing, store opening, JSON output, and stdout.
// The service layer handles: business logic, executor creation, DB operations.
// Per AGENTS.md rule 2, plan creation, approval, and execution are separate
// steps that must not be merged.
// ============================================================================

func runQuarantine(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("quarantine requires subcommand: list|restore-plan|restore-approve|restore-execute|recover")
	}
	switch args[0] {
	case "list":
		return runQuarantineList(args[1:])
	case "restore-plan":
		return runRestorePlan(args[1:])
	case "restore-approve":
		return runRestoreApprove(args[1:])
	case "restore-execute":
		return runRestoreExecute(args[1:])
	case "recover":
		return runRestoreRecover(args[1:])
	default:
		return fmt.Errorf("unknown quarantine subcommand")
	}
}

func runQuarantineList(args []string) error {
	fs := flag.NewFlagSet("quarantine list", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database (required)")
	statusText := fs.String("status", "", "optional lifecycle status")
	out := fs.String("out", "", "private JSON report (paths are never printed to stdout)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" {
		return fmt.Errorf("--db is required")
	}
	status := domain.QuarantineStatus(*statusText)
	if status != "" && !validQuarantineStatus(status) {
		return fmt.Errorf("invalid --status")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	svc := app.NewQuarantineService(st)
	items, err := svc.ListItems(ctx, status)
	if err != nil {
		return err
	}
	if *out != "" {
		if err := writePrivateJSON(*out, items); err != nil {
			return err
		}
	}
	fmt.Printf("managed quarantine items: %d (paths omitted)\n", len(items))
	return nil
}

func runRestorePlan(args []string) error {
	fs := flag.NewFlagSet("quarantine restore-plan", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database (required)")
	itemID := fs.String("item-id", "", "exact managed quarantine item ID")
	out := fs.String("out", "./var/restore-plan.json", "private DRAFT restore plan")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *itemID == "" {
		return fmt.Errorf("--db and --item-id are required")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	svc := app.NewQuarantineService(st)
	plan, err := svc.CreateRestorePlan(ctx, *itemID)
	if err != nil {
		return err
	}
	if err := writePrivateJSON(*out, plan); err != nil {
		return err
	}
	fmt.Printf("created 1 DRAFT restore plan in %s (no filesystem writes)\n", *out)
	return nil
}

func runRestoreApprove(args []string) error {
	fs := flag.NewFlagSet("quarantine restore-approve", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database (required)")
	planID := fs.String("plan-id", "", "exact restore plan ID")
	digest := fs.String("digest", "", "approval digest from private DRAFT plan")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *planID == "" || *digest == "" {
		return fmt.Errorf("--db, --plan-id and --digest are required")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	svc := app.NewQuarantineService(st)
	if err := svc.ApproveRestorePlan(ctx, *planID, *digest); err != nil {
		return err
	}
	fmt.Println("approved 1 restore plan (paths omitted)")
	return nil
}

func runRestoreExecute(args []string) error {
	fs := flag.NewFlagSet("quarantine restore-execute", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database (required)")
	planID := fs.String("plan-id", "", "exact approved restore plan ID")
	digest := fs.String("digest", "", "approval digest (required again at execution)")
	quarantineRoot := fs.String("quarantine", "", "managed quarantine root (absolute)")
	var sourceRoots stringList
	fs.Var(&sourceRoots, "source-root", "approved restore root (repeatable, absolute)")
	out := fs.String("out", "./var/restore-audit.json", "private restore audit")
	dryRun := fs.Bool("dry-run", false, "read-only stale and boundary validation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *planID == "" || *digest == "" || *quarantineRoot == "" || len(sourceRoots) == 0 {
		return fmt.Errorf("--db, --plan-id, --digest, --quarantine and --source-root are required")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	svc := app.NewQuarantineService(st)
	result, err := svc.ExecuteRestore(ctx, app.RestoreExecuteInput{
		PlanID:         *planID,
		Digest:         *digest,
		QuarantineRoot: *quarantineRoot,
		SourceRoots:    sourceRoots,
		DryRun:         *dryRun,
	})
	// Always write the audit file, even on failure, so the operator has
	// a record of what the executor attempted.
	if result != nil {
		if werr := writePrivateJSON(*out, result.Result); werr != nil {
			return werr
		}
	}
	if err != nil {
		return err
	}
	if *dryRun {
		fmt.Printf("restore dry-run passed for 1 plan — audit: %s\n", *out)
	} else {
		fmt.Printf("restored 1 managed quarantine item — audit: %s\n", *out)
	}
	return nil
}

func runRestoreRecover(args []string) error {
	fs := flag.NewFlagSet("quarantine recover", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database (required)")
	quarantineRoot := fs.String("quarantine", "", "managed quarantine root (absolute)")
	var sourceRoots stringList
	fs.Var(&sourceRoots, "source-root", "approved restore root (repeatable, absolute)")
	out := fs.String("out", "./var/restore-recovery.json", "private recovery report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *quarantineRoot == "" || len(sourceRoots) == 0 {
		return fmt.Errorf("--db, --quarantine and --source-root are required")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	svc := app.NewQuarantineService(st)
	results, err := svc.RecoverRestores(ctx, app.RecoverRestoresInput{
		QuarantineRoot: *quarantineRoot,
		SourceRoots:    sourceRoots,
	})
	if err != nil {
		return err
	}
	if err := writePrivateJSON(*out, results); err != nil {
		return err
	}
	failed := 0
	for _, result := range results {
		if result.Err != nil {
			failed++
		}
	}
	fmt.Printf("restore recovery reconciled %d item(s), %d failed — paths omitted\n", len(results), failed)
	if failed > 0 {
		return fmt.Errorf("restore recovery requires manual review")
	}
	return nil
}

func validQuarantineStatus(status domain.QuarantineStatus) bool {
	switch status {
	case domain.QuarantineActive, domain.QuarantineHold,
		domain.QuarantinePurgeEligible, domain.QuarantineRestored,
		domain.QuarantinePurged, domain.QuarantineRolledBack:
		return true
	default:
		return false
	}
}

// ============================================================================
// runPurge — delegates to app.PurgeService
//
// Purge is restricted to item-by-item permanent deletion within the managed
// quarantine area (user directive). The service preserves stale checks,
// verification, audit, and rollback per AGENTS.md rule 7.
// ============================================================================

func runPurge(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("purge requires subcommand: plan|approve|execute|recover")
	}
	switch args[0] {
	case "plan":
		return runPurgePlan(args[1:])
	case "approve":
		return runPurgeApprove(args[1:])
	case "execute":
		return runPurgeExecute(args[1:])
	case "recover":
		return runPurgeRecover(args[1:])
	default:
		return fmt.Errorf("unknown purge subcommand")
	}
}

func runPurgePlan(args []string) error {
	fs := flag.NewFlagSet("purge plan", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database (required)")
	out := fs.String("out", "./var/purge-plan.json", "private DRAFT plan report")
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
	svc := app.NewPurgeService(st)
	plans, err := svc.CreatePlans(ctx)
	if err != nil {
		return err
	}
	if err := writePrivateJSON(*out, plans); err != nil {
		return err
	}
	fmt.Printf("created %d DRAFT purge plan(s) in %s (no filesystem writes)\n", len(plans), *out)
	return nil
}

func runPurgeApprove(args []string) error {
	fs := flag.NewFlagSet("purge approve", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database (required)")
	planID := fs.String("plan-id", "", "exact purge plan ID (single item only)")
	digest := fs.String("digest", "", "approval digest from the private DRAFT plan")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *planID == "" || *digest == "" {
		return fmt.Errorf("--db, --plan-id and --digest are required")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	svc := app.NewPurgeService(st)
	if err := svc.ApprovePlan(ctx, *planID, *digest); err != nil {
		return err
	}
	fmt.Println("approved 1 purge plan (paths omitted)")
	return nil
}

func runPurgeExecute(args []string) error {
	fs := flag.NewFlagSet("purge execute", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database (required)")
	planID := fs.String("plan-id", "", "exact approved purge plan ID")
	digest := fs.String("digest", "", "approval digest (required again at execution)")
	quarantineRoot := fs.String("quarantine", "", "managed quarantine root (absolute)")
	out := fs.String("out", "./var/purge-audit.json", "private audit report")
	dryRun := fs.Bool("dry-run", false, "read-only stale and boundary validation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *planID == "" || *digest == "" || *quarantineRoot == "" {
		return fmt.Errorf("--db, --plan-id, --digest and --quarantine are required")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	svc := app.NewPurgeService(st)
	result, err := svc.ExecutePurge(ctx, app.PurgeExecuteInput{
		PlanID:         *planID,
		Digest:         *digest,
		QuarantineRoot: *quarantineRoot,
		DryRun:         *dryRun,
	})
	// Always write the audit file, even on failure, so the operator has
	// a record of what the executor attempted.
	if result != nil {
		if werr := writePrivateJSON(*out, result.Result); werr != nil {
			return werr
		}
	}
	if err != nil {
		return err
	}
	if *dryRun {
		fmt.Printf("purge dry-run passed for 1 plan — audit: %s\n", *out)
	} else {
		fmt.Printf("permanently purged 1 managed quarantine item — audit: %s\n", *out)
	}
	return nil
}

func runPurgeRecover(args []string) error {
	fs := flag.NewFlagSet("purge recover", flag.ContinueOnError)
	dbPath := fs.String("db", "", "SQLite database (required)")
	quarantineRoot := fs.String("quarantine", "", "managed quarantine root (absolute)")
	out := fs.String("out", "./var/purge-recovery.json", "private recovery report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dbPath == "" || *quarantineRoot == "" {
		return fmt.Errorf("--db and --quarantine are required")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	svc := app.NewPurgeService(st)
	results, err := svc.RecoverPurges(ctx, *quarantineRoot)
	if err != nil {
		return err
	}
	if err := writePrivateJSON(*out, results); err != nil {
		return err
	}
	failed := 0
	for _, result := range results {
		if result.Err != nil {
			failed++
		}
	}
	fmt.Printf("purge recovery reconciled %d item(s), %d failed — paths omitted\n", len(results), failed)
	if failed > 0 {
		return fmt.Errorf("purge recovery requires manual review")
	}
	return nil
}
