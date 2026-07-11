package executor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPathForFlatStructure(t *testing.T) {
	c := QuarantineConfig{Root: "/var/quarantine", Structure: QuarantineFlat, SourceRoots: []string{"/data"}}
	got := c.PathFor("/data/temp/report.pdf", time.Now())
	want := "/var/quarantine/report.pdf"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestPathForDatedStructure(t *testing.T) {
	c := QuarantineConfig{Root: "/var/quarantine", Structure: QuarantineDated, SourceRoots: []string{"/data"}}
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	got := c.PathFor("/data/temp/report.pdf", now)
	want := "/var/quarantine/2026-07/report.pdf"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveCollisionReturnsNominalWhenAbsent(t *testing.T) {
	nominal := filepath.Join(t.TempDir(), "absent.txt")
	got, err := ResolveCollision(nominal)
	if err != nil {
		t.Fatal(err)
	}
	if got != nominal {
		t.Fatalf("got %q, want %q", got, nominal)
	}
}

func TestResolveCollisionAppendsSuffixWhenPresent(t *testing.T) {
	dir := t.TempDir()
	nominal := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(nominal, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveCollision(nominal)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "report.1.pdf")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveCollisionIncrementsSuffix(t *testing.T) {
	dir := t.TempDir()
	nominal := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(nominal, []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.1.pdf"), []byte("2"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveCollision(nominal)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "report.2.pdf")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestValidateRejectsEmptyRoot(t *testing.T) {
	if err := (QuarantineConfig{Root: "", Structure: QuarantineFlat, SourceRoots: []string{"/data"}}).Validate(); err == nil {
		t.Fatal("expected error for empty root")
	}
}

func TestValidateRejectsRelativeRoot(t *testing.T) {
	if err := (QuarantineConfig{Root: "relative/path", Structure: QuarantineFlat, SourceRoots: []string{"/data"}}).Validate(); err == nil {
		t.Fatal("expected error for relative root")
	}
}

func TestValidateRejectsUnknownStructure(t *testing.T) {
	if err := (QuarantineConfig{Root: "/var/q", Structure: "weird", SourceRoots: []string{"/data"}}).Validate(); err == nil {
		t.Fatal("expected error for unknown structure")
	}
}

func TestValidateAcceptsKnownStructures(t *testing.T) {
	for _, s := range []QuarantineStructure{QuarantineFlat, QuarantineDated} {
		c := QuarantineConfig{Root: "/var/q", Structure: s, SourceRoots: []string{"/data"}}
		if err := c.Validate(); err != nil {
			t.Fatalf("structure %q: %v", s, err)
		}
	}
}
