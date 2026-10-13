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
