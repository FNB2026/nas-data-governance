//go:build !linux

package scanner

import (
	"errors"
	"os"

	"github.com/FNB2026/nas-data-governance/internal/domain"
)

var ErrMountBoundary = errors.New("native scan mount boundary unsupported")

type Boundary struct{}

func OpenBoundary(string) (*Boundary, error)                               { return nil, ErrMountBoundary }
func (*Boundary) Close() error                                             { return ErrMountBoundary }
func (*Boundary) VerifyRoot() error                                        { return ErrMountBoundary }
func (*Boundary) ReadDirectory(string) ([]os.DirEntry, error)              { return nil, ErrMountBoundary }
func (*Boundary) ReadFile(domain.FileInstance, func(*os.File) error) error { return ErrMountBoundary }
