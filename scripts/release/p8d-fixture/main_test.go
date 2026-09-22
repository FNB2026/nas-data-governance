package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

func TestCreateK5CreatesExpiredManagedItem(t *testing.T) {
	m, err := createK5(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(m.FixtureRoot) })
	if m.RetentionHours != 24 || m.QuarantineRoot == "" || m.QuarantineID == "" {
		t.Fatalf("unexpected K5 manifest: %#v", m)
	}
	if _, err := os.Stat(m.QuarantineRoot + "/eligible-for-purge.txt"); err != nil {
		t.Fatalf("fixture quarantine file missing: %v", err)
	}
	st, err := store.OpenReadOnly(context.Background(), m.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close() //nolint:errcheck
	items, err := st.ListQuarantineItems(context.Background(), domain.QuarantineActive)
	if err != nil || len(items) != 1 {
		t.Fatalf("active item count=%d err=%v", len(items), err)
	}
	if items[0].RetainUntil.After(time.Now().UTC()) || items[0].QuarantinedAt.After(time.Now().UTC().Add(-24*time.Hour)) {
		t.Fatalf("K5 item does not model a completed 24-hour retention: %#v", items[0])
	}
}

func TestCreateK6CreatesNoWriteRecoveryLock(t *testing.T) {
	m, err := createK6(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(m.FixtureRoot) })
	st, err := store.OpenReadOnly(context.Background(), m.DatabasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close() //nolint:errcheck
	plans, err := st.ListExecutingPlans(context.Background())
	if err != nil || len(plans) != 1 || plans[0] != m.PlanID {
		t.Fatalf("executing plans=%v err=%v", plans, err)
	}
	done, err := st.ListJournalDone(context.Background(), m.PlanID)
	if err != nil || len(done) != 0 {
		t.Fatalf("done journal entries=%v err=%v", done, err)
	}
	if _, err := os.Stat(m.SourceRoot + "/recovery-lock-source.txt"); err != nil {
		t.Fatalf("source file must remain untouched: %v", err)
	}
}
