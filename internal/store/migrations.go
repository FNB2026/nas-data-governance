package store

import (
	"context"
	"fmt"

	"nas-data-governance/schemas"
)

// Init applies migrations inside a single connection. SQLite's "CREATE
// TABLE" without IF NOT EXISTS would error on re-run; the schema file
// already uses IF NOT EXISTS / PRAGMA so this is safe to call repeatedly.
func (s *SQLiteStore) Init(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("store: acquire conn: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("store: enable foreign_keys: %w", err)
	}
	for i, m := range schemas.All() {
		if _, err := conn.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("store: migration %d failed: %w", i+1, err)
		}
	}
	return nil
}
