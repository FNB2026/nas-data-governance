// Package app provides application services that sit between the CLI/desktop
// presentation layer and the core domain packages. Each service encapsulates
// a use case (scan, query duplicates, review, plan, execute, recover) so that
// both the CLI and the future Wails desktop binding can share the same logic
// without duplicating orchestration code.
//
// Per ADR-0006: services do not print to stdout/stderr, do not parse command-
// line flags, and do not depend on any UI framework. They accept dependencies
// via constructors and return structured results. The CLI layer is responsible
// for flag parsing, file I/O, and output formatting.
package app
