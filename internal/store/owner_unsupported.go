//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package store

import "errors"

var ErrWriterUnavailable = errors.New("store: project writer unavailable")

// This increment does not certify a new owner protocol on unsupported hosts.
func acquireOwner(string) (func(), error) { return nil, ErrWriterUnavailable }
func databaseScopeBinding(string) string  { return "" }
