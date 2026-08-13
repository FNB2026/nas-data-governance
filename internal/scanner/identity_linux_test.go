//go:build linux

package scanner

import "testing"

func TestProfileForLinuxMagicKeepsNetworkIdentityUnreliable(t *testing.T) {
	for _, magic := range []uint64{linuxNFSMagic, linuxFuseMagic, linuxCIFSMagic, linuxSMB2Magic, linux9PMagic} {
		_, network, reliable := profileForLinuxMagic(magic)
		if !network || reliable {
			t.Fatalf("magic %#x: network=%v reliable=%v, want true/false", magic, network, reliable)
		}
	}
}

func TestProfileForLinuxMagicAllowsKnownLocalIdentity(t *testing.T) {
	for _, magic := range []uint64{linuxExtMagic, linuxXFSMagic, linuxBtrfsMagic, linuxTmpfsMagic, linuxOverlayMagic, linuxZFSMagic} {
		_, network, reliable := profileForLinuxMagic(magic)
		if network || !reliable {
			t.Fatalf("magic %#x: network=%v reliable=%v, want false/true", magic, network, reliable)
		}
	}
	_, network, reliable := profileForLinuxMagic(0xdeadbeef)
	if network || reliable {
		t.Fatalf("unknown magic must remain conservative: network=%v reliable=%v", network, reliable)
	}
}
