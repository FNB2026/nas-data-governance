// ndg-fnos-https is an optional non-root TLS adapter, not a file worker.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/FNB2026/nas-data-governance/internal/fnosgateway"
)

var configPath = "/var/apps/ndg-readonly-web/cmd/https.json"

var instanceID = regexp.MustCompile(`^[0-9a-f]{12}$`)

func instanceConfig(args []string) (string, string, error) {
	if len(args) == 0 {
		return configPath, "", nil
	}
	if len(args) != 2 || args[0] != "--instance" || !instanceID.MatchString(args[1]) {
		return "", "", fnosgateway.ErrRejected
	}
	return "/etc/ndg/instances/" + args[1] + "/https.json", "ndgweb-" + args[1], nil
}

func run(ctx context.Context, args []string) error {
	config, identity, e := instanceConfig(args)
	if e != nil {
		return e
	}
	if e := fnosgateway.LockPrivileges(); e != nil {
		return e
	}
	c, e := fnosgateway.LoadHTTPSConfig(config)
	if e != nil {
		return e
	}
	if identity != "" && (c.Gateway.AppName != identity || c.Gateway.PackageUser != identity || c.Gateway.PackageGroup != identity) {
		return fnosgateway.ErrRejected
	}
	if e = fnosgateway.CheckIdentity(c.Gateway); e != nil {
		return e
	}
	cert, key, e := fnosgateway.LoadTLSMaterial(config)
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
	if run(ctx, os.Args[1:]) != nil {
		fmt.Fprintln(os.Stderr, "gateway_unavailable")
		os.Exit(1)
	}
}
