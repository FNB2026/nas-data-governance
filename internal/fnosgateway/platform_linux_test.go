//go:build linux

package fnosgateway

import (
	"strings"
	"testing"
)

const validProcess = "Uid:\t966 966 966 966\nGid:\t967 967 967 967\nGroups:\t\nCapInh:\t0000000000000000\nCapPrm:\t0000000000000000\nCapEff:\t0000000000000000\nCapAmb:\t0000000000000000\nNoNewPrivs:\t1\n"

func TestRuntimeGroupIsExplicitNotPasswdPrimary(t *testing.T) {
	if !validStatus([]byte(validProcess), 966, 967) {
		t.Fatal("dedicated runtime identity rejected")
	}
	for _, bad := range []string{
		strings.Replace(validProcess, "967 967 967 967", "901 901 901 901", 1),
		strings.Replace(validProcess, "966 966 966 966", "966 966 0 966", 1),
		strings.Replace(validProcess, "Groups:\t", "Groups:\t901 ", 1),
		strings.Replace(validProcess, "Groups:\t\n", "", 1),
		strings.Replace(validProcess, "CapAmb:\t0000000000000000", "CapAmb:\t0000000000000001", 1),
		strings.Replace(validProcess, "NoNewPrivs:\t1", "NoNewPrivs:\t0", 1),
	} {
		if validStatus([]byte(bad), 966, 967) {
			t.Fatal("unsafe or unknown process accepted")
		}
	}
}
func TestUntrustedConfigPathRefused(t *testing.T) {
	for _, p := range []string{"relative.json", "/tmp/../etc/passwd", "/tmp/nonexistent-ndg-config"} {
		if _, e := LoadConfig(p); e == nil {
			t.Fatal("untrusted path accepted")
		}
	}
}
