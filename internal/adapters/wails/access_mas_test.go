//go:build mas

package wails

import (
	"errors"
	"github.com/FNB2026/nas-data-governance/internal/fileaccess"
	"testing"
)

func TestMASDefaultConstructorAndPickerInjectionCannotBypass(t *testing.T) {
	a := NewAPI()
	a.SetDirectoryPicker(func(string) (string, error) { return t.TempDir(), nil })
	if _, e := a.PickDirectory(""); !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatal("legacy picker granted authority")
	}
	if _, e := a.CreateProjectFromSource(CreateProjectInput{SourcePath: t.TempDir()}); !errors.Is(e, fileaccess.ErrAuthorizationRequired) {
		t.Fatal("default constructor bypassed grant policy")
	}
	if _, e := a.ExecuteRestore(ExecuteRestoreRequest{}); !errors.Is(e, fileaccess.ErrWriteDisabled) {
		t.Fatal("default constructor enabled source writes")
	}
}
