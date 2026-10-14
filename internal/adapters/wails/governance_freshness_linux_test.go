//go:build linux

package wails

import (
	"context"
	"crypto/sha256"
	"os"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/FNB2026/nas-data-governance/internal/app"
	"github.com/FNB2026/nas-data-governance/internal/domain"
)

func TestOpenat2BoundaryErrorsBlockGovernancePreviewWithoutSideEffects(t *testing.T) {
	for _, injectedErr := range []error{unix.ENOSYS, unix.EXDEV} {
		t.Run(injectedErr.Error(), func(t *testing.T) {
			a, before := governanceAPIWithSource(t)
			ctx := context.Background()
			page, err := a.ListGovernanceGroups(domain.GovernanceQuery{PageSize: 10})
			if err != nil || len(page.Items) == 0 {
				t.Fatal("synthetic groups unavailable")
			}
			called := false
			service := app.NewGovernanceDecisionService(a.store, true, governanceSourceVerifier{
				api: a,
				read: func(context.Context, string, domain.FileInstance) (string, error) {
					called = true
					return "", injectedErr
				},
			})
			preview, err := service.Preview(ctx, domain.PreviewBatchRequest{
				Items: []domain.GovernanceSubject{page.Items[0].GovernanceSubject}, DecisionType: domain.DecisionKeepAll,
			})
			if err != nil || !called || preview.PreviewToken != "" || len(preview.Items) != 1 || preview.Items[0].Eligibility != "BLOCKED" || preview.Items[0].SourceFreshness != "SOURCE_UNAVAILABLE" || preview.Items[0].ReasonCode != "SOURCE_UNAVAILABLE" {
				t.Fatalf("openat2 %v must block before preview persistence: %+v, %v", injectedErr, preview, err)
			}
			if _, err := a.store.GetGroupDecision(ctx, page.Items[0].GroupID); err == nil {
				t.Fatal("boundary refusal persisted a governance decision")
			}
			summary, err := a.store.ScanSummary(ctx, page.Items[0].StorageID)
			if err != nil || summary.Missing != 0 || summary.Unavailable != 0 {
				t.Fatal("boundary refusal changed indexed source status")
			}
			for path, want := range before {
				data, err := os.ReadFile(path)
				if err != nil || sha256.Sum256(data) != want {
					t.Fatal("boundary refusal changed disposable source bytes")
				}
			}
		})
	}
}
