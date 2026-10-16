// ndg-fnos-cgi is a non-privileged, short-request fnOS adapter, not a worker.
package main

import (
	"fmt"
	"net/http/cgi"
	"os"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/fnosgateway"
)

var configPath = "/var/apps/ndg-readonly-web/cmd/gateway.json"

func run() error {
	if e := fnosgateway.LockPrivileges(); e != nil {
		return e
	}
	c, e := fnosgateway.LoadConfig(configPath)
	if e != nil {
		return e
	}
	if e = fnosgateway.CheckIdentity(c); e != nil {
		return e
	}
	h, e := fnosgateway.New(c)
	if e != nil {
		return e
	}
	return cgi.Serve(h)
}
func main() {
	// CGI has no http.Server.ReadTimeout. Terminate the whole short-lived
	// process even if stdin stalls; a context cannot interrupt arbitrary Read.
	deadline := time.AfterFunc(20*time.Second, func() { os.Exit(1) })
	defer deadline.Stop()
	if run() != nil {
		fmt.Print("Status: 503 Service Unavailable\r\nContent-Type: application/json\r\nCache-Control: no-store\r\nContent-Security-Policy: default-src 'none'; frame-ancestors 'none'\r\nX-Content-Type-Options: nosniff\r\n\r\n{\"error\":\"gateway_unavailable\"}\n")
	}
}
