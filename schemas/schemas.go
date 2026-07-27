// Package schemas holds the canonical SQL migrations and example rule YAML.
// Migrations live as .sql files in this directory and are embedded so the
// store package can apply them without runtime file I/O.
package schemas

import _ "embed"

//go:embed 001_initial.sql
var Schema001 string

//go:embed 002_rules_learning.sql
var Schema002 string

//go:embed 003_execution_journal.sql
var Schema003 string

//go:embed 004_scan_checkpoints.sql
var Schema004 string

//go:embed 005_deletion_lifecycle.sql
var Schema005 string

//go:embed 006_desktop_jobs.sql
var Schema006 string

//go:embed 007_directory_context_query_fields.sql
var Schema007 string

//go:embed 008_governance_decisions.sql
var Schema008 string

// All returns every migration in order. Append here when adding a new file.
func All() []string {
	return []string{Schema001, Schema002, Schema003, Schema004, Schema005, Schema006, Schema007, Schema008}
}
