package fnosgateway

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/netutil"
)

type HTTPSConfig struct {
	Gateway Config `json:"gateway"`
	Listen  string `json:"listen"`
}

func decodeConfig(b []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(target) != nil || d.Decode(new(any)) != io.EOF {
		return ErrRejected
	}
	return nil
}

var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// NewHTTPSServer terminates real TLS; forwarded headers never supply TLS state.
// It exposes only the existing read-only UI and fixed API allowlist. Callers
// must verify native identity/configuration before binding the listener.
func NewHTTPSServer(c HTTPSConfig, certPEM, keyPEM []byte) (*http.Server, error) {
	g, e := New(c.Gateway)
	if e != nil {
		return nil, ErrRejected
	}
	o, _ := url.Parse(c.Gateway.Origin)
	name := o.Hostname()
	if len(name) > 253 || net.ParseIP(name) != nil || !strings.Contains(name, ".") {
		return nil, ErrRejected
	}
	for _, label := range strings.Split(name, ".") {
		if !dnsLabel.MatchString(label) {
			return nil, ErrRejected
		}
	}
	bind, port, e := net.SplitHostPort(c.Listen)
	if e != nil {
		return nil, ErrRejected
	}
	ip := net.ParseIP(bind)
	n, e := strconv.Atoi(port)
	if e != nil || n < 1024 || n > 65535 || strconv.Itoa(n) != port || ip == nil || ip.To4() == nil || !ip.IsPrivate() && !ip.IsLoopback() || o.Port() != port {
		return nil, ErrRejected
	}
	pair, e := tls.X509KeyPair(certPEM, keyPEM)
	if e != nil || len(pair.Certificate) == 0 {
		return nil, ErrRejected
	}
	leaf, e := x509.ParseCertificate(pair.Certificate[0])
	if e != nil || leaf.VerifyHostname(name) != nil || time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
		return nil, ErrRejected
	}
	serverUsage := len(leaf.ExtKeyUsage) == 0 && len(leaf.UnknownExtKeyUsage) == 0
	for _, usage := range leaf.ExtKeyUsage {
		serverUsage = serverUsage || usage == x509.ExtKeyUsageServerAuth || usage == x509.ExtKeyUsageAny
	}
	if !serverUsage {
		return nil, ErrRejected
	}
	switch key := pair.PrivateKey.(type) {
	case *rsa.PrivateKey:
		if key.N.BitLen() < 2048 {
			return nil, ErrRejected
		}
	case *ecdsa.PrivateKey:
		if key.Curve.Params().BitSize < 256 {
			return nil, ErrRejected
		}
	case ed25519.PrivateKey:
		if len(key) != ed25519.PrivateKeySize {
			return nil, ErrRejected
		}
	default:
		return nil, ErrRejected
	}
	pair.Leaf = leaf
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if !strings.EqualFold(hello.ServerName, name) || time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
			return nil, ErrRejected
		}
		return &pair, nil
	}}
	// Dedicated HTTPS origin serves the UI at its root. CGI keeps its existing
	// prefix and authentication model; no general caller-supplied prefix exists.
	g.prefix = "/"
	return &http.Server{Addr: c.Listen, Handler: g, TLSConfig: tlsConfig, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 8192, ErrorLog: log.New(io.Discard, "", 0)}, nil
}

// RunHTTPS bounds concurrent connections before TLS handshakes. It does not
// create a listener, load keys, use an environment proxy, or open an HTTP port.
func RunHTTPS(ctx context.Context, s *http.Server, listener net.Listener) error {
	done := make(chan error, 1)
	go func() { done <- s.Serve(tls.NewListener(netutil.LimitListener(listener, 64), s.TLSConfig)) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) && ctx.Err() != nil {
			return nil
		}
		return ErrRejected
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if s.Shutdown(shutdown) != nil {
			_ = s.Close()
		}
		err := <-done
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return ErrRejected
		}
		return nil
	}
}
