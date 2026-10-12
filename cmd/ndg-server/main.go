// ndg-server is a local experimental read-only HTTP channel, not a Wails host.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"github.com/FNB2026/nas-data-governance/internal/apihttp"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type config struct {
	Listen         string            `json:"listen"`
	Origin         string            `json:"origin"`
	State          string            `json:"state"`
	CredentialFile string            `json:"credential_file"`
	Sources        map[string]string `json:"sources"`
}

func run() error {
	file := flag.String("config", "", "operator-owned configuration file")
	flag.Parse()
	if *file == "" {
		return errors.New("configuration required")
	}
	info, e := os.Lstat(*file)
	if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return errors.New("configuration rejected")
	}
	f, e := os.Open(*file)
	if e != nil {
		return errors.New("configuration rejected")
	}
	var c config
	d := json.NewDecoder(io.LimitReader(f, 65536))
	d.DisallowUnknownFields()
	e = d.Decode(&c)
	tail := d.Decode(new(any))
	_ = f.Close()
	if e != nil || tail != io.EOF {
		return errors.New("configuration rejected")
	}
	host, _, e := net.SplitHostPort(c.Listen)
	if e != nil || net.ParseIP(host) == nil {
		return errors.New("listen address must be an explicit IP")
	}
	ci, e := os.Lstat(c.CredentialFile)
	if e != nil || !ci.Mode().IsRegular() || ci.Mode().Perm() != 0600 || ci.Size() > 4096 {
		return errors.New("credential file rejected")
	}
	p, e := os.ReadFile(c.CredentialFile)
	if e != nil {
		return errors.New("credential file rejected")
	}
	b, e := apihttp.OpenLocal(c.State, c.Sources)
	if e != nil {
		return errors.New("state or source boundary rejected")
	}
	defer b.Close()
	h, e := apihttp.New(apihttp.Config{Origin: c.Origin, Password: strings.TrimSpace(string(p)), Secure: strings.HasPrefix(c.Origin, "https://")}, b)
	if e != nil {
		return errors.New("HTTP configuration rejected")
	}
	srv := &http.Server{Addr: c.Listen, Handler: h, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 8192, ErrorLog: log.New(io.Discard, "", 0)}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(stop)
	done := make(chan struct{})
	go func() {
		select {
		case <-stop:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctx)
		case <-done:
		}
	}()
	e = srv.ListenAndServe()
	close(done)
	if e != nil && !errors.Is(e, http.ErrServerClosed) {
		return errors.New("HTTP service stopped")
	}
	return nil
}
func main() {
	if run() != nil {
		_, _ = os.Stderr.WriteString("ndg-server: startup or shutdown rejected; private details omitted\n")
		os.Exit(1)
	}
}
