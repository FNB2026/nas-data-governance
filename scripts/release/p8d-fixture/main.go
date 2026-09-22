// p8d-fixture creates disposable, owner-local project databases for the
// P8-D desktop acceptance gate. It never accepts a database, NAS, or source
// path: every file and database lives in a fresh directory from MkdirTemp.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

const fixtureMarker = ".ndg-p8d-release-fixture"

type manifest struct {
	Kind            string `json:"kind"`
	FixtureRoot     string `json:"fixture_root"`
	DatabasePath    string `json:"database_path"`
	SourceRoot      string `json:"source_root"`
	QuarantineRoot  string `json:"quarantine_root,omitempty"`
	PlanID          string `json:"plan_id"`
	QuarantineID    string `json:"quarantine_item_id,omitempty"`
	SHA256          string `json:"sha256"`
	ExpectedSize    int64  `json:"expected_size"`
	RetentionHours  int    `json:"retention_hours,omitempty"`
	FixtureContract string `json:"fixture_contract"`
}

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "k5" && os.Args[1] != "k6") {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/release/p8d-fixture <k5|k6>")
		os.Exit(2)
	}

	var (
		m   manifest
		err error
	)
	if os.Args[1] == "k5" {
		m, err = createK5(context.Background())
	} else {
		m, err = createK6(context.Background())
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "p8d-fixture: %v\n", err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(m); err != nil {
		fmt.Fprintf(os.Stderr, "p8d-fixture: encode manifest: %v\n", err)
		os.Exit(1)
	}
}

func createK5(ctx context.Context) (manifest, error) {
	root, st, sourceRoot, quarantineRoot, err := newFixture(ctx, "k5")
	if err != nil {
		return manifest{}, err
	}
	defer st.Close() //nolint:errcheck

	const name = "eligible-for-purge.txt"
	content := []byte("P8-D K5 disposable purge fixture\n")
	qPath := filepath.Join(quarantineRoot, name)
	if err := os.WriteFile(qPath, content, 0o600); err != nil {
		return manifest{}, fmt.Errorf("write quarantine file: %w", err)
	}
	snapshot, err := executor.Snapshot(qPath, true)
	if err != nil {
		return manifest{}, fmt.Errorf("snapshot quarantine file: %w", err)
	}

	// This is a fixture for a completed history: 25 elapsed hours of a legal
	// 24-hour minimum retention period, not a runtime override of that policy.
	now := time.Now().UTC()
	quarantinedAt := now.Add(-25 * time.Hour)
	retainUntil := now.Add(-1 * time.Hour)
	planID := "p8d-k5-quarantine-history"
	taskID := "p8d-k5-task"
	sourcePath := filepath.Join(sourceRoot, name)
	actions := []domain.PlannedAction{{
		Path: sourcePath, Action: domain.OperationQuarantine,
		Reason:  "P8-D K5 disposable history fixture",
		Context: domain.DirectoryContext{Role: domain.RoleTemporary},
		File:    domain.FileInstance{Path: sourcePath, Size: snapshot.Size, ContentSHA256: snapshot.Hash},
	}}
	if err := st.CreateTask(ctx, domain.OperationTask{ID: taskID, RootPath: sourceRoot, State: string(domain.TaskCompleted), CreatedAt: quarantinedAt}); err != nil {
		return manifest{}, fmt.Errorf("create task: %w", err)
	}
	if err := st.SavePlans(ctx, taskID, []domain.OperationPlan{{
		ID: planID, TaskID: taskID, State: domain.PlanApproved, Risk: domain.RiskLow, Actions: actions,
		Evidence: []string{"P8-D disposable fixture; historical retention elapsed"},
	}}); err != nil {
		return manifest{}, fmt.Errorf("save plan: %w", err)
	}
	if err := st.BeginJournal(ctx, taskID, planID, actions); err != nil {
		return manifest{}, fmt.Errorf("begin journal: %w", err)
	}
	if err := st.MarkJournalDone(ctx, planID, 0, qPath); err != nil {
		return manifest{}, fmt.Errorf("mark journal done: %w", err)
	}
	items, err := st.RegisterQuarantinesFromJournal(ctx, planID, quarantinedAt, retainUntil)
	if err != nil || len(items) != 1 {
		return manifest{}, fmt.Errorf("register quarantine history: items=%d err=%w", len(items), err)
	}
	return buildManifest("k5", root, sourceRoot, quarantineRoot, planID, items[0].ID, snapshot.Hash, snapshot.Size, 24), nil
}

func createK6(ctx context.Context) (manifest, error) {
	root, st, sourceRoot, _, err := newFixture(ctx, "k6")
	if err != nil {
		return manifest{}, err
	}
	defer st.Close() //nolint:errcheck

	const name = "recovery-lock-source.txt"
	content := []byte("P8-D K6 untouched recovery fixture\n")
	sourcePath := filepath.Join(sourceRoot, name)
	if err := os.WriteFile(sourcePath, content, 0o600); err != nil {
		return manifest{}, fmt.Errorf("write source file: %w", err)
	}
	snapshot, err := executor.Snapshot(sourcePath, true)
	if err != nil {
		return manifest{}, fmt.Errorf("snapshot source file: %w", err)
	}
	planID := "p8d-k6-interrupted-plan"
	taskID := "p8d-k6-task"
	actions := []domain.PlannedAction{{
		Path: sourcePath, Action: domain.OperationQuarantine,
		Reason:  "P8-D K6 interrupted-before-write fixture",
		Context: domain.DirectoryContext{Role: domain.RoleTemporary},
		File:    domain.FileInstance{Path: sourcePath, Size: snapshot.Size, ContentSHA256: snapshot.Hash},
	}}
	if err := st.CreateTask(ctx, domain.OperationTask{ID: taskID, RootPath: sourceRoot, State: string(domain.TaskCompleted), CreatedAt: time.Now().UTC()}); err != nil {
		return manifest{}, fmt.Errorf("create task: %w", err)
	}
	if err := st.SavePlans(ctx, taskID, []domain.OperationPlan{{
		ID: planID, TaskID: taskID, State: domain.PlanApproved, Risk: domain.RiskLow, Actions: actions,
		Evidence: []string{"P8-D disposable fixture; interrupted before filesystem write"},
	}}); err != nil {
		return manifest{}, fmt.Errorf("save plan: %w", err)
	}
	if err := st.BeginJournal(ctx, taskID, planID, actions); err != nil {
		return manifest{}, fmt.Errorf("begin journal: %w", err)
	}
	if err := st.UpdatePlanState(ctx, planID, domain.PlanExecuting); err != nil {
		return manifest{}, fmt.Errorf("mark interrupted plan executing: %w", err)
	}
	return buildManifest("k6", root, sourceRoot, "", planID, "", snapshot.Hash, snapshot.Size, 0), nil
}

func newFixture(ctx context.Context, kind string) (string, *store.SQLiteStore, string, string, error) {
	root, err := os.MkdirTemp("", "ndg-p8d-"+kind+"-")
	if err != nil {
		return "", nil, "", "", fmt.Errorf("create fixture root: %w", err)
	}
	fail := func(err error) (string, *store.SQLiteStore, string, string, error) {
		_ = os.RemoveAll(root)
		return "", nil, "", "", err
	}
	if err := os.WriteFile(filepath.Join(root, fixtureMarker), []byte("disposable P8-D release fixture\n"), 0o600); err != nil {
		return fail(fmt.Errorf("write fixture marker: %w", err))
	}
	sourceRoot := filepath.Join(root, "source")
	quarantineRoot := filepath.Join(root, "quarantine")
	if err := os.MkdirAll(sourceRoot, 0o700); err != nil {
		return fail(fmt.Errorf("create source root: %w", err))
	}
	if err := os.MkdirAll(quarantineRoot, 0o700); err != nil {
		return fail(fmt.Errorf("create quarantine root: %w", err))
	}
	st, err := store.Open(ctx, filepath.Join(root, "governance.db"))
	if err != nil {
		return fail(fmt.Errorf("create fixture database: %w", err))
	}
	return root, st, sourceRoot, quarantineRoot, nil
}

func buildManifest(kind, root, sourceRoot, quarantineRoot, planID, itemID, sha string, size int64, retentionHours int) manifest {
	return manifest{
		Kind: kind, FixtureRoot: root, DatabasePath: filepath.Join(root, "governance.db"),
		SourceRoot: sourceRoot, QuarantineRoot: quarantineRoot, PlanID: planID,
		QuarantineID: itemID, SHA256: sha, ExpectedSize: size, RetentionHours: retentionHours,
		FixtureContract: "Created only with MkdirTemp; no input paths; no production, NAS, or existing project database is opened.",
	}
}
