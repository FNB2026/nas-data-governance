//go:build !linux

package managedworker

// These kernel primitives do not exist here. The refusals are fixed categories
// so that a caller cannot mistake an unsupported platform for an authorized
// one, and a local build stays meaningful for editing and cross-compilation.
func RequireSubject(Subject) error { return ErrUnsupported }

func RequireLayout(Layout, Subject) error { return ErrUnsupported }

func Listen(Layout, Subject, []Subject) (*Listener, error) { return nil, ErrUnsupported }

func AcquireLease(string, Subject) (*Lease, error) { return nil, ErrUnsupported }

func LeaseHolder(string) (int, bool, error) { return 0, false, ErrUnsupported }
