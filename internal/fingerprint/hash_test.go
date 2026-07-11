package fingerprint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashesAreStable(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sample")
	if err := os.WriteFile(p, []byte("same content"), 0o600); err != nil {
		t.Fatal(err)
	}
	q, err := Quick(p, 12)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Full(p)
	if err != nil {
		t.Fatal(err)
	}
	if q == "" || f == "" || q != f {
		t.Fatalf("unexpected hashes quick=%q full=%q", q, f)
	}
}
