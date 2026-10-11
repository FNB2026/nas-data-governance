//go:build darwin && masprobe

package main

import "testing"

func TestInvalidBookmarkFailsClosed(t *testing.T) {
	for _, raw := range []string{"", "not a base64 bookmark", "bm90LWEtYm9va21hcms="} {
		g, err := resolveBookmark(raw)
		if err != nil || g.Error != "authorization_required" || g.Path != "" || g.Bookmark != "" {
			t.Fatal("invalid authority must not grant a source")
		}
	}
}
func TestProjectIdentitySeparated(t *testing.T) {
	if storageID("/synthetic/a") == storageID("/synthetic/b") {
		t.Fatal("roots must not share project identity")
	}
	if storageID("/synthetic/a") != storageID("/synthetic/a") {
		t.Fatal("root identity must survive restart")
	}
}
