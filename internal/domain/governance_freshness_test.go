package domain

import "testing"

func TestFreshnessReasonsAreStatusBoundAndAllowlisted(t *testing.T) {
	for _, tc := range []struct{ status, reason, want string }{
		{"UNVERIFIED", "SOURCE_PLATFORM_PROOF_UNAVAILABLE", "SOURCE_PLATFORM_PROOF_UNAVAILABLE"},
		{"UNVERIFIED", "SOURCE_VERIFICATION_CANCELLED", "SOURCE_VERIFICATION_CANCELLED"},
		{"AUTHORIZATION_REQUIRED", "SOURCE_PERMISSION_DENIED", "SOURCE_PERMISSION_DENIED"},
		{"SOURCE_CHANGED", "SOURCE_CONTENT_CHANGED", "SOURCE_CONTENT_CHANGED"},
		{"SOURCE_UNAVAILABLE", "SOURCE_MOUNT_BOUNDARY_REJECTED", "SOURCE_MOUNT_BOUNDARY_REJECTED"},
		{"UNVERIFIED", "SOURCE_PERMISSION_DENIED", "SOURCE_UNVERIFIED"},
		{"SOURCE_CHANGED", "SOURCE_PLATFORM_PROOF_UNAVAILABLE", "SOURCE_CHANGED"},
		{"SOURCE_UNAVAILABLE", "private-path-and-business-marker", "SOURCE_UNAVAILABLE"},
		{"UNVERIFIED", "BOOKMARK_EXPIRED", "SOURCE_UNVERIFIED"},
		{"UNVERIFIED", "VOLUME_UUID_CHANGED", "SOURCE_UNVERIFIED"},
		{"MATCH", "SOURCE_CONTENT_CHANGED", "SOURCE_UNVERIFIED"},
	} {
		check := SourceFreshness{Status: tc.status, ReasonCode: tc.reason}
		if got := SourceFreshnessRejection(check, true); got != tc.want {
			t.Fatalf("status %q reason %q: got %q, want %q", tc.status, tc.reason, got, tc.want)
		}
		if got := SourceFreshnessRejection(check, false); got != "SOURCE_UNVERIFIED" {
			t.Fatal("an absent observation must not carry a diagnosis")
		}
	}
}
