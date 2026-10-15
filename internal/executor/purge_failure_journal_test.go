package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/domain"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

type purgeFailureStore struct {
	*store.SQLiteStore
	fault         string
	stage         string
	rollbackCalls int
	cancel        context.CancelFunc
}

func (s *purgeFailureStore) BeginPurge(ctx context.Context, p domain.PurgePlan, q domain.QuarantineItem, stage string, at time.Time) error {
	if err := s.SQLiteStore.BeginPurge(ctx, p, q, stage, at); err != nil {
		return err
	}
	s.stage = stage
	switch s.fault {
	case "staging_directory_failed":
		return os.WriteFile(filepath.Dir(stage), []byte("unknown blocker"), 0600)
	case "stage_move_failed":
		return os.Rename(q.QuarantinePath, q.QuarantinePath+".preserved")
	case "cancel_before_stage":
		s.cancel()
	}
	return nil
}
func (s *purgeFailureStore) MarkPurgeStaged(ctx context.Context, id string) error {
	if s.fault == "journal_staged_failed" {
		return errors.New("injected staged failure")
	}
	if err := s.SQLiteStore.MarkPurgeStaged(ctx, id); err != nil {
		return err
	}
	if s.fault == "staged_verification_failed" {
		return os.WriteFile(s.stage, []byte("changed staged object"), 0600)
	}
	return nil
}
func (s *purgeFailureStore) MarkPurgeCommitPending(ctx context.Context, id string) error {
	if s.fault == "journal_commit_pending_failed" {
		return errors.New("injected commit pending failure")
	}
	if err := s.SQLiteStore.MarkPurgeCommitPending(ctx, id); err != nil {
		return err
	}
	if s.fault == "cancel_before_unlink" {
		s.cancel()
	}
	if s.fault == "purge_remove_failed" {
		if err := os.Rename(s.stage, s.stage+".preserved"); err != nil {
			return err
		}
		if err := os.Mkdir(s.stage, 0700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(s.stage, "unknown"), []byte("do not remove"), 0600)
	}
	return nil
}
func (s *purgeFailureStore) MarkPurgeCommitted(ctx context.Context, p, q string, at time.Time) error {
	if s.fault == "journal_commit_failed" {
		return errors.New("injected commit failure")
	}
	return s.SQLiteStore.MarkPurgeCommitted(ctx, p, q, at)
}
func (s *purgeFailureStore) MarkPurgeRolledBack(ctx context.Context, p, q string, at time.Time) error {
	s.rollbackCalls++
	return s.SQLiteStore.MarkPurgeRolledBack(ctx, p, q, at)
}

func TestPurgeFailuresNeverFinalizeUnprovenRollback(t *testing.T) {
	cases := []struct{ fault, journal, retained string }{
		{"staging_directory_failed", "pending", "original"},
		{"stage_move_failed", "pending", "external"},
		{"journal_staged_failed", "pending", "stage"},
		{"staged_verification_failed", "staged", "changed_stage"},
		{"journal_commit_pending_failed", "staged", "stage"},
		{"purge_remove_failed", "commit_pending", "preserved_stage"},
		{"journal_commit_failed", "commit_pending", "unlinked"},
		{"cancel_before_stage", "pending", "original"},
		{"cancel_before_unlink", "commit_pending", "stage"},
	}
	for _, c := range cases {
		t.Run(c.fault, func(t *testing.T) {
			st, root, item, plan, now := seedPurgeExecution(t)
			fake := &purgeFailureStore{SQLiteStore: st, fault: c.fault}
			exec, err := NewPurgeExecutor(root, fake)
			if err != nil {
				t.Fatal(err)
			}
			exec.now = func() time.Time { return now }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fake.cancel = cancel
			r := exec.ExecutePurge(ctx, &plan, &item)
			wantError := c.fault
			if c.fault == "cancel_before_stage" || c.fault == "cancel_before_unlink" {
				wantError = "cancelled"
			}
			if r.Err == nil || r.Status != StepFailed || r.ErrorType != wantError || r.FinalState == domain.PurgeRolledBack || r.FinalState == domain.PurgeCommitted || fake.rollbackCalls != 0 {
				t.Fatalf("unproven finalization: %+v rollback=%d", r, fake.rollbackCalls)
			}
			entries, err := st.ListRecoverablePurges(context.Background())
			if err != nil || len(entries) != 1 || entries[0].Status != c.journal {
				t.Fatalf("lost unresolved journal: %+v %v", entries, err)
			}
			p, err := st.GetPurgePlan(context.Background(), plan.ID)
			if err != nil || p.State != domain.PurgeApproved || p.PurgedAt != nil {
				t.Fatal("durable plan finalized")
			}
			q, err := st.GetQuarantineItem(context.Background(), item.ID)
			if err != nil || q.Status != domain.QuarantinePurgeEligible || q.PurgedAt != nil {
				t.Fatal("durable item finalized")
			}
			path := item.QuarantinePath
			want := "permanent test"
			switch c.retained {
			case "external":
				path += ".preserved"
			case "stage":
				path = fake.stage
			case "changed_stage":
				path = fake.stage
				want = "changed staged object"
			case "preserved_stage":
				path = fake.stage + ".preserved"
			case "unlinked":
				for _, p := range []string{path, fake.stage} {
					if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("unlink fixture invalid")
					}
				}
			}
			if c.retained != "unlinked" {
				b, err := os.ReadFile(path)
				if err != nil || string(b) != want {
					t.Fatal("unresolved bytes lost")
				}
			}
			// A subsequent real store-backed recovery must not convert these path
			// shapes into an invented rollback or a successful irreversible commit.
			recovery, _ := NewPurgeExecutor(root, st)
			rr := recovery.RecoverPurges(context.Background())
			if len(rr) != 1 || rr[0].ErrorType != "manual_reconciliation_required" || rr[0].Err == nil {
				t.Fatalf("failure became a recovery PASS: %+v", rr)
			}
			after, err := st.ListRecoverablePurges(context.Background())
			if err != nil || len(after) != 1 || after[0].Status != c.journal {
				t.Fatal("recovery released unresolved journal")
			}
		})
	}
}

type purgeReadFailureStore struct{ *stagedFailStore }

func (*purgeReadFailureStore) ListRecoverablePurges(context.Context) ([]domain.PurgeJournalEntry, error) {
	return nil, errors.New("private-journal-read-canary")
}

func TestPurgeRecoveryReadFailureDoesNotLeakOrSucceed(t *testing.T) {
	exec, err := NewPurgeExecutor(t.TempDir(), &purgeReadFailureStore{&stagedFailStore{}})
	if err != nil {
		t.Fatal(err)
	}
	r := exec.RecoverPurges(context.Background())
	if len(r) != 1 || r[0].Status != StepFailed || r[0].ErrorType != "journal_read_failed" || r[0].Err != errActionFailed || r[0].FinalState != "" {
		t.Fatalf("unsafe read failure: %+v", r)
	}
}
