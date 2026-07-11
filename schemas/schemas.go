// Package schemas holds the canonical SQL migrations and example rule YAML.
// Migrations live as .sql files in this directory and are embedded so the
// store package can apply them without runtime file I/O.
package schemas

import _ "embed"

//go:embed 001_initial.sql
var Schema001 string

//go:embed 002_rules_learning.sql
var Schema002 string

// All returns every migration in order. Append here when adding a new file.
func All() []string {
	return []string{Schema001, Schema002}
}
