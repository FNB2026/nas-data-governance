package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func seedCLIQuarantineItem(
	t *testing.T,
	dbPath, sourceRoot, quarantineRoot, name string,
	quarantinedAt, retainUntil time.Time,
) domain.QuarantineItem {
	t.Helper()
	ctx := context.Background()
	qPath := filepath.Join(quarantineRoot, name)
	if err := os.WriteFile(qPath, []byte("m7-"+name), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := executor.Snapshot(qPath, true)
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	taskID := "task-" + name
	planID := "plan-" + name
	sourcePath := filepath.Join(sourceRoot, name)
	if err := st.CreateTask(ctx, domain.OperationTask{
		ID: taskID, RootPath: sourceRoot, State: "completed", CreatedAt: quarantinedAt,
	}); err != nil {
		t.Fatal(err)
	}
	action := domain.PlannedAction{
		Path: sourcePath, Action: domain.OperationQuarantine,
		Context: domain.DirectoryContext{Role: domain.RoleTemporary},
		File: domain.FileInstance{
			Path: sourcePath, Size: snapshot.Size, ContentSHA256: snapshot.Hash,
		},
	}
	plan := domain.OperationPlan{
		ID: planID, TaskID: taskID, State: domain.PlanApproved,
		Actions: []domain.PlannedAction{action},
	}
	if err := st.SavePlans(ctx, taskID, []domain.OperationPlan{plan}); err != nil {
		t.Fatal(err)
	}
	if err := st.BeginJournal(ctx, taskID, planID, plan.Actions); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkJournalDone(ctx, planID, 0, qPath); err != nil {
		t.Fatal(err)
	}
	items, err := st.RegisterQuarantinesFromJournal(ctx, planID, quarantinedAt, retainUntil)
	if err != nil || len(items) != 1 {
		t.Fatalf("register item: len=%d err=%v", len(items), err)
	}
	return items[0]
}

func TestM7RestoreAndPermanentPurgeCLI(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "m7.db")
	sourceRoot := filepath.Join(tmp, "source")
	quarantineRoot := filepath.Join(tmp, "quarantine")
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(quarantineRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()

	restoreItem := seedCLIQuarantineItem(
		t, dbPath, sourceRoot, quarantineRoot, "restore.dat",
		now.Add(-time.Hour), now.Add(30*24*time.Hour),
	)
	restorePlanPath := filepath.Join(tmp, "restore-plan.json")
	if err := runRestorePlan([]string{
		"--db", dbPath, "--item-id", restoreItem.ID, "--out", restorePlanPath,
	}); err != nil {
		t.Fatal(err)
	}
	var restorePlan domain.RestorePlan
	readJSONFile(t, restorePlanPath, &restorePlan)
	if err := runRestoreApprove([]string{
		"--db", dbPath, "--plan-id", restorePlan.ID, "--digest", restorePlan.ApprovalDigest,
	}); err != nil {
		t.Fatal(err)
	}
	if err := runRestoreExecute([]string{
		"--db", dbPath, "--plan-id", restorePlan.ID, "--digest", restorePlan.ApprovalDigest,
		"--quarantine", quarantineRoot, "--source-root", sourceRoot, "--dry-run",
		"--out", filepath.Join(tmp, "restore-dry.json"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(restoreItem.QuarantinePath); err != nil {
		t.Fatalf("restore dry-run changed quarantine: %v", err)
	}
	if err := runRestoreExecute([]string{
		"--db", dbPath, "--plan-id", restorePlan.ID, "--digest", restorePlan.ApprovalDigest,
		"--quarantine", quarantineRoot, "--source-root", sourceRoot,
		"--out", filepath.Join(tmp, "restore-audit.json"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(restoreItem.SourcePath); err != nil {
		t.Fatalf("restored source missing: %v", err)
	}

	purgeItem := seedCLIQuarantineItem(
		t, dbPath, sourceRoot, quarantineRoot, "purge.dat",
		now.Add(-48*time.Hour), now.Add(-24*time.Hour),
	)
	purgePlanPath := filepath.Join(tmp, "purge-plan.json")
	if err := runPurgePlan([]string{"--db", dbPath, "--out", purgePlanPath}); err != nil {
		t.Fatal(err)
	}
	var purgePlans []domain.PurgePlan
	data, err := os.ReadFile(purgePlanPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &purgePlans); err != nil {
		t.Fatal(err)
	}
	if len(purgePlans) != 1 || purgePlans[0].ItemID != purgeItem.ID {
		t.Fatalf("unexpected purge plans: %#v", purgePlans)
	}
	purgePlan := purgePlans[0]
	secondPlanPath := filepath.Join(tmp, "purge-plan-second.json")
	if err := runPurgePlan([]string{"--db", dbPath, "--out", secondPlanPath}); err != nil {
		t.Fatal(err)
	}
	var secondPlans []domain.PurgePlan
	readJSONFile(t, secondPlanPath, &secondPlans)
	if len(secondPlans) != 0 {
		t.Fatalf("repeated planning duplicated an active plan: %#v", secondPlans)
	}
	if err := runPurgeApprove([]string{
		"--db", dbPath, "--plan-id", purgePlan.ID, "--digest", purgePlan.ApprovalDigest,
	}); err != nil {
		t.Fatal(err)
	}
	if err := runPurgeExecute([]string{
		"--db", dbPath, "--plan-id", purgePlan.ID, "--digest", purgePlan.ApprovalDigest,
		"--quarantine", quarantineRoot, "--dry-run",
		"--out", filepath.Join(tmp, "purge-dry.json"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(purgeItem.QuarantinePath); err != nil {
		t.Fatalf("purge dry-run changed item: %v", err)
	}
	if err := runPurgeExecute([]string{
		"--db", dbPath, "--plan-id", purgePlan.ID, "--digest", purgePlan.ApprovalDigest,
		"--quarantine", quarantineRoot,
		"--out", filepath.Join(tmp, "purge-audit.json"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(purgeItem.QuarantinePath); !os.IsNotExist(err) {
		t.Fatalf("purged item still exists: %v", err)
	}
}

func TestM7L1CopiedFixtureDryRunQuarantineAndRollback(t *testing.T) {
	tmp := t.TempDir()
	sourceRoot := filepath.Join(tmp, "copied-fixture")
	quarantineRoot := filepath.Join(tmp, "managed-quarantine")
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(quarantineRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("copied test data for M7 L1 drill")
	keepPath := filepath.Join(sourceRoot, "keep.dat")
	candidatePath := filepath.Join(sourceRoot, "candidate.dat")
	for _, path := range []string{keepPath, candidatePath} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	keepSnapshot, err := executor.Snapshot(keepPath, true)
	if err != nil {
		t.Fatal(err)
	}
	candidateSnapshot, err := executor.Snapshot(candidatePath, true)
	if err != nil {
		t.Fatal(err)
	}
	plan := domain.OperationPlan{
		ID: "m7-l1-copied-fixture", State: domain.PlanDraft, Risk: domain.RiskMedium,
		ContentSHA256: keepSnapshot.Hash, Size: keepSnapshot.Size, RetainPath: keepPath,
		Actions: []domain.PlannedAction{
			{Path: keepPath, Action: domain.OperationKeep, Reason: "fixture control copy", File: domain.FileInstance{Path: keepPath, Size: keepSnapshot.Size, ModifiedAt: keepSnapshot.ModifiedAt, Device: keepSnapshot.Device, Inode: keepSnapshot.Inode, ContentSHA256: keepSnapshot.Hash}},
			{Path: candidatePath, Action: domain.OperationQuarantine, Reason: "L1 copied-fixture drill", Context: domain.DirectoryContext{Role: domain.RoleTemporary}, File: domain.FileInstance{Path: candidatePath, Size: candidateSnapshot.Size, ModifiedAt: candidateSnapshot.ModifiedAt, Device: candidateSnapshot.Device, Inode: candidateSnapshot.Inode, ContentSHA256: candidateSnapshot.Hash}},
		},
	}
	planPath := filepath.Join(tmp, "draft.json")
	if err := writePrivateJSON(planPath, []domain.OperationPlan{plan}); err != nil {
		t.Fatal(err)
	}
	approvedPath := filepath.Join(tmp, "approved.json")
	if err := runApprove([]string{"--plan", planPath, "--out", approvedPath, "--plan-id", plan.ID}); err != nil {
		t.Fatal(err)
	}
	dryDBPath := filepath.Join(tmp, "l1-dry.db")
	if err := runExecute([]string{
		"--plan", approvedPath, "--out", filepath.Join(tmp, "dry-run.json"),
		"--quarantine", quarantineRoot, "--source-root", sourceRoot, "--db", dryDBPath, "--dry-run",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(candidatePath); err != nil {
		t.Fatalf("dry-run changed fixture: %v", err)
	}
	dbPath := filepath.Join(tmp, "l1.db")
	if err := runExecute([]string{
		"--plan", approvedPath, "--out", filepath.Join(tmp, "quarantine-audit.json"),
		"--quarantine", quarantineRoot, "--source-root", sourceRoot, "--db", dbPath,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(candidatePath); !os.IsNotExist(err) {
		t.Fatalf("candidate was not quarantined: %v", err)
	}
	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	items, err := st.ListQuarantineItems(context.Background(), domain.QuarantineActive)
	_ = st.Close()
	if err != nil || len(items) != 1 {
		t.Fatalf("managed quarantine registration: len=%d err=%v", len(items), err)
	}
	restorePlanPath := filepath.Join(tmp, "restore-plan.json")
	if err := runRestorePlan([]string{"--db", dbPath, "--item-id", items[0].ID, "--out", restorePlanPath}); err != nil {
		t.Fatal(err)
	}
	var restorePlan domain.RestorePlan
	readJSONFile(t, restorePlanPath, &restorePlan)
	if err := runRestoreApprove([]string{"--db", dbPath, "--plan-id", restorePlan.ID, "--digest", restorePlan.ApprovalDigest}); err != nil {
		t.Fatal(err)
	}
	if err := runRestoreExecute([]string{
		"--db", dbPath, "--plan-id", restorePlan.ID, "--digest", restorePlan.ApprovalDigest,
		"--quarantine", quarantineRoot, "--source-root", sourceRoot,
		"--out", filepath.Join(tmp, "rollback-audit.json"),
	}); err != nil {
		t.Fatal(err)
	}
	if restored, err := os.ReadFile(candidatePath); err != nil || string(restored) != string(content) {
		t.Fatalf("rollback did not restore fixture: %v", err)
	}
}
