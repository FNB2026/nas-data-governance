//go:build linux

package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/govweb"
	"github.com/FNB2026/nas-data-governance/internal/managedworker"
	"github.com/FNB2026/nas-data-governance/internal/store"
)

const (
	workerUID = 10002
	webUID    = 10001
)

var workerSubject = managedworker.Subject{UID: workerUID, GID: workerUID}
var webSubject = managedworker.Subject{UID: webUID, GID: webUID}

func runWorker(ctx context.Context) (result error) {
	if err := managedworker.RequireSubject(workerSubject); err != nil {
		return err
	}
	layout := managedworker.Layout{Run: "/run/ndg-worker", State: "/state"}
	if err := managedworker.RequireLayout(layout, workerSubject); err != nil {
		return err
	}
	lease, err := managedworker.AcquireLease(layout.Run, workerSubject)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, lease.Release()) }()
	project, err := store.Open(ctx, filepath.Join(layout.State, "project.db"))
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, project.Close()) }()
	purge, err := managedworker.NewQuarantinePurge(project, "/quarantine")
	if err != nil {
		return err
	}
	server, err := managedworker.Listen(layout, workerSubject, []managedworker.Subject{webSubject})
	if err != nil {
		return err
	}
	defer server.Close()
	stop := context.AfterFunc(ctx, func() { _ = server.Close() })
	defer stop()
	for ctx.Err() == nil {
		conn, err := server.Accept()
		if errors.Is(err, managedworker.ErrUnauthorized) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		_ = conn.ServeQuarantinePurge(ctx, purge)
	}
	return nil
}

func runWeb(ctx context.Context) error {
	if err := managedworker.RequireSubject(webSubject); err != nil {
		return err
	}
	addr := "10.254.140.2:8443"
	web, err := govweb.New(govweb.Config{
		Addr: addr, LANAllowlist: []string{"10.254.140.2/32"}, RequireTLS: true,
		CredentialPath: "/config/token", Backing: govweb.ClosedBacking{},
		QuarantinePurge: managedworker.QuarantineClient{RunDir: "/run/ndg-worker", Worker: workerSubject},
	})
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr: addr, Handler: web.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 5 * time.Minute, IdleTimeout: 30 * time.Second,
	}
	stop := context.AfterFunc(ctx, func() { _ = server.Close() })
	defer stop()
	err = server.ListenAndServeTLS("/config/tls.crt", "/config/tls.key")
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func main() {
	role := flag.String("role", "", "worker or web")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := managedworker.RequireNoNewPrivs(); err != nil {
		log.Fatal("QUARANTINE_CANDIDATE_ISOLATION_REFUSED")
	}
	var err error
	switch *role {
	case "worker":
		err = runWorker(ctx)
	case "web":
		err = runWeb(ctx)
	default:
		log.Fatal("QUARANTINE_CANDIDATE_ROLE_REQUIRED")
	}
	if err != nil {
		log.Fatal("QUARANTINE_CANDIDATE_START_OR_RUNTIME_FAILED")
	}
}
