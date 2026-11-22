//go:build darwin && mas

package macaccess

import (
	"context"
	"errors"
	"testing"

	"github.com/FNB2026/nas-data-governance/internal/fileaccess"
)

func TestMalformedNativeBookmarksDoNotReturnAuthority(t *testing.T) {
	for _, token := range []string{"", "not-base64", "bm90LWEtYm9va21hcms="} {
		g, stop, e := (Native{}).Resolve(context.Background(), fileaccess.Grant{Bookmark: token})
		if stop != nil {
			stop()
		}
		if !errors.Is(e, fileaccess.ErrAuthorizationRequired) || g.Root != "" || g.Bookmark != "" {
			t.Fatal("invalid bookmark returned authority or private error")
		}
	}
}
func TestCancelledResolveDoesNotStartScope(t *testing.T) {
	c, cancel := context.WithCancel(context.Background())
	cancel()
	g, stop, e := (Native{}).Resolve(c, fileaccess.Grant{Bookmark: "invalid"})
	if stop != nil || g.Root != "" || !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatal("cancelled request activated scope")
	}
}

func TestSupportContainerMatchesCurrentMASBundle(t *testing.T) {
	const base = "/sandbox-test/Library/Containers/"
	const suffix = "/Data/Library/Application Support/NDG"
	for _, tc := range []struct {
		path, bundle string
		want         bool
	}{
		{base + "com.fnb.ndg.mas" + suffix, "com.fnb.ndg.mas", true},
		{base + "com.fnb.ndg.mas.launchqa" + suffix, "com.fnb.ndg.mas.launchqa", true},
		{base + "com.fnb.ndg.mas.phase1a" + suffix, "com.fnb.ndg.mas.phase1a", true},
		{base + "com.fnb.ndg.mas" + suffix, "com.fnb.ndg.mas.launchqa", false},
		{base + "com.fnb.ndg" + suffix, "com.fnb.ndg", false},
		{base + "com.fnb.ndg.mas/../com.fnb.ndg.mas" + suffix, "com.fnb.ndg.mas", false},
		{base + "com.fnb.ndg.mas" + suffix + "/other", "com.fnb.ndg.mas", false},
	} {
		if got := isSupportContainer(tc.path, tc.bundle); got != tc.want {
			t.Errorf("isSupportContainer(%q, %q) = %t, want %t", tc.path, tc.bundle, got, tc.want)
		}
	}
}
