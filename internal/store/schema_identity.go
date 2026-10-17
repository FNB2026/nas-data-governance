package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
)

// Frozen sqlite_master identities. See UX2 backend evidence for release-source
// initialization provenance and the deterministic fixture that detects drift.
const (
	emptySchemaLayout        = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	legacySchema11Layout     = "e177d2a1ec16d33cd96eed49a70082c8b25a10c5b6b72ca7e4c7e5138f811af5"
	governanceSchema12Layout = "2cdb8fa859ae5f680af9cb6ed2455422946e317381ab93a4118077d3e34ccf84"
	// authorizationAudit13Layout adds the authorization_audit table and its
	// two indexes (migration 013, R4 Task C). No schema_migrations row is
	// written for it — the marker tripwire stays at 12, and the layout
	// fingerprint itself distinguishes the increment.
	authorizationAudit13Layout = "552e600f555aa7ffa47ef5ae52417109ea8071656e6b13ff3578b6bb3dc984c2"
)

// Fingerprint only the exact stored layout. It is not a provenance certificate
// and deliberately does not infer identity from a common version-table name.
func schemaLayoutFingerprint(ctx context.Context, r governanceReader) (string, error) {
	rows, e := r.QueryContext(ctx, `SELECT type,name,tbl_name,COALESCE(sql,'') FROM sqlite_master WHERE substr(name,1,7)<>'sqlite_' ORDER BY type,name`)
	if e != nil {
		return "", ErrGovernanceRejected
	}
	defer rows.Close()
	h := sha256.New()
	count, bytes := 0, 0
	for rows.Next() {
		var kind, name, table, statement string
		if rows.Scan(&kind, &name, &table, &statement) != nil {
			return "", ErrGovernanceRejected
		}
		count++
		bytes += len(kind) + len(name) + len(table) + len(statement)
		if count > 512 || bytes > 1<<20 {
			return "", ErrGovernanceRejected
		}
		for _, part := range []string{kind, name, table, strings.TrimSpace(statement)} {
			_, _ = io.WriteString(h, part)
			_, _ = h.Write([]byte{0})
		}
	}
	if rows.Err() != nil || rows.Close() != nil {
		return "", ErrGovernanceRejected
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
