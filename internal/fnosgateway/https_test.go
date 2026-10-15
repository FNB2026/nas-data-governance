package fnosgateway

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func httpsFixture(t *testing.T, name string, expired bool) (HTTPSConfig, []byte, []byte) {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	until := time.Now().Add(time.Hour)
	if expired {
		until = time.Now().Add(-time.Minute)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: until, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, e := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	private, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	cfg := HTTPSConfig{Gateway: Config{AppName: "ndg-test", Origin: "https://ndg.invalid:24483", Upstream: "http://10.98.45.2:8080", PackageUser: "ndgtest", PackageGroup: "ndgtest"}, Listen: "127.0.0.1:24483"}
	return cfg, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})
}

func TestHTTPSNativeBoundaryOnRealTLSConnection(t *testing.T) {
	cfg, cert, key := httpsFixture(t, "ndg.invalid", false)
	s, e := NewHTTPSServer(cfg, cert, key)
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"wrong.invalid", ""} {
		if _, e := s.TLSConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: name}); e == nil {
			t.Fatal("SNI policy missing")
		}
	}
	g := s.Handler.(*Gateway)
	var upstream atomic.Int32
	g.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		upstream.Add(1)
		if r.URL.String() != cfg.Gateway.Upstream+"/api/session" || r.Host != "ndg.invalid:24483" {
			t.Fatal("destination changed")
		}
		for _, h := range []string{"Authorization", "Forwarded", "X-Forwarded-Proto", "X-Trim-Userid"} {
			if r.Header.Get(h) != "" {
				t.Fatal("untrusted header forwarded")
			}
		}
		for _, c := range r.Cookies() {
			if c.Name != "ndg_session" {
				t.Fatal("foreign cookie forwarded")
			}
		}
		return &http.Response{StatusCode: 401, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader("private-backend-canary"))}, nil
	})
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- RunHTTPS(ctx, s, ln) }()
	t.Cleanup(func() {
		cancel()
		if e := <-done; e != nil {
			t.Errorf("shutdown: %v", e)
		}
	})
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(cert) {
		t.Fatal("test root")
	}
	client := &http.Client{Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "ndg.invalid", MinVersion: tls.VersionTLS12}}, Timeout: 3 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	for _, tc := range []struct {
		path, host, origin string
		want               int
	}{
		{"/", "ndg.invalid:24483", "", 200},
		{"/", "wrong.invalid:24483", "", 403},
		{"/", "ndg.invalid:24483", "https://foreign.invalid", 403},
		{"/api/session", "ndg.invalid:24483", "", 401},
		{"/api/purge/execute", "ndg.invalid:24483", "", 403},
		{"/api/restore/execute", "ndg.invalid:24483", "", 403},
		{"/api/quarantine/execute", "ndg.invalid:24483", "", 403},
	} {
		r, e := http.NewRequest("GET", "https://"+ln.Addr().String()+tc.path, nil)
		if e != nil {
			t.Fatal(e)
		}
		r.Host = tc.host
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Forwarded", "proto=https;host=ndg.invalid")
		r.Header.Set("X-Forwarded-Proto", "http")
		r.Header.Set("X-Trim-Userid", "synthetic-private-canary")
		r.Header.Set("Authorization", "synthetic-private-canary")
		r.AddCookie(&http.Cookie{Name: "fnos_session", Value: "synthetic-private-canary"})
		res, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if e != nil || res.StatusCode != tc.want || strings.Contains(string(b), "canary") {
			t.Fatalf("unexpected TLS result: %d %v", res.StatusCode, e)
		}
	}
	if upstream.Load() != 1 {
		t.Fatal("forbidden request reached upstream")
	}
	for _, name := range []string{"wrong.invalid", ""} {
		c, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", ln.Addr().String(), &tls.Config{RootCAs: roots, ServerName: name, MinVersion: tls.VersionTLS12})
		if e == nil {
			c.Close()
			t.Fatal("wrong/missing SNI accepted")
		}
	}
	c, e := net.DialTimeout("tcp", ln.Addr().String(), time.Second)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(time.Second))
	_, _ = io.WriteString(c, "GET /api/session HTTP/1.1\r\nHost: ndg.invalid:24483\r\nX-Forwarded-Proto: https\r\nConnection: close\r\n\r\n")
	res, e := http.ReadResponse(bufio.NewReader(c), nil)
	if e != nil || res.StatusCode != 400 {
		t.Fatal("plaintext accepted")
	}
	_ = res.Body.Close()
	if upstream.Load() != 1 {
		t.Fatal("plaintext reached core")
	}
}

func TestHTTPSConfigAndCertificateRefusals(t *testing.T) {
	cfg, cert, key := httpsFixture(t, "ndg.invalid", false)
	for _, listen := range []string{"0.0.0.0:24483", ":24483", "8.8.8.8:24483", "[::]:24483", "127.0.0.1:443", "127.0.0.1:024483", "127.0.0.1:24484"} {
		c := cfg
		c.Listen = listen
		if _, e := NewHTTPSServer(c, cert, key); e == nil {
			t.Fatal("unsafe bind accepted", listen)
		}
	}
	for _, origin := range []string{"https://192.168.31.193:24483", "https://localhost:24483", "https://NDG.invalid:24483", "https://-ndg.invalid:24483", "https://ndg.invalid.:24483"} {
		c := cfg
		c.Gateway.Origin = origin
		if _, e := NewHTTPSServer(c, cert, key); e == nil {
			t.Fatal("invalid dedicated hostname accepted")
		}
	}
	for _, tc := range []struct {
		name    string
		expired bool
	}{{"foreign.invalid", false}, {"ndg.invalid", true}} {
		_, c, k := httpsFixture(t, tc.name, tc.expired)
		if _, e := NewHTTPSServer(cfg, c, k); e == nil {
			t.Fatal("wrong/expired certificate accepted")
		}
	}
	_, wrongCert, _ := httpsFixture(t, "foreign.invalid", false)
	if _, e := NewHTTPSServer(cfg, wrongCert, key); e == nil {
		t.Fatal("mismatched key accepted")
	}
	if _, e := NewHTTPSServer(cfg, cert, []byte("not a key")); e == nil {
		t.Fatal("malformed key accepted")
	}
	var c HTTPSConfig
	for _, b := range []string{`{"listen":"127.0.0.1:24483","extra":true}`, `{} {}`, `not json`} {
		if decodeConfig([]byte(b), &c) == nil {
			t.Fatal("ambiguous config accepted")
		}
	}
}

func TestHTTPSConnectionLimitReleasesAfterClose(t *testing.T) {
	cfg, cert, key := httpsFixture(t, "ndg.invalid", false)
	s, e := NewHTTPSServer(cfg, cert, key)
	if e != nil {
		t.Fatal(e)
	}
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- RunHTTPS(ctx, s, ln) }()
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(cert)
	conns := make([]net.Conn, 0, 64)
	t.Cleanup(func() {
		for _, c := range conns {
			_ = c.Close()
		}
		cancel()
		if e := <-done; e != nil {
			t.Error(e)
		}
	})
	config := &tls.Config{RootCAs: roots, ServerName: "ndg.invalid", MinVersion: tls.VersionTLS12}
	for range 64 {
		c, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", ln.Addr().String(), config)
		if e != nil {
			t.Fatal("bounded TLS connection failed", e)
		}
		conns = append(conns, c)
	}
	if c, e := tls.DialWithDialer(&net.Dialer{Timeout: 200 * time.Millisecond}, "tcp", ln.Addr().String(), config); e == nil {
		c.Close()
		t.Fatal("connection bound exceeded")
	}
	_ = conns[0].Close()
	c, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", ln.Addr().String(), config)
	if e != nil {
		t.Fatal("connection slot not released", e)
	}
	_ = c.Close()
}
