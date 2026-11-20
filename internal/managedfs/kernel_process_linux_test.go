//go:build linux

package managedfs

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// Driven only by the disposable namespace harness, with artificial private
// objects. This path neither creates a fixture nor bypasses OpenReader.
func TestManagedReaderKernelProcess(t *testing.T) {
	path := os.Getenv("NDG_MANAGED_READER_PROBE")
	if path == "" {
		t.Skip("disposable kernel driver required")
	}
	var input struct {
		Path     string      `json:"path"`
		Proof    ObjectProof `json:"proof"`
		Expected string      `json:"expected"`
	}
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &input) != nil {
		t.Fatal("private probe input unavailable")
	}
	r, err := OpenReader(input.Path, input.Proof.Root)
	if input.Expected == "constructor_refused" {
		if !errors.Is(err, ErrRejected) || r != nil {
			t.Fatal("mounted/linked constructor admitted")
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"state": "PASS", "category": "constructor_refused"})
		return
	}
	if err != nil {
		t.Fatal("normal kernel fixture rejected", err)
	}
	defer r.Close()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"state": "READY"})
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil || line != "inspect\n" {
		t.Fatal("probe synchronization failed")
	}
	err = r.Inspect(context.Background(), input.Proof)
	if input.Expected == "match" && err != nil {
		t.Fatal("normal private object did not match", err)
	}
	if input.Expected == "refused" && !errors.Is(err, ErrRejected) {
		t.Fatal("changed mount/path admitted", err)
	}
	if input.Expected != "match" && input.Expected != "refused" {
		t.Fatal("unknown probe expectation")
	}
	if r.Close() != nil {
		t.Fatal("kernel session closure failed")
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"state": "PASS", "category": input.Expected})
}
