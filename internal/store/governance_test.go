package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func governanceFixture(t *testing.T, n int) (*SQLiteStore, []domain.GovernanceSubject, map[string]string) {
	t.Helper()
	ctx := context.Background()
	s := newTestStore(t)
	root := t.TempDir()
	if e := s.RegisterStorage(ctx, domain.Storage{ID: "synthetic", RootPath: root, CreatedAt: time.Now()}); e != nil {
		t.Fatal(e)
	}
	files := []domain.FileInstance{}
	before := map[string]string{}
	for i := 0; i < n; i++ {
		data := []byte(fmt.Sprintf("disposable-%04d", i))
		sum := sha256.Sum256(data)
		h := hex.EncodeToString(sum[:])
		for j := 0; j < 2; j++ {
			name := fmt.Sprintf("synthetic_%04d_%d", i, j)
			path := filepath.Join(root, name)
			if e := os.WriteFile(path, data, 0600); e != nil {
				t.Fatal(e)
			}
			before[path] = h
			files = append(files, domain.FileInstance{StorageID: "synthetic", Path: path, Name: name, Size: int64(len(data)), ModifiedAt: time.Now(), DiscoveredAt: time.Now(), ContentSHA256: h, QuickHash: h})
		}
	}
	if _, e := s.UpsertFiles(ctx, files); e != nil {
		t.Fatal(e)
	}
	p, e := s.ListGovernanceGroups(ctx, domain.GovernanceQuery{PageSize: 200})
	if e != nil {
		t.Fatal(e)
	}
	refs := []domain.GovernanceSubject{}
	for _, g := range p.Items {
		refs = append(refs, g.GovernanceSubject)
	}
	if len(refs) != n {
		t.Fatal("fixture coverage", len(refs))
	}
	return s, refs, before
}
func checkSourceBytes(t *testing.T, before map[string]string) {
	t.Helper()
	for path, h := range before {
		data, e := os.ReadFile(path)
		if e != nil {
			t.Fatal("source unavailable")
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != h {
			t.Fatal("source bytes changed")
		}
	}
}
func previewFor(t *testing.T, s *SQLiteStore, refs []domain.GovernanceSubject, kind domain.ReviewDecisionType) domain.GovernancePreview {
	t.Helper()
	p, e := s.PreviewBatchDecisions(context.Background(), domain.PreviewBatchRequest{Items: refs, DecisionType: kind, Reason: "synthetic reason"}, true)
	if e != nil || p.PreviewToken == "" {
		t.Fatal("preview failed", e)
	}
	return p
}
func auditCount(t *testing.T, s *SQLiteStore) int {
	t.Helper()
	var n int
	if s.db.QueryRow(`SELECT COUNT(*) FROM governance_decision_audit`).Scan(&n) != nil {
		t.Fatal("audit query")
	}
	return n
}

func TestBatchHundredItemsReplayRestartAndSourceConservation(t *testing.T) {
	runtimeKey0 := governanceTestKey(t)
	s, refs, before := governanceFixture(t, 100)
	p := previewFor(t, s, refs, domain.DecisionDraftAction)
	req := domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: runtimeKey0}
	v, e := s.CommitBatchDecisions(context.Background(), req)
	if e != nil || v.Status != "COMPLETE" || v.Counts.Saved != 100 || v.Counts.Total != 100 || v.RequestedCount != 100 {
		t.Fatal("batch coverage", v.Counts, e)
	}
	var plans int
	s.db.QueryRow(`SELECT COUNT(*) FROM operation_plans`).Scan(&plans)
	if plans != 0 {
		t.Fatal("intent created plans")
	}
	if auditCount(t, s) != 100 {
		t.Fatal("audit mismatch")
	}
	var wg sync.WaitGroup
	results := make(chan domain.GovernanceBatchResult, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.CommitBatchDecisions(context.Background(), req)
			if err != nil {
				t.Error(err)
			}
			results <- r
		}()
	}
	wg.Wait()
	close(results)
	for got := range results {
		if !reflect.DeepEqual(got, v) {
			t.Fatal("replay changed receipt")
		}
	}
	if auditCount(t, s) != 100 {
		t.Fatal("replay duplicated audit")
	}
	path := s.path
	if s.Close() != nil {
		t.Fatal("close")
	}
	reopened, e := Open(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	got, e := reopened.CommitBatchDecisions(context.Background(), req)
	if e != nil || !reflect.DeepEqual(got, v) {
		t.Fatal("restart replay")
	}
	checkSourceBytes(t, before)
}

func TestBatchVersionsPartialConflictAndUnchanged(t *testing.T) {
	runtimeKey0 := governanceTestKey(t)
	runtimeKey1 := governanceTestKey(t)
	s, refs, before := governanceFixture(t, 3)
	p := previewFor(t, s, refs, domain.DecisionKeepAll)
	if _, e := s.db.Exec(`UPDATE file_instances SET size=size+1 WHERE storage_id=? AND content_sha256=?`, refs[0].StorageID, refs[0].SHA256); e != nil {
		t.Fatal(e)
	}
	v, e := s.CommitBatchDecisions(context.Background(), domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: runtimeKey0})
	if e != nil || v.Counts.Conflict != 1 || v.Counts.Saved != 2 || v.Counts.Total != 3 {
		t.Fatal(v.Counts, e)
	}
	page, e := s.ListGovernanceGroups(context.Background(), domain.GovernanceQuery{DecisionType: "KEEP_ALL"})
	if e != nil || page.TotalCount != 2 {
		t.Fatal(page.TotalCount, e)
	}
	fresh := []domain.GovernanceSubject{}
	for _, g := range page.Items {
		fresh = append(fresh, g.GovernanceSubject)
	}
	p = previewFor(t, s, fresh, domain.DecisionKeepAll)
	v, e = s.CommitBatchDecisions(context.Background(), domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: runtimeKey1})
	if e != nil || v.Counts.Unchanged != 2 {
		t.Fatal(v.Counts, e)
	}
	checkSourceBytes(t, before)
}

func TestBatchDurableFailureResumeAndAtomicAudit(t *testing.T) {
	runtimeKey0 := governanceTestKey(t)
	s, refs, _ := governanceFixture(t, 3)
	p := previewFor(t, s, refs, domain.DecisionDeferred)
	if _, e := s.db.Exec(`CREATE TRIGGER synthetic_audit_failure BEFORE INSERT ON governance_decision_audit WHEN NEW.group_id='` + refs[1].GroupID + `' BEGIN SELECT RAISE(ABORT,'private_failure_anchor'); END`); e != nil {
		t.Fatal(e)
	}
	req := domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: runtimeKey0}
	v, e := s.CommitBatchDecisions(context.Background(), req)
	if e != nil || v.Status != "IN_PROGRESS" || v.Counts.Saved != 1 || v.Counts.Failed != 2 {
		t.Fatal("failure classification", v.Counts, e)
	}
	if _, e = s.GetGroupDecision(context.Background(), refs[1].GroupID); e == nil {
		t.Fatal("decision without audit")
	}
	if auditCount(t, s) != 1 {
		t.Fatal("failed audit not atomic")
	}
	path := s.path
	s.Close()
	s, e = Open(context.Background(), path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if _, e = s.db.Exec(`DROP TRIGGER synthetic_audit_failure`); e != nil {
		t.Fatal(e)
	}
	v, e = s.CommitBatchDecisions(context.Background(), req)
	if e != nil || v.Status != "COMPLETE" || v.Counts.Saved != 3 || auditCount(t, s) != 3 {
		t.Fatal("durable resume", v.Counts, e)
	}
	encoded, _ := json.Marshal(v)
	if strings.Contains(string(encoded), "private_failure_anchor") {
		t.Fatal("private error leaked")
	}
}

func TestBatchPolicyExpiryScopeAndBodyGuards(t *testing.T) {
	runtimeKey0 := governanceTestKey(t)
	runtimeKey1 := governanceTestKey(t)
	s, refs, _ := governanceFixture(t, 2)
	ctx := context.Background()
	p := previewFor(t, s, refs, domain.DecisionKeepAll)
	req := domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: runtimeKey0}
	if _, e := s.CommitBatchDecisions(ctx, req); e != nil {
		t.Fatal(e)
	}
	other := previewFor(t, s, refs, domain.DecisionDeferred)
	if _, e := s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: other.PreviewToken, IdempotencyKey: req.IdempotencyKey}); e == nil {
		t.Fatal("same key different body accepted")
	}
	s.db.Exec(`UPDATE governance_previews SET expires_at=? WHERE token=?`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano), other.PreviewToken)
	if _, e := s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: other.PreviewToken, IdempotencyKey: runtimeKey1}); e == nil {
		t.Fatal("expired token accepted")
	}
	otherDB := newTestStore(t)
	if _, e := otherDB.CommitBatchDecisions(ctx, req); e == nil {
		t.Fatal("foreign project token accepted")
	}
	for _, input := range []domain.PreviewBatchRequest{{Items: refs, DecisionType: domain.DecisionPrimaryRetention}, {Items: append(refs, refs[0]), DecisionType: domain.DecisionKeepAll}, {Items: refs, DecisionType: domain.DecisionKeepAll, Reason: strings.Repeat("x", 2049)}} {
		if _, e := s.PreviewBatchDecisions(ctx, input, true); e == nil {
			t.Fatal("invalid preview accepted")
		}
	}
	p, e := s.PreviewBatchDecisions(ctx, domain.PreviewBatchRequest{Items: refs, DecisionType: domain.DecisionKeepAll}, false)
	if e != nil || p.PreviewToken != "" || p.Items[0].Eligibility != "BLOCKED" {
		t.Fatal("read-only preview")
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM governance_previews`).Scan(&n)
	if n != 2 {
		t.Fatal("read-only preview persisted")
	}
}

func TestBatchDecisionCASRulesProtectionAndGuardChanges(t *testing.T) {
	runtimeKey0 := governanceTestKey(t)
	runtimeKey1 := governanceTestKey(t)
	runtimeKey2 := governanceTestKey(t)
	runtimeKey3 := governanceTestKey(t)
	s, refs, before := governanceFixture(t, 2)
	ctx := context.Background()
	p1 := previewFor(t, s, refs, domain.DecisionKeepAll)
	p2 := previewFor(t, s, refs, domain.DecisionDeferred)
	if _, e := s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: p1.PreviewToken, IdempotencyKey: runtimeKey0}); e != nil {
		t.Fatal(e)
	}
	v, e := s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: p2.PreviewToken, IdempotencyKey: runtimeKey1})
	if e != nil || v.Counts.Conflict != 2 {
		t.Fatal("decision CAS", v.Counts, e)
	}
	page, e := s.ListGovernanceGroups(ctx, domain.GovernanceQuery{})
	if e != nil {
		t.Fatal(e)
	}
	fresh := []domain.GovernanceSubject{}
	for _, g := range page.Items {
		fresh = append(fresh, g.GovernanceSubject)
	}
	p3 := previewFor(t, s, fresh, domain.DecisionCrossArchive)
	if _, e = s.db.Exec(`INSERT INTO protected_paths(storage_id,path_prefix,reason) VALUES('synthetic','/synthetic/protected','synthetic HOLD')`); e != nil {
		t.Fatal(e)
	}
	v, e = s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: p3.PreviewToken, IdempotencyKey: runtimeKey2})
	if e != nil || v.Counts.Conflict != 2 {
		t.Fatal("protection snapshot changed", v.Counts, e)
	}
	page, _ = s.ListGovernanceGroups(ctx, domain.GovernanceQuery{})
	fresh = nil
	for _, g := range page.Items {
		fresh = append(fresh, g.GovernanceSubject)
	}
	p4 := previewFor(t, s, fresh, domain.DecisionBackupRelation)
	if _, e = s.db.Exec(`INSERT INTO job_runs(id,project_id,job_type,state,stage,progress_json,created_at) VALUES('synthetic-job','synthetic','scan','RUNNING','DISCOVERING','{}',?)`, time.Now().UTC().Format(time.RFC3339Nano)); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: p4.PreviewToken, IdempotencyKey: runtimeKey3}); e == nil {
		t.Fatal("active scan recording accepted")
	}
	checkSourceBytes(t, before)
}

func TestCopiedDatabasePreservesHistoryButRejectsOriginalPreview(t *testing.T) {
	runtimeKey0 := governanceTestKey(t)
	runtimeKey1 := governanceTestKey(t)
	s, refs, _ := governanceFixture(t, 2)
	ctx := context.Background()
	p := previewFor(t, s, refs, domain.DecisionKeepAll)
	first, e := s.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: runtimeKey0})
	if e != nil {
		t.Fatal(e)
	}
	page, _ := s.ListGovernanceGroups(ctx, domain.GovernanceQuery{})
	fresh := []domain.GovernanceSubject{}
	for _, g := range page.Items {
		fresh = append(fresh, g.GovernanceSubject)
	}
	pending := previewFor(t, s, fresh, domain.DecisionDeferred)
	scope, e := s.GovernanceCapabilities(ctx)
	if e != nil {
		t.Fatal(e)
	}
	path := s.path
	if s.Close() != nil {
		t.Fatal("close")
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	copyPath := filepath.Join(t.TempDir(), "private-copy.db")
	if os.WriteFile(copyPath, data, 0600) != nil {
		t.Fatal("copy fixture")
	}
	copyStore, e := Open(ctx, copyPath)
	if e != nil {
		t.Fatal(e)
	}
	defer copyStore.Close()
	copyScope, e := copyStore.GovernanceCapabilities(ctx)
	if e != nil || copyScope.ProjectScope == scope.ProjectScope {
		t.Fatal("clone identity not isolated")
	}
	if _, e = copyStore.CommitBatchDecisions(ctx, domain.CommitBatchRequest{PreviewToken: pending.PreviewToken, IdempotencyKey: runtimeKey1}); e == nil {
		t.Fatal("original preview replayed in copied database")
	}
	history, e := copyStore.GetBatchDecisionResult(ctx, first.BatchID)
	if e != nil || history.Counts.Saved != 2 || history.ProjectScope != copyScope.ProjectScope {
		t.Fatal("private historical receipt lost", e)
	}
}

func TestOutcomePersistenceFailureAndCancelledCallNeverReportSaved(t *testing.T) {
	runtimeKey0 := governanceTestKey(t)
	s, refs, before := governanceFixture(t, 2)
	p := previewFor(t, s, refs, domain.DecisionKeepAll)
	req := domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: runtimeKey0}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := s.CommitBatchDecisions(ctx, req); e == nil {
		t.Fatal("cancelled request accepted")
	}
	if auditCount(t, s) != 0 {
		t.Fatal("cancelled request wrote audit")
	}
	if _, e := s.db.Exec(`CREATE TRIGGER synthetic_outcome_failure BEFORE INSERT ON governance_outcomes BEGIN SELECT RAISE(ABORT,'private_outcome_anchor'); END`); e != nil {
		t.Fatal(e)
	}
	v, e := s.CommitBatchDecisions(context.Background(), req)
	if e != nil || v.Status != "IN_PROGRESS" || v.Counts.Saved != 0 || v.Counts.Failed != 2 || auditCount(t, s) != 0 {
		t.Fatal("outcome failure misreported", v.Counts, e)
	}
	for _, ref := range refs {
		if _, e = s.GetGroupDecision(context.Background(), ref.GroupID); e == nil {
			t.Fatal("outcome failure persisted decision")
		}
	}
	got, e := s.GetBatchDecisionResult(context.Background(), "key:"+req.IdempotencyKey)
	if e != nil || got.BatchID != v.BatchID || got.Counts.Failed != 2 {
		t.Fatal("lost response key lookup", e)
	}
	checkSourceBytes(t, before)
}

func TestCriticalProtectedIntentNeverApprovesOrClearsRecoveryLock(t *testing.T) {
	runtimeKey0 := governanceTestKey(t)
	s, refs, before := governanceFixture(t, 2)
	ctx := context.Background()
	task := domain.OperationTask{ID: "protected-task", RootPath: "/synthetic", State: "planning", CreatedAt: time.Now()}
	if e := s.CreateTask(ctx, task); e != nil {
		t.Fatal(e)
	}
	plan := domain.OperationPlan{ID: "protected-plan", TaskID: task.ID, GroupID: refs[0].GroupID, State: domain.PlanDraft, Risk: domain.RiskCritical, Actions: []domain.PlannedAction{{Path: "/synthetic/protected", Action: domain.OperationReview, Context: domain.DirectoryContext{Protected: true}}}}
	if e := s.SavePlans(ctx, task.ID, []domain.OperationPlan{plan}); e != nil {
		t.Fatal(e)
	}
	page, _ := s.ListGovernanceGroups(ctx, domain.GovernanceQuery{})
	fresh := []domain.GovernanceSubject{}
	for _, g := range page.Items {
		fresh = append(fresh, g.GovernanceSubject)
	}
	p := previewFor(t, s, fresh, domain.DecisionKeepAll)
	req := domain.CommitBatchRequest{PreviewToken: p.PreviewToken, IdempotencyKey: runtimeKey0}
	v, e := s.CommitBatchDecisions(ctx, req)
	if e != nil || v.Counts.Saved != 2 {
		t.Fatal(v.Counts, e)
	}
	plans, e := s.ListAllPlans(ctx)
	if e != nil || len(plans) != 1 || plans[0].State != domain.PlanDraft || plans[0].Risk != domain.RiskCritical || !plans[0].Actions[0].Context.Protected {
		t.Fatal("protected plan mutated", e)
	}
	if e = s.CompareAndSwapPlanState(ctx, plan.ID, domain.PlanDraft, domain.PlanExecuting); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CommitBatchDecisions(ctx, req); e == nil {
		t.Fatal("replay bypassed active recovery lock")
	}
	read, e := s.GetBatchDecisionResult(ctx, "key:"+req.IdempotencyKey)
	if e != nil || read.Counts.Saved != 2 {
		t.Fatal("recovery lock blocked historical read")
	}
	locked, e := s.ListExecutingPlans(ctx)
	if e != nil || len(locked) != 1 {
		t.Fatal("decision cleared recovery lock")
	}
	checkSourceBytes(t, before)
}

func TestGovernanceTenThousandGroupPaginationAndFilterCount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if e := s.RegisterStorage(ctx, domain.Storage{ID: "stress", RootPath: "/synthetic", CreatedAt: time.Now()}); e != nil {
		t.Fatal(e)
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	stmt, e := tx.PrepareContext(ctx, `INSERT INTO file_instances(storage_id,path,name,size,mode,mtime,content_sha256,discovered_at,file_status) VALUES('stress',?,?,16,384,?,?,?,'active')`)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 10000; i++ {
		sum := sha256.Sum256([]byte(fmt.Sprintf("group-%05d", i)))
		h := hex.EncodeToString(sum[:])
		for j := 0; j < 2; j++ {
			name := fmt.Sprintf("virtual_%05d_%d", i, j)
			if _, e = stmt.ExecContext(ctx, "/synthetic/"+name, name, "2026-10-13T00:00:00Z", h, "2026-10-13T00:00:00Z"); e != nil {
				t.Fatal(e)
			}
		}
	}
	stmt.Close()
	if tx.Commit() != nil {
		t.Fatal("stress commit")
	}
	durations := []time.Duration{}
	for i := 0; i < 10; i++ {
		start := time.Now()
		p, e := s.ListGovernanceGroups(ctx, domain.GovernanceQuery{StorageID: "stress", Page: i + 1, PageSize: 20, Sort: "path_count"})
		durations = append(durations, time.Since(start))
		if e != nil || p.TotalCount != 10000 || len(p.Items) != 20 {
			t.Fatal("server count/pagination", p.TotalCount, e)
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p, e := s.ListGovernanceGroups(ctx, domain.GovernanceQuery{Search: "virtual_00001", MinSize: 16, MaxSize: 16, DecisionType: "UNREVIEWED"})
	if e != nil || p.TotalCount != 1 || len(p.Items) != 1 || p.Items[0].Risk != "UNKNOWN" {
		t.Fatal("whole dataset filters", p.TotalCount, e)
	}
	t.Logf("synthetic SQL-only 10000 groups/20000 rows, %s/%s, page20 P50=%s P95=%s (not GUI or source scan)", runtime.GOOS, runtime.GOARCH, durations[4], durations[9])
}

func TestLegacyStoreRejectsStaleRevision(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if e := s.SaveGroupDecision(ctx, domain.GroupDecision{GroupID: "synthetic", DecisionType: domain.DecisionKeepAll}); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveGroupDecision(ctx, domain.GroupDecision{GroupID: "synthetic", DecisionType: domain.DecisionDeferred, Revision: 99}); e == nil {
		t.Fatal("stale decision revision accepted")
	}
}

func governanceTestKey(t *testing.T) string {
	t.Helper()
	key, e := opaqueID()
	if e != nil {
		t.Fatal("runtime disposable key creation failed")
	}
	return key
}
