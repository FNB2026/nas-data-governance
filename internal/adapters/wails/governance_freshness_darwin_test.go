//go:build darwin

package wails

import (
	"context"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestGovernanceFreshnessDarwinIsUnverifiedBeforeStoreOrLease(t *testing.T) {
	a := NewAPI()
	checks := a.verifyGovernanceSources(context.Background(), []domain.GovernanceSubject{{GroupID: "synthetic-group"}})
	check, ok := checks["synthetic-group"]
	if !ok || check.Status != "UNVERIFIED" || check.CheckedAt == "" {
		t.Fatalf("Darwin must report an explicit unverified observation before any store or source access: %+v", checks)
	}
}

func TestGovernancePreviewDarwinDoesNotIssueTokenWithoutMountProof(t *testing.T) {
	a, _ := governanceAPIWithSource(t)
	page, err := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 10})
	if err != nil || len(page.Items) == 0 {
		t.Fatal("synthetic governance groups unavailable")
	}
	preview, err := a.PreviewBatchDecisions(domain.PreviewBatchRequest{
		Items: []domain.GovernanceSubject{page.Items[0].GovernanceSubject}, DecisionType: domain.DecisionKeepAll,
	})
	if err != nil || preview.PreviewToken != "" || len(preview.Items) != 1 || preview.Items[0].SourceFreshness != "UNVERIFIED" || preview.Items[0].ReasonCode != "SOURCE_PLATFORM_PROOF_UNAVAILABLE" {
		t.Fatalf("Darwin must block a new record-only preview without opening source members: %+v, %v", preview, err)
	}
}

func TestGovernancePreviewDarwinExplainsPlatformProofRefusal(t *testing.T) {
	a, _ := governanceAPIWithSource(t)
	page, err := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 10})
	if err != nil || len(page.Items) == 0 {
		t.Fatal("synthetic groups unavailable")
	}
	for attempt := 0; attempt < 2; attempt++ {
		preview, err := a.PreviewBatchDecisions(domain.PreviewBatchRequest{
			Items: []domain.GovernanceSubject{page.Items[0].GovernanceSubject}, DecisionType: domain.DecisionKeepAll,
		})
		if err != nil || len(preview.Items) != 1 || preview.Items[0].ReasonCode != "SOURCE_PLATFORM_PROOF_UNAVAILABLE" {
			t.Fatalf("platform refusal needs its actual cause, not a guessed network or permission error: %+v, %v", preview, err)
		}
		if preview.PreviewToken != "" || preview.Items[0].Eligibility != "BLOCKED" || preview.Items[0].SourceFreshness != "UNVERIFIED" {
			t.Fatal("a fresh explicit revalidation must not bypass the platform boundary")
		}
	}
}
