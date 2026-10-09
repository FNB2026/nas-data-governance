package app

import (
	"context"
	"fmt"

	"github.com/FNB2026/nas-data-governance/internal/store"
)

// All lifecycle executors use the same project owner and recovery evidence.
func requireNoRecovery(ctx context.Context, st *store.SQLiteStore) error {
	ids, err := st.ListExecutingPlans(ctx)
	if err != nil || len(ids) > 0 {
		return fmt.Errorf("app: source recovery lock active or unavailable")
	}
	restores, err := st.ListPendingRestores(ctx)
	if err != nil || len(restores) > 0 {
		return fmt.Errorf("app: restore recovery lock active or unavailable")
	}
	purges, err := st.ListRecoverablePurges(ctx)
	if err != nil || len(purges) > 0 {
		return fmt.Errorf("app: purge recovery lock active or unavailable")
	}
	return nil
}
