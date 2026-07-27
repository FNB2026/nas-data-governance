package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/purge"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

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
	items, err := st.ListQuarantineItems(ctx, status)
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
	item, err := st.GetQuarantineItem(ctx, *itemID)
	if err != nil {
		return err
	}
	plan, err := purge.BuildRestorePlan(item, time.Now().UTC())
	if err != nil {
		return err
	}
	if err := st.SaveRestorePlan(ctx, plan); err != nil {
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
	if err := st.ApproveRestorePlan(ctx, *planID, *digest, time.Now().UTC()); err != nil {
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
	plan, err := st.GetRestorePlan(ctx, *planID)
	if err != nil {
		return err
	}
	if plan.ApprovalDigest != *digest {
		return fmt.Errorf("restore execution digest rejected")
	}
	item, err := st.GetQuarantineItem(ctx, plan.ItemID)
	if err != nil {
		return err
	}
	exec, err := executor.NewRestoreExecutor(*quarantineRoot, sourceRoots, st)
	if err != nil {
		return err
	}
	var result executor.RestoreResult
	if *dryRun {
		result = exec.ValidateRestore(ctx, plan, item)
	} else {
		result = exec.ExecuteRestore(ctx, &plan, &item)
	}
	if err := writePrivateJSON(*out, result); err != nil {
		return err
	}
	if result.Err != nil {
		return fmt.Errorf("restore failed: %s", result.ErrorType)
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
	exec, err := executor.NewRestoreExecutor(*quarantineRoot, sourceRoots, st)
	if err != nil {
		return err
	}
	results := exec.RecoverRestores(ctx)
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
	now := time.Now().UTC()
	items, err := st.ListPurgeCandidates(ctx, now)
	if err != nil {
		return err
	}
	plans := purge.BuildPlans(items, now)
	if len(plans) > 0 {
		if err := st.SavePurgePlans(ctx, plans); err != nil {
			return err
		}
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
	if err := st.ApprovePurgePlan(ctx, *planID, *digest, time.Now().UTC()); err != nil {
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
	plan, err := st.GetPurgePlan(ctx, *planID)
	if err != nil {
		return err
	}
	if plan.ApprovalDigest != *digest {
		return fmt.Errorf("purge execution digest rejected")
	}
	item, err := st.GetQuarantineItem(ctx, plan.ItemID)
	if err != nil {
		return err
	}
	exec, err := executor.NewPurgeExecutor(*quarantineRoot, st)
	if err != nil {
		return err
	}
	var result executor.PurgeResult
	if *dryRun {
		result = exec.ValidatePurge(ctx, plan, item)
	} else {
		result = exec.ExecutePurge(ctx, &plan, &item)
	}
	if err := writePrivateJSON(*out, result); err != nil {
		return err
	}
	if result.Err != nil {
		return fmt.Errorf("purge failed: %s", result.ErrorType)
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
	exec, err := executor.NewPurgeExecutor(*quarantineRoot, st)
	if err != nil {
		return err
	}
	results := exec.RecoverPurges(ctx)
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
