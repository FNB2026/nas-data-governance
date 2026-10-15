//go:build ndg_devprofile && !mas

package wails

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	projectsvc "github.com/FNB2026/nas-data-governance/internal/project"
)

var errDevProfile = errors.New("NDG_DEV_PROFILE_REQUIRED")

const devProfileMarker = "ndg-b2-dev-profile-v1\n"

// NewIsolatedDevAPI changes only project-service storage composition. It does
// not change capabilities, source readers, freshness, tokens or write guards.
// The launcher creates the disposable root; startup never creates a fallback.
func NewIsolatedDevAPI(root string) (*API, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, errDevProfile
	}
	temp, err := filepath.EvalSymlinks(os.TempDir())
	sharedTemp, sharedErr := filepath.EvalSymlinks("/tmp")
	if err != nil || sharedErr != nil || (filepath.Dir(root) != temp && filepath.Dir(root) != sharedTemp) || !strings.HasPrefix(filepath.Base(root), "ndg-b2-") {
		return nil, errDevProfile
	}
	initial, err := os.Lstat(root)
	if err != nil || !initial.IsDir() || initial.Mode().Perm() != 0o700 {
		return nil, errDevProfile
	}
	base := filepath.Join(root, "support")
	check := func() (string, error) {
		current, e := os.Lstat(root)
		if e != nil || !os.SameFile(initial, current) || !current.IsDir() || current.Mode().Perm() != 0o700 {
			return "", errDevProfile
		}
		marker := filepath.Join(root, ".ndg-b2-profile")
		m, e := os.Lstat(marker)
		if e != nil || !m.Mode().IsRegular() || m.Mode().Perm() != 0o600 || m.Size() != int64(len(devProfileMarker)) {
			return "", errDevProfile
		}
		body, e := os.ReadFile(marker)
		if e != nil || string(body) != devProfileMarker {
			return "", errDevProfile
		}
		if b, e := os.Lstat(base); e == nil {
			if !b.IsDir() || b.Mode().Perm() != 0o700 {
				return "", errDevProfile
			}
		} else if !os.IsNotExist(e) {
			return "", errDevProfile
		}
		return base, nil
	}
	if _, err := check(); err != nil {
		return nil, err
	}
	a := NewAPI()
	a.projectSvc = projectsvc.NewService(projectsvc.Options{SupportBase: check})
	return a, nil
}
