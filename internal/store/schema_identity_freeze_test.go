package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSchemaLayoutFrozenIdentity(t *testing.T) {
	ctx := context.Background()
	current := newTestStore(t)
	h, e := schemaLayoutFingerprint(ctx, current.db)
	if e != nil || h != authorizationAudit13Layout {
		t.Fatal("current fingerprint drift")
	}
	t.Log("schema13", h)
	db, e := sql.Open("sqlite", filepath.Join(t.TempDir(), "frozen-legacy.db"))
	if e != nil {
		t.Fatal("frozen fixture")
	}
	defer db.Close()
	if createSupportedLegacySchema11(ctx, db) != nil {
		t.Fatal("frozen legacy layout")
	}
	h, e = schemaLayoutFingerprint(ctx, db)
	if e != nil || h != "e177d2a1ec16d33cd96eed49a70082c8b25a10c5b6b72ca7e4c7e5138f811af5" {
		t.Fatal("legacy identity drift")
	}
	t.Log("legacy11", h)
}
