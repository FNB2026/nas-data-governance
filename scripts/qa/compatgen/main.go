// Command compatgen writes a project database using the schema-11 code base
// checked out at a fixed commit, so the compatibility test runs against a file
// that an older build actually produced.
//
// It is copied into a temporary worktree of that commit and executed there; it
// is not meant to run against the current tree, where the schema has moved on.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/FNB2026/nas-data-governance/internal/store"
	_ "modernc.org/sqlite"
)

func fingerprint(path string) (string, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
	if err != nil {
		return "", err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT type,name,tbl_name,COALESCE(sql,'') FROM sqlite_master WHERE substr(name,1,7)<>'sqlite_' ORDER BY type,name`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	h := sha256.New()
	for rows.Next() {
		var kind, name, table, statement string
		if err := rows.Scan(&kind, &name, &table, &statement); err != nil {
			return "", err
		}
		for _, part := range []string{kind, name, table, strings.TrimSpace(statement)} {
			_, _ = io.WriteString(h, part)
			_, _ = h.Write([]byte{0})
		}
	}
	return hex.EncodeToString(h.Sum(nil)), rows.Err()
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: compatgen <database path>")
		os.Exit(2)
	}
	path := os.Args[1]
	ctx := context.Background()
	s, err := store.Open(ctx, path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	if err := s.Init(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		os.Exit(1)
	}
	if err := s.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "close:", err)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed open:", err)
		os.Exit(1)
	}
	defer db.Close()
	// No schema_migrations row: every frozen beta initializer produced the
	// legacy layout without one, and the preflight check depends on that.
	seed := []string{
		`PRAGMA journal_mode=WAL`,
		`INSERT INTO storages (id, root_path, kind, created_at)
		 VALUES ('storage-legacy','/mnt/legacy','local','2026-10-01T00:00:00Z')`,
		`INSERT INTO file_instances (id, storage_id, path, name, size, mode, mtime, device, inode, quick_hash, content_sha256, discovered_at)
		 VALUES (1,'storage-legacy','/mnt/legacy/a.txt','a.txt',64,420,'2026-10-01T00:00:00Z',1,11,'qh-a','sha-a','2026-10-01T00:00:00Z')`,
		`INSERT INTO file_instances (id, storage_id, path, name, size, mode, mtime, device, inode, quick_hash, content_sha256, discovered_at)
		 VALUES (2,'storage-legacy','/mnt/legacy/b.txt','b.txt',64,420,'2026-10-01T00:00:00Z',1,12,'qh-b','sha-b','2026-10-01T00:00:00Z')`,
		`INSERT INTO rules (id, version, priority, enabled, definition_yaml)
		 VALUES ('rule-legacy','1',10,1,'id: rule-legacy')`,
	}
	for _, statement := range seed {
		if _, err := db.Exec(statement); err != nil {
			fmt.Fprintln(os.Stderr, "seed:", err)
			os.Exit(1)
		}
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		fmt.Fprintln(os.Stderr, "checkpoint:", err)
		os.Exit(1)
	}
	db.Close()

	layout, err := fingerprint(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fingerprint:", err)
		os.Exit(1)
	}
	fmt.Printf(`{"path":%q,"layout":%q}`+"\n", path, layout)
}
