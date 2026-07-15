package pathdiag

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
)

func TestBuildClassifiesExistingMissingAndOutsidePaths(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "existing.txt")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Build(root, []string{existing, filepath.Join(root, "missing.txt"), filepath.Join(root, "missing-parent", "x")}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.ExecutionAuthorized || report.Summary.Candidates != 3 || report.Summary.ExactNowExists != 1 || report.Summary.NoCurrentMatch != 1 || report.Summary.ParentMissing != 1 {
		t.Fatalf("unexpected summary: %#v", report.Summary)
	}
}

func TestBuildRejectsOutsideRootAndSymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	report, err := Build(root, []string{filepath.Join(outside, "x"), filepath.Join(link, "x")}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.SafetyRejected != 2 {
		t.Fatalf("expected two safety rejections: %#v", report.Summary)
	}
}

func TestNormalizedNamesMatch(t *testing.T) {
	composed := "é.wav"
	decomposed := norm.NFD.String(composed)
	if composed == decomposed || !normalizedNamesMatch(composed, decomposed) {
		t.Fatal("expected NFC and NFD spellings to normalize equally")
	}
}

func TestHasDecomposedForm(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"plain-ascii.txt", false},
		{"café.txt", false},         // NFC precomposed é
		{"cafe\u0301.txt", true},    // NFD decomposed e + combining acute
		{"中文.txt", false},           // CJK is identical in NFC/NFD
		{"Raphaël.txt", false},      // NFC precomposed ë
		{"Raphae\u0308l.txt", true}, // NFD decomposed e + combining diaeresis
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasDecomposedForm(c.name); got != c.want {
				t.Fatalf("hasDecomposedForm(%q)=%v want %v", c.name, got, c.want)
			}
		})
	}
}

// TestBuildClassifiesListableNotOpenable simulates the observed state where a
// directory entry is visible (Lstat succeeds) but Open fails. We
// cannot reliably reproduce ENOENT-on-Open after Lstat-success on a local
// filesystem, so we use a Unix permission denial on a regular file: chmod
// the file to 0000 and run as non-root. Lstat succeeds, Open returns EACCES.
// This still exercises the new (lstatOK && !openable) branch and confirms
// the classification is listable_not_openable rather than exact_now_exists.
func TestBuildClassifiesListableNotOpenable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based Open failure not applicable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root, permission test not applicable")
	}
	root := t.TempDir()
	target := filepath.Join(root, "locked.txt")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Revoke read permission: Lstat still succeeds, Open returns EACCES.
	if err := os.Chmod(target, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(target, 0o600) // restore so cleanup works

	report, err := Build(root, []string{target}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Candidates != 1 || report.Summary.ListableNotOpenable != 1 {
		t.Fatalf("expected 1 listable_not_openable, got %#v", report.Summary)
	}
	if len(report.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(report.Items))
	}
	it := report.Items[0]
	if it.Classification != "listable_not_openable" {
		t.Fatalf("classification=%q want listable_not_openable", it.Classification)
	}
	if it.Openable {
		t.Fatalf("Openable should be false for permission-denied file")
	}
	// plain ASCII filename should not set NameHasDecomposedForm
	if it.NameHasDecomposedForm {
		t.Fatalf("NameHasDecomposedForm should be false for ASCII filename")
	}
}

// TestBuildListableNotOpenableWithDecomposedName verifies that the NFD
// detection flag is set when a listable-but-not-openable file has a
// decomposed Latin diacritic filename. Uses permission denial to trigger
// the Open failure path.
func TestBuildListableNotOpenableWithDecomposedName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based Open failure not applicable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root, permission test not applicable")
	}
	root := t.TempDir()
	// NFD spelling: "Raphae\u0308l.txt" (e + combining diaeresis)
	name := "Raphae\u0308l.txt"
	target := filepath.Join(root, name)
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(target, 0o600)

	report, err := Build(root, []string{target}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.ListableNotOpenable != 1 {
		t.Fatalf("expected 1 listable_not_openable, got %#v", report.Summary)
	}
	it := report.Items[0]
	if !it.NameHasDecomposedForm {
		t.Fatalf("NameHasDecomposedForm should be true for NFD filename")
	}
}
