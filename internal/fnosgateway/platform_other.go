//go:build !linux

package fnosgateway

func LoadConfig(string) (Config, error) { return Config{}, ErrRejected }
func LockPrivileges() error             { return ErrRejected }
func CheckIdentity(Config) error        { return ErrRejected }

func LoadHTTPSConfig(string) (HTTPSConfig, error)    { return HTTPSConfig{}, ErrRejected }
func LoadTLSMaterial(string) ([]byte, []byte, error) { return nil, nil, ErrRejected }
