//go:build !linux

package fnosgateway

func LoadConfig(string) (Config, error) { return Config{}, ErrRejected }
func LockPrivileges() error             { return ErrRejected }
func CheckIdentity(Config) error        { return ErrRejected }
