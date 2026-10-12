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
