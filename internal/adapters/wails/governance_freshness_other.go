//go:build !linux && !darwin

package wails

func governanceFreshnessMountProofAvailable() bool { return false }
