// ndg-fnos-https is an optional non-root TLS adapter, not a file worker.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/FNB2026/nas-data-governance/internal/fnosgateway"
)

var configPath = "/var/apps/ndg-readonly-web/cmd/https.json"

func run(ctx context.Context) error {
	if e := fnosgateway.LockPrivileges(); e != nil {
		return e
	}
	c, e := fnosgateway.LoadHTTPSConfig(configPath)
	if e != nil {
		return e
	}
	if e = fnosgateway.CheckIdentity(c.Gateway); e != nil {
		return e
	}
	cert, key, e := fnosgateway.LoadTLSMaterial(configPath)
	if e != nil {
		return e
	}
	defer clear(key)
	server, e := fnosgateway.NewHTTPSServer(c, cert, key)
	clear(key)
	if e != nil {
		return e
	}
	listener, e := net.Listen("tcp4", c.Listen)
	if e != nil {
		return fnosgateway.ErrRejected
	}
	return fnosgateway.RunHTTPS(ctx, server, listener)
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if run(ctx) != nil {
		fmt.Fprintln(os.Stderr, "gateway_unavailable")
		os.Exit(1)
	}
}
