package app

import (
	"context"
	"fmt"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/executor"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

// ExecutionStore is the store subset needed by ExecutionService.
// store.SQLiteStore satisfies this interface structurally.
type ExecutionStore interface {
	CreateTask(ctx context.Context, task domain.OperationTask) error
	SavePlans(ctx context.Context, taskID string, plans []domain.OperationPlan) error
	AppendLog(ctx context.Context, planID, eventType string, detail map[string]any) error
	RegisterQuarantinesFromJournal(ctx context.Context, planID string, quarantinedAt, purgeAfter time.Time) ([]domain.QuarantineItem, error)
}

// ExecutionInput defines parameters for a batch execution run.
type ExecutionInput struct {
	// Plans is the approved plans to execute.
	Plans []domain.OperationPlan
	// QuarantineRoot is the absolute path to the quarantine directory.
	QuarantineRoot string
	// SourceRoots are the approved task roots (absolute paths).
	SourceRoots []string
	// DryRun validates without executing filesystem actions.
	DryRun bool
	// Retention is the managed quarantine retention period before PURGE
	// eligibility. Minimum 24h.
	Retention time.Duration
}

// ExecutionResult holds the outcome of executing one plan.
type ExecutionResult struct {
	PlanID     string                `json:"plan_id"`
	FinalState domain.PlanState      `json:"final_state"`
	Steps      []executor.AuditStep  `json:"steps"`
	ErrorType  string                `json:"error_type,omitempty"`
	Err        error                 `json:"-"`
}

// ExecutionSummary aggregates the counts from a batch run.
type ExecutionSummary struct {
	Results    []ExecutionResult
	Executed   int
	Skipped    int
	Failed     int
	LifecycleErr error
}

// ExecutionService runs approved plans through the safe-operation pipeline.
// It wraps the executor with task creation, plan persistence, audit logging,
// and managed quarantine registration.
type ExecutionService struct {
	store *store.SQLiteStore // nil for dry-run only
}

// NewExecutionService creates an execution service. The store is required
// for real execution (journal, audit, quarantine registration) and optional
// for dry-run validation.
func NewExecutionService(st *store.SQLiteStore) *ExecutionService {
	return &ExecutionService{store: st}
}

// Execute runs the approved plans. In dry-run mode, plans are validated
// without filesystem writes. In real mode, the executor journal is mandatory
// and each successful quarantine is registered for lifecycle management.
func (s *ExecutionService) Execute(ctx context.Context, in ExecutionInput) (*ExecutionSummary, error) {
	if in.QuarantineRoot == "" {
		return nil, fmt.Errorf("app: Execute: quarantine root is required")
	}
	if len(in.SourceRoots) == 0 {
		return nil, fmt.Errorf("app: Execute: at least one source root is required")
	}
	if !in.DryRun && s.store == nil {
		return nil, fmt.Errorf("app: Execute: database is required for non-dry-run execution")
	}
	if in.Retention < 24*time.Hour {
		return nil, fmt.Errorf("app: Execute: retention must be at least 24h")
	}

	// Create executor.
	qCfg := executor.QuarantineConfig{
		Root:        in.QuarantineRoot,
		Structure:   executor.QuarantineFlat,
		SourceRoots: in.SourceRoots,
	}
	var exec *executor.Executor
	var err error
	if !in.DryRun {
		exec, err = executor.NewWithJournal(qCfg, s.store)
	} else {
		exec, err = executor.New(qCfg)
	}
	if err != nil {
		return nil, err
	}

	// For real execution, create a task and save plans so FK constraints
	// on operation_logs and execution_journal are satisfied.
	if !in.DryRun && s.store != nil {
		taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
		if err := s.store.CreateTask(ctx, domain.OperationTask{
			ID: taskID, RootPath: in.SourceRoots[0], State: "executing", CreatedAt: time.Now(),
		}); err != nil {
			return nil, fmt.Errorf("app: create task: %w", err)
		}
		for i := range in.Plans {
			in.Plans[i].TaskID = taskID
		}
		if err := s.store.SavePlans(ctx, taskID, in.Plans); err != nil {
			return nil, fmt.Errorf("app: save plans: %w", err)
		}
	}

	summary := &ExecutionSummary{Results: make([]ExecutionResult, 0, len(in.Plans))}
	for i := range in.Plans {
		p := &in.Plans[i]
		if p.State != domain.PlanApproved {
			summary.Skipped++
			summary.Results = append(summary.Results, ExecutionResult{
				PlanID:     p.ID,
				FinalState: p.State,
				Steps: []executor.AuditStep{{
					Name:   "skip",
					Status: executor.StepSkipped,
					Detail: map[string]any{"reason": "not approved"},
				}},
			})
			continue
		}
		if in.DryRun {
			result := exec.Validate(ctx, p)
			summary.Results = append(summary.Results, toAppResult(result))
			if result.Err != nil {
				summary.Failed++
			} else {
				summary.Skipped++
			}
			continue
		}
		result := exec.Execute(ctx, p)
		summary.Results = append(summary.Results, toAppResult(result))
		if result.Err != nil {
			summary.Failed++
		} else {
			summary.Executed++
			quarantinedAt := time.Now().UTC()
			if _, err := s.store.RegisterQuarantinesFromJournal(
				ctx, p.ID, quarantinedAt, quarantinedAt.Add(in.Retention),
			); err != nil {
				summary.Executed--
				summary.Failed++
				summary.LifecycleErr = fmt.Errorf("managed quarantine registration failed")
			}
		}
		// Persist audit steps to SQLite.
		if s.store != nil {
			persistAudit(ctx, s.store, result)
		}
	}
	return summary, summary.LifecycleErr
}

// toAppResult converts an executor.Result to an app.ExecutionResult.
func toAppResult(r executor.Result) ExecutionResult {
	return ExecutionResult{
		PlanID:     r.PlanID,
		FinalState: r.FinalState,
		Steps:      r.Steps,
		ErrorType:  r.ErrorType,
		Err:        r.Err,
	}
}

// persistAudit writes each audit step as an operation_log row. Failures are
// silently ignored — the audit JSON file is the primary record, and the
// database is a secondary index for querying.
func persistAudit(ctx context.Context, st ExecutionStore, result executor.Result) {
	if len(result.Steps) == 0 && result.Err != nil {
		_ = st.AppendLog(ctx, result.PlanID, "pipeline_error", map[string]any{
			"status":      "failed",
			"final_state": string(result.FinalState),
			"error_type":  result.ErrorType,
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
		_ = st.AppendLog(ctx, result.PlanID, step.Name, detail)
	}
}
