//go:build !darwin

package scanner

// Unknown platforms default to conservative identity handling. Platform-
// specific support can opt in after filesystem-type reliability is verified.
func physicalIdentityReliable(string) bool { return false }

func filesystemProfile(string) (string, bool, bool) { return "unknown", false, false }
