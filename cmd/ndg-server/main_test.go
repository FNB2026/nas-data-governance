package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCLIPrivateArguments(t *testing.T) {
	if os.Getenv("NDG_CLI_TEST_CHILD") == "1" {
		os.Args = []string{"ndg-server", "-unknown-private_path_anchor=/synthetic_private_anchor"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCLIPrivateArguments$")
	cmd.Env = append(os.Environ(), "NDG_CLI_TEST_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err == nil || strings.Contains(string(output), "anchor") || string(output) != "ndg-server: startup or shutdown rejected; private details omitted\n" {
		t.Fatalf("CLI rejection did not preserve private output boundary")
	}
	for _, args := range [][]string{nil, {"/synthetic_private_anchor"}, {"-config"}, {"-help"}} {
		if run(args) == nil {
			t.Fatal("invalid arguments accepted")
		}
	}
}
