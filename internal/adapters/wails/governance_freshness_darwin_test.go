//go:build darwin

package wails

import (
	"context"
	"os"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestGovernanceFreshnessDarwinFailsClosedWithoutStore(t *testing.T) {
	a := NewAPI()
	checks := a.verifyGovernanceSources(context.Background(), []domain.GovernanceSubject{{GroupID: "synthetic-group"}})
	check, ok := checks["synthetic-group"]
	if !ok || check.Status == "MATCH" || check.CheckedAt == "" {
		t.Fatalf("missing catalog must never yield a fresh observation: %+v", checks)
	}
}

func TestGovernancePreviewDarwinIssuesTokenAfterMountProof(t *testing.T) {
	a, _ := governanceAPIWithSource(t)
	page, err := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 10})
	if err != nil || len(page.Items) == 0 {
		t.Fatal("synthetic governance groups unavailable")
	}
	preview, err := a.PreviewBatchDecisions(domain.PreviewBatchRequest{
		Items: []domain.GovernanceSubject{page.Items[0].GovernanceSubject}, DecisionType: domain.DecisionKeepAll,
	})
	if err != nil || preview.PreviewToken == "" || len(preview.Items) != 1 {
		t.Fatalf("Darwin mount proof should allow a record-only preview: %+v, %v", preview, err)
	}
	if preview.Items[0].SourceFreshness != "MATCH" || preview.Items[0].Eligibility != "ELIGIBLE" {
		t.Fatalf("unexpected preview eligibility: %+v", preview.Items[0])
	}
}

func TestGovernancePreviewDarwinRejectsChangedSource(t *testing.T) {
	a, before := governanceAPIWithSource(t)
	page, err := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 10})
	if err != nil || len(page.Items) == 0 {
		t.Fatal("synthetic groups unavailable")
	}
	var changed string
	for path := range before {
		changed = path
		break
	}
	if changed == "" {
		t.Fatal("synthetic source path unavailable")
	}
	if err := os.WriteFile(changed, []byte("changed after scan"), 0600); err != nil {
		t.Fatal(err)
	}
	refs := make([]domain.GovernanceSubject, len(page.Items))
	for i, item := range page.Items {
		refs[i] = item.GovernanceSubject
	}
	preview, err := a.PreviewBatchDecisions(domain.PreviewBatchRequest{
		Items: refs, DecisionType: domain.DecisionKeepAll,
	})
	if err != nil {
		t.Fatalf("preview returned an unexpected error: %v", err)
	}
	for _, item := range preview.Items {
		if item.Eligibility != "ELIGIBLE" || item.SourceFreshness != "MATCH" {
			return
		}
	}
	t.Fatalf("changed source must be rejected: %+v", preview.Items)
}
