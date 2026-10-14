package wails

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

// Calls real adapter/service/store/scanner code. TestMain redirects App Support;
// all source/database data are disposable. This is not a native GUI certificate.
func governanceAPIWithSource(t *testing.T) (*API, map[string][32]byte) {
	t.Helper()
	source := t.TempDir()
	before := map[string][32]byte{}
	for i := 0; i < 3; i++ {
		for j := 0; j < 2; j++ {
			path := filepath.Join(source, fmt.Sprintf("disposable_%d_%d", i, j))
			data := []byte(fmt.Sprintf("artificial-pair-content-%d", i))
			if e := os.WriteFile(path, data, 0600); e != nil {
				t.Fatal("fixture write")
			}
			before[path] = sha256.Sum256(data)
		}
	}
	path := filepath.Join(source, "disposable_singleton")
	data := []byte("artificial-unique-content")
	if e := os.WriteFile(path, data, 0600); e != nil {
		t.Fatal("singleton fixture write")
	}
	before[path] = sha256.Sum256(data)
	a := NewAPI()
	// Explicit GitHub/DMG test composition, even under a MAS-tagged test run.
	// Genuine sandbox policy tests use sandboxTestAPI and authorized containers.
	a.sandbox = false
	t.Cleanup(func() { _ = a.CloseProject() })
	if _, e := a.OpenProjectReadWrite(filepath.Join(t.TempDir(), "ui-integration.db")); e != nil {
		t.Fatal(e)
	}
	started, e := a.StartScan(StartScanRequest{Root: source, Workers: 2})
	if e != nil {
		t.Fatal(e)
	}
	progress := waitForJobTerminal(t, a, started.JobID, 30*time.Second)
	if progress.State != "COMPLETED" || progress.Processed != 7 || progress.Failed != 0 {
		t.Fatal("synthetic scan did not naturally complete")
	}
	return a, before
}

func TestGovernanceFreshnessCancellationReleasesAuthorizedLease(t *testing.T) {
	a, native, root := sandboxTestAPI(t)
	for _, name := range []string{"disposable-a", "disposable-b"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("same disposable bytes"), 0600); err != nil {
			t.Fatal("synthetic source setup")
		}
	}
	if _, err := a.PickDirectory(""); err != nil {
		t.Fatal("synthetic source authorization")
	}
	if _, err := a.CreateProjectFromSource(CreateProjectInput{SourcePath: root}); err != nil {
		t.Fatal("synthetic project setup")
	}
	job, err := a.StartScan(StartScanRequest{Root: root, Workers: 1})
	if err != nil {
		t.Fatal("synthetic scan start")
	}
	progress := waitForJobTerminal(t, a, job.JobID, 10*time.Second)
	if progress.State != "COMPLETED" || progress.Processed != 2 || progress.Failed != 0 {
		t.Fatal("synthetic duplicate scan did not complete")
	}
	page, err := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatal("synthetic duplicate subject unavailable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	native.mu.Lock()
	before := native.releases
	native.afterResolve = cancel
	native.mu.Unlock()
	checks := a.VerifyGovernanceSources(ctx, []domain.GovernanceSubject{page.Items[0].GovernanceSubject})
	if got := checks[page.Items[0].GroupID].Status; got != "UNVERIFIED" {
		t.Fatalf("canceled freshness observation should fail closed, got %q", got)
	}
	done := make(chan struct{})
	go func() {
		a.access.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("access manager remained blocked, indicating an unreleased source lease")
	}
	native.mu.Lock()
	defer native.mu.Unlock()
	if native.releases <= before {
		t.Fatal("authorized source lease cleanup did not run")
	}
}

func TestGovernanceFreshnessMemberCeilingCreatesNoPreviewToken(t *testing.T) {
	root := t.TempDir()
	for i := 0; i <= governanceFreshnessMaxFiles; i++ {
		path := filepath.Join(root, fmt.Sprintf("disposable-member-%03d", i))
		if err := os.WriteFile(path, []byte("same bounded fixture"), 0600); err != nil {
			t.Fatal("synthetic source setup")
		}
	}
	a := NewAPI()
	a.sandbox = false // Explicit disposable GitHub/DMG test composition.
	t.Cleanup(func() { _ = a.CloseProject() })
	if _, err := a.OpenProjectReadWrite(filepath.Join(t.TempDir(), "freshness-limit.db")); err != nil {
		t.Fatal("synthetic project setup")
	}
	job, err := a.StartScan(StartScanRequest{Root: root, Workers: 2})
	if err != nil {
		t.Fatal("synthetic scan start")
	}
	progress := waitForJobTerminal(t, a, job.JobID, 30*time.Second)
	if progress.State != "COMPLETED" || progress.Processed != governanceFreshnessMaxFiles+1 || progress.Failed != 0 {
		t.Fatal("synthetic member scan did not complete")
	}
	page, err := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatal("synthetic duplicate group unavailable")
	}
	preview, err := a.PreviewBatchDecisions(domain.PreviewBatchRequest{Items: []domain.GovernanceSubject{page.Items[0].GovernanceSubject}, DecisionType: domain.DecisionKeepAll})
	if err != nil || preview.PreviewToken != "" || len(preview.Items) != 1 || preview.Items[0].ReasonCode != "SOURCE_LIMIT_EXCEEDED" {
		t.Fatal("oversized duplicate group was not blocked before source reads", err)
	}
}

func TestSandboxGovernancePreviewDoesNotAcquireSourceLeaseOrPersistToken(t *testing.T) {
	a, native, root := sandboxTestAPI(t)
	for _, name := range []string{"disposable-a", "disposable-b"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("same disposable bytes"), 0600); err != nil {
			t.Fatal("synthetic source setup")
		}
	}
	if _, err := a.PickDirectory(""); err != nil {
		t.Fatal("synthetic source authorization")
	}
	if _, err := a.CreateProjectFromSource(CreateProjectInput{SourcePath: root}); err != nil {
		t.Fatal("synthetic project setup")
	}
	job, err := a.StartScan(StartScanRequest{Root: root, Workers: 1})
	if err != nil {
		t.Fatal("synthetic scan start")
	}
	progress := waitForJobTerminal(t, a, job.JobID, 10*time.Second)
	if progress.State != "COMPLETED" || progress.Processed != 2 || progress.Failed != 0 {
		t.Fatal("synthetic duplicate scan did not complete")
	}
	page, err := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 10})
	if err != nil || len(page.Items) != 1 {
		t.Fatal("synthetic duplicate group unavailable")
	}
	native.mu.Lock()
	beforeResolve := native.releases
	leaseAttempted := false
	native.afterResolve = func() { leaseAttempted = true }
	native.mu.Unlock()
	preview, err := a.PreviewBatchDecisions(domain.PreviewBatchRequest{Items: []domain.GovernanceSubject{page.Items[0].GovernanceSubject}, DecisionType: domain.DecisionKeepAll})
	if err != nil || preview.PreviewToken != "" || len(preview.Items) != 1 || preview.Items[0].ReasonCode != "RECORDING_UNAVAILABLE" {
		t.Fatal("sandbox preview did not remain read-only", err)
	}
	if leaseAttempted {
		t.Fatal("sandbox recording-disabled preview unnecessarily reacquired a source lease")
	}
	native.mu.Lock()
	afterResolve := native.releases
	native.mu.Unlock()
	if afterResolve != beforeResolve {
		t.Fatal("sandbox preview resolved an additional source bookmark")
	}
}
func TestGovernanceWailsJSONRoundTripReceiptRestartAndSourceBytes(t *testing.T) {
	var nonce [16]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		t.Fatal("disposable nonce creation")
	}
	runtimeKey := hex.EncodeToString(nonce[:])
	a, before := governanceAPIWithSource(t)
	page, e := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 20})
	if e != nil || len(page.Items) != 3 || page.TotalCount != 3 {
		t.Fatal("real group query", e)
	}
	// Decode the actual wire shape, rather than treating mock string revisions as
	// evidence. Go int64 revisions are JSON numbers consumed by the frontend.
	wire, e := json.Marshal(page)
	if e != nil {
		t.Fatal(e)
	}
	var decoded struct {
		Items []domain.GovernanceGroup `json:"items"`
	}
	if json.Unmarshal(wire, &decoded) != nil {
		t.Fatal("wire decode")
	}
	refs := []domain.GovernanceSubject{}
	for _, g := range decoded.Items {
		refs = append(refs, g.GovernanceSubject)
	}
	request := domain.PreviewBatchRequest{Items: refs, DecisionType: domain.DecisionKeepAll, Reason: "synthetic review note"}
	payload, _ := json.Marshal(request)
	var roundTrip domain.PreviewBatchRequest
	if json.Unmarshal(payload, &roundTrip) != nil {
		t.Fatal("numeric subject DTO contract")
	}
	plansBefore, e := a.ListAllPlans()
	if e != nil {
		t.Fatal(e)
	}
	preview, e := a.PreviewBatchDecisions(roundTrip)
	if e != nil || preview.PreviewToken == "" {
		t.Fatal("real preview", e)
	}
	result, e := a.CommitBatchDecisions(domain.CommitBatchRequest{PreviewToken: preview.PreviewToken, IdempotencyKey: runtimeKey})
	if e != nil || result.Status != "COMPLETE" || result.Counts.Saved != 3 || result.Counts.Total != 3 {
		t.Fatal("real commit receipt", e)
	}
	replay, e := a.GetBatchDecisionResult("key:" + runtimeKey)
	if e != nil || !reflect.DeepEqual(result, replay) {
		t.Fatal("read-only key reconciliation", e)
	}
	plansAfter, e := a.ListAllPlans()
	if e != nil || !reflect.DeepEqual(plansBefore, plansAfter) {
		t.Fatal("intent modified execution plans")
	}
	files, e := a.store.ListFiles(context.Background(), "")
	if e != nil || len(files) != 7 {
		t.Fatal("file coverage")
	}
	full := 0
	for _, f := range files {
		if f.ContentSHA256 != "" {
			full++
		}
	}
	if full != 6 {
		t.Fatal("ordinary singleton was unconditionally full-hashed")
	}
	projectPath := a.path
	if e = a.CloseProject(); e != nil {
		t.Fatal(e)
	}
	if _, e = a.OpenProject(projectPath); e != nil {
		t.Fatal(e)
	}
	history, e := a.ListGovernanceBatches(domain.GovernanceBatchQuery{Page: 1, PageSize: 10})
	if e != nil || history.TotalCount != 1 || len(history.Items) != 1 || history.Items[0].BatchID != result.BatchID || history.Items[0].RecordedCount != 3 {
		t.Fatal("actual adapter did not discover a durable batch after readonly restart")
	}
	historyWire, e := json.Marshal(history)
	if e != nil {
		t.Fatal("history wire contract")
	}
	var historyDecoded domain.GovernanceBatchPage
	if json.Unmarshal(historyWire, &historyDecoded) != nil || !reflect.DeepEqual(history, historyDecoded) {
		t.Fatal("history JSON roundtrip")
	}
	replay, e = a.GetBatchDecisionResult(result.BatchID)
	if e != nil || !reflect.DeepEqual(result, replay) {
		t.Fatal("restart durable receipt", e)
	}
	readonlyPreview, e := a.PreviewBatchDecisions(request)
	if e != nil || readonlyPreview.PreviewToken != "" {
		t.Fatal("readonly preview returned a commit token", e)
	}
	if _, e = a.CommitBatchDecisions(domain.CommitBatchRequest{PreviewToken: preview.PreviewToken, IdempotencyKey: runtimeKey}); e == nil {
		t.Fatal("readonly direct API mutation accepted")
	}
	for path, sum := range before {
		data, e := os.ReadFile(path)
		if e != nil || sha256.Sum256(data) != sum {
			t.Fatal("source byte conservation failed")
		}
	}
}

func TestGovernancePreviewRejectsPhysicalSourceChangedWithoutReindex(t *testing.T) {
	a, _ := governanceAPIWithSource(t)
	ctx := context.Background()
	page, err := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 20})
	if err != nil || len(page.Items) == 0 {
		t.Fatal("synthetic duplicate group unavailable")
	}
	ref := page.Items[0].GovernanceSubject
	files, err := a.store.ListFiles(ctx, ref.StorageID)
	if err != nil {
		t.Fatal("synthetic indexed members unavailable")
	}
	var changed *domain.FileInstance
	for i := range files {
		if files[i].ContentSHA256 == ref.SHA256 {
			changed = &files[i]
			break
		}
	}
	if changed == nil {
		t.Fatal("synthetic indexed member unavailable")
	}
	original, err := os.ReadFile(changed.Path)
	if err != nil {
		t.Fatal("synthetic source read")
	}
	if len(original) == 0 {
		t.Fatal("synthetic source unexpectedly empty")
	}
	mutated := append([]byte(nil), original...)
	mutated[0] ^= 0x7f
	if err := os.WriteFile(changed.Path, mutated, 0600); err != nil {
		t.Fatal("synthetic source mutation")
	}
	if err := os.Chtimes(changed.Path, changed.ModifiedAt, changed.ModifiedAt); err != nil {
		t.Fatal("restore synthetic source timestamp")
	}
	preview, err := a.PreviewBatchDecisions(domain.PreviewBatchRequest{Items: []domain.GovernanceSubject{ref}, DecisionType: domain.DecisionKeepAll, Reason: "synthetic stale-source test"})
	if err != nil {
		t.Fatal("preview should return a safe blocked result", err)
	}
	if len(preview.Items) != 1 || preview.Items[0].Eligibility == "ELIGIBLE" || preview.Items[0].ReasonCode != "SOURCE_CHANGED" {
		t.Fatalf("stale physical source must block preview, got eligibility=%q reason=%q", preview.Items[0].Eligibility, preview.Items[0].ReasonCode)
	}
	if preview.PreviewToken != "" {
		t.Fatal("stale source must not create a durable preview token")
	}
	if _, err := a.SaveGroupDecision(SaveDecisionRequest{GroupID: ref.GroupID, DecisionType: string(domain.DecisionKeepAll)}); err == nil {
		t.Fatal("legacy decision binding bypassed physical source freshness")
	}
	// A source freshness check is read-only and must not reconcile scan status.
	rows, err := a.store.ListFiles(ctx, ref.StorageID)
	if err != nil {
		t.Fatal("indexed rows unavailable after preview")
	}
	for _, row := range rows {
		if row.Path == changed.Path && row.ContentSHA256 != ref.SHA256 {
			t.Fatal("preview changed indexed source state")
		}
	}
	summary, err := a.store.ScanSummary(ctx, ref.StorageID)
	if err != nil || summary.Missing != 0 || summary.Unavailable != 0 {
		t.Fatal("freshness check incorrectly reconciled file status")
	}
}

func TestGovernanceCommitRechecksSourceAfterPreview(t *testing.T) {
	a, _ := governanceAPIWithSource(t)
	ctx := context.Background()
	page, err := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 20})
	if err != nil || len(page.Items) == 0 {
		t.Fatal("synthetic duplicate group unavailable")
	}
	ref := page.Items[0].GovernanceSubject
	preview, err := a.PreviewBatchDecisions(domain.PreviewBatchRequest{Items: []domain.GovernanceSubject{ref}, DecisionType: domain.DecisionKeepAll, Reason: "synthetic stale-at-commit test"})
	if err != nil || preview.PreviewToken == "" || preview.Items[0].SourceFreshness != "MATCH" {
		t.Fatal("fresh preview missing", err)
	}
	files, err := a.store.ListFiles(ctx, ref.StorageID)
	if err != nil {
		t.Fatal("synthetic indexed members unavailable")
	}
	var changed *domain.FileInstance
	for i := range files {
		if files[i].ContentSHA256 == ref.SHA256 {
			changed = &files[i]
			break
		}
	}
	if changed == nil {
		t.Fatal("synthetic indexed member unavailable")
	}
	original, err := os.ReadFile(changed.Path)
	if err != nil {
		t.Fatal("synthetic source read")
	}
	mutated := append([]byte(nil), original...)
	mutated[len(mutated)-1] ^= 0x33
	if err := os.WriteFile(changed.Path, mutated, 0600); err != nil {
		t.Fatal("synthetic source mutation")
	}
	if err := os.Chtimes(changed.Path, changed.ModifiedAt, changed.ModifiedAt); err != nil {
		t.Fatal("restore synthetic source timestamp")
	}
	result, err := a.CommitBatchDecisions(domain.CommitBatchRequest{PreviewToken: preview.PreviewToken, IdempotencyKey: historyTestKey(t)})
	if err != nil || result.Status != "COMPLETE" || result.Counts.Blocked != 1 || result.Counts.Saved != 0 || result.Items[0].ReasonCode != "SOURCE_CHANGED" {
		t.Fatal("commit must durably block changed source", result, err)
	}
	if _, err := a.store.GetGroupDecision(ctx, ref.GroupID); err == nil {
		t.Fatal("stale source recorded a decision")
	}
	summary, err := a.store.ScanSummary(ctx, ref.StorageID)
	if err != nil || summary.Missing != 0 || summary.Unavailable != 0 {
		t.Fatal("stale source was incorrectly reconciled missing/unavailable")
	}
}

func TestGovernanceHistoryReadsUnderRecoveryLockWithoutClearingIt(t *testing.T) {
	a, before := governanceAPIWithSource(t)
	ctx := context.Background()
	page, e := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 20})
	if e != nil || len(page.Items) == 0 {
		t.Fatal("synthetic subjects")
	}
	preview, e := a.PreviewBatchDecisions(domain.PreviewBatchRequest{Items: []domain.GovernanceSubject{page.Items[0].GovernanceSubject}, DecisionType: domain.DecisionKeepAll})
	if e != nil {
		t.Fatal("fixture preview")
	}
	r, e := a.CommitBatchDecisions(domain.CommitBatchRequest{PreviewToken: preview.PreviewToken, IdempotencyKey: historyTestKey(t)})
	if e != nil || r.Status != "COMPLETE" {
		t.Fatal("fixture commit")
	}
	task := domain.OperationTask{ID: "synthetic-history-locked-task", RootPath: t.TempDir(), State: "executing", CreatedAt: time.Now()}
	if a.store.CreateTask(ctx, task) != nil || a.store.SavePlans(ctx, task.ID, []domain.OperationPlan{{ID: "synthetic-history-locked-plan", TaskID: task.ID, State: domain.PlanExecuting}}) != nil {
		t.Fatal("recovery lock fixture")
	}
	lockBefore, e := a.CheckRecoveryLock()
	if e != nil || !lockBefore.LockActive {
		t.Fatal("actual recovery lock not established")
	}
	a.sandbox = true // Adapter policy only, not native Sandbox runtime evidence.
	history, e := a.ListGovernanceBatches(domain.GovernanceBatchQuery{PageSize: 10})
	if e != nil || len(history.Items) != 1 || history.Items[0].BatchID != r.BatchID {
		t.Fatal("locked/channel readonly history unavailable")
	}
	if _, e = a.GetBatchDecisionResult(history.Items[0].BatchID); e != nil {
		t.Fatal("locked receipt query")
	}
	lockAfter, e := a.CheckRecoveryLock()
	if e != nil || !reflect.DeepEqual(lockBefore, lockAfter) {
		t.Fatal("history changed recovery lock")
	}
	plans, e := a.store.ListAllPlans(ctx)
	if e != nil || len(plans) != 1 || plans[0].State != domain.PlanExecuting {
		t.Fatal("history changed plan or approval")
	}
	for path, want := range before {
		bytes, e := os.ReadFile(path)
		if e != nil || sha256.Sum256(bytes) != want {
			t.Fatal("history changed source")
		}
	}
}

func historyTestKey(t *testing.T) string {
	t.Helper()
	var bytes [16]byte
	if _, e := rand.Read(bytes[:]); e != nil {
		t.Fatal("synthetic key")
	}
	return hex.EncodeToString(bytes[:])
}
