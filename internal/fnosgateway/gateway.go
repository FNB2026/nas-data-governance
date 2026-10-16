// Package fnosgateway provides bounded, read-only fnOS adapters. They own no source,
// database or Docker permissions. NAS cookies and proxy headers never reach core.
package fnosgateway

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/apihttp"
)

var ErrRejected = errors.New("gateway unavailable")
var appName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type Config struct {
	AppName      string `json:"app_name"`
	Origin       string `json:"origin"`
	Upstream     string `json:"upstream"`
	PackageUser  string `json:"package_user"`
	PackageGroup string `json:"package_group"`
}
type Gateway struct {
	cfg            Config
	origin         *url.URL
	prefix, cookie string
	client         *http.Client
}

func New(c Config) (*Gateway, error) {
	o, e := url.Parse(c.Origin)
	if e != nil || o.Scheme != "https" || o.Host == "" || o.User != nil || o.Path != "" || o.RawQuery != "" || o.Fragment != "" {
		return nil, ErrRejected
	}
	u, e := url.Parse(c.Upstream)
	if e != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Port() != "8080" || net.ParseIP(u.Hostname()) == nil {
		return nil, ErrRejected
	}
	ip := net.ParseIP(u.Hostname())
	if ip.To4() == nil || !ip.IsPrivate() {
		return nil, ErrRejected
	}
	if !appName.MatchString(c.AppName) || !appName.MatchString(c.PackageUser) || !appName.MatchString(c.PackageGroup) {
		return nil, ErrRejected
	}
	sum := sha256.Sum256([]byte(c.AppName))
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 3 * time.Second}).DialContext, DisableKeepAlives: true, ResponseHeaderTimeout: 8 * time.Second, MaxResponseHeaderBytes: 8192}
	return &Gateway{cfg: c, origin: o, prefix: "/cgi/ThirdParty/" + c.AppName + "/index.cgi/", cookie: "ndg_" + hex.EncodeToString(sum[:6]), client: &http.Client{Transport: tr, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func reject(w http.ResponseWriter, n int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(n)
	_, _ = io.WriteString(w, `{"error":"request_rejected"}`)
}
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'self'")
	if r.TLS == nil || r.Host != g.origin.Host || r.URL.RawQuery != "" || r.URL.RawPath != "" || r.Header.Get("Origin") != "" && r.Header.Get("Origin") != g.cfg.Origin || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		reject(w, 403)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == strings.TrimSuffix(g.prefix, "/") {
		w.Header().Set("Location", g.prefix)
		w.WriteHeader(307)
		return
	}
	if !strings.HasPrefix(r.URL.Path, g.prefix) {
		reject(w, 403)
		return
	}
	route := "/" + strings.TrimPrefix(r.URL.Path, g.prefix)
	if path.Clean(route) != route || strings.Contains(route, "\\") {
		reject(w, 403)
		return
	}
	if r.Method == http.MethodGet && (route == "/" || strings.HasPrefix(route, "/assets/")) {
		q := r.Clone(r.Context())
		u := *r.URL
		u.Path = route
		u.RawPath = ""
		q.URL = &u
		apihttp.ServeReadOnlyUI(w, q)
		return
	}
	allowed := false
	switch route {
	case "/api/session", "/api/sources", "/api/capabilities", "/api/version":
		allowed = r.Method == http.MethodGet
	case "/api/login", "/api/logout", "/api/status", "/api/scan", "/api/cancel":
		allowed = r.Method == http.MethodPost
	}
	if !allowed {
		reject(w, 403)
		return
	}
	if r.Method == http.MethodPost && (r.Header.Get("Origin") != g.cfg.Origin || r.Header.Get("Content-Type") != "application/json") {
		reject(w, 403)
		return
	}
	var body []byte
	if r.Body != nil {
		b, e := io.ReadAll(io.LimitReader(r.Body, 4097))
		if e != nil || len(b) > 4096 {
			reject(w, 413)
			return
		}
		body = b
	}
	if r.Method == http.MethodGet && len(body) != 0 {
		reject(w, 400)
		return
	}
	q, e := http.NewRequestWithContext(r.Context(), r.Method, g.cfg.Upstream+route, bytes.NewReader(body))
	if e != nil {
		reject(w, 502)
		return
	}
	q.Host = g.origin.Host
	for _, h := range []string{"Accept", "Content-Type", "Origin", "X-CSRF-Token", "Sec-Fetch-Site"} {
		if v := r.Header.Get(h); v != "" {
			q.Header.Set(h, v)
		}
	}
	// Never forward the fnOS session, user identity, Authorization, X-Trim-*,
	// Forwarded, X-Forwarded-* or a client-selected destination.
	cookies := r.Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == g.cookie {
			if found || !validToken(c.Value) {
				reject(w, 403)
				return
			}
			found = true
			q.AddCookie(&http.Cookie{Name: "ndg_session", Value: c.Value})
		}
	}
	res, e := g.client.Do(q)
	if e != nil {
		reject(w, 503)
		return
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 && res.StatusCode < 400 {
		reject(w, 502)
		return
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		n := res.StatusCode
		switch n {
		case 400, 401, 403, 405, 409, 429, 503:
		default:
			n = 502
		}
		reject(w, n)
		return
	}
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "application/json") {
		reject(w, 502)
		return
	}
	data, e := io.ReadAll(io.LimitReader(res.Body, 262145))
	if e != nil || len(data) > 262144 || !json.Valid(data) {
		reject(w, 502)
		return
	}
	for _, c := range res.Cookies() {
		if c.Name != "ndg_session" || c.MaxAge >= 0 && !validToken(c.Value) {
			reject(w, 502)
			return
		}
		c.Name = g.cookie
		c.Path = g.prefix
		c.Domain = ""
		c.HttpOnly = true
		c.Secure = true
		c.SameSite = http.SameSiteStrictMode
		http.SetCookie(w, c)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(data)
}
func validToken(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil
}
