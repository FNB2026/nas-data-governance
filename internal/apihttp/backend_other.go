//go:build !linux

package apihttp

// The HTTP contract can be tested elsewhere; host filesystem execution is Linux-only.
type LocalBackend struct{}

func OpenLocal(string, map[string]string) (*LocalBackend, error) { return nil, ErrDenied }
func (*LocalBackend) Sources() []Source                          { return nil }
func (*LocalBackend) Snapshot(string) (Snapshot, error)          { return Snapshot{}, ErrDenied }
func (*LocalBackend) Start(string, bool) error                   { return ErrDenied }
func (*LocalBackend) Cancel(string) error                        { return ErrDenied }
func (*LocalBackend) Close() error                               { return ErrDenied }
