// Package apihttp exposes a deliberately narrow, authenticated read-only channel.
// Source paths, domain structs and raw errors never cross this adapter boundary.
package apihttp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/FNB2026/nas-data-governance/internal/version"
)

type Source struct {
	Alias string `json:"alias"`
}
type Group struct {
	ID     string `json:"id"`
	Copies int    `json:"copies"`
	Bytes  int64  `json:"bytes"`
}
type Snapshot struct {
	State       string  `json:"state"`
	Stage       string  `json:"stage"`
	Discovered  int64   `json:"discovered"`
	Processed   int64   `json:"processed"`
	Failed      int64   `json:"failed"`
	Checkpoint  string  `json:"checkpoint"`
	Scanned     int     `json:"scanned"`
	Active      int     `json:"active"`
	Missing     int     `json:"missing"`
	Unavailable int     `json:"unavailable"`
	Quick       int     `json:"quick"`
	Full        int     `json:"full"`
	Groups      []Group `json:"groups"`
}

// Backend has no execution or filesystem-write methods. Aliases are operator-only.
type Backend interface {
	Sources() []Source
	Snapshot(string) (Snapshot, error)
	Start(string, bool) error
	Cancel(string) error
}
type Config struct {
	Origin      string
	Password    string
	Secure      bool
	IdleTimeout time.Duration
}
type session struct {
	csrf     string
	touched  time.Time
	window   time.Time
	requests int
}
type Server struct {
	cfg      Config
	backend  Backend
	password [32]byte
	mu       sync.Mutex
	sessions map[string]session
	failures int
	retryAt  time.Time
	now      func() time.Time
}

var ErrDenied = errors.New("operation unavailable")

func New(cfg Config, b Backend) (*Server, error) {
	u, e := url.Parse(cfg.Origin)
	if b == nil || e != nil || u.User != nil || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || len(cfg.Password) < 32 || strings.TrimSpace(cfg.Password) != cfg.Password {
		return nil, ErrDenied
	}
	if cfg.Secure != (u.Scheme == "https") {
		return nil, ErrDenied
	}
	if cfg.IdleTimeout == 0 {
		cfg.IdleTimeout = 15 * time.Minute
	}
	if cfg.IdleTimeout < time.Second || cfg.IdleTimeout > time.Hour {
		return nil, ErrDenied
	}
	return &Server{cfg: cfg, backend: b, password: sha256.Sum256([]byte(cfg.Password)), sessions: map[string]session{}, now: time.Now}, nil
}
func nonce() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func reject(w http.ResponseWriter, status int) {
	reply(w, status, map[string]string{"error": "request_rejected"})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Header.Get("Content-Type") != "application/json" {
		reject(w, 400)
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		reject(w, 400)
		return false
	}
	return true
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'none'")
	u, _ := url.Parse(s.cfg.Origin)
	if r.Host != u.Host || r.URL.RawQuery != "" || r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.cfg.Origin || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		reject(w, 403)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/healthz" {
		reply(w, 200, map[string]string{"state": "alive"})
		return
	}
	if r.Method == "GET" && (r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/assets/")) {
		serveUI(w, r)
		return
	}
	if r.Method != "GET" && r.Method != "POST" {
		reject(w, 405)
		return
	}
	if r.Method == "POST" && r.Header.Get("Origin") != s.cfg.Origin {
		reject(w, 403)
		return
	}
	if r.URL.Path == "/api/login" && r.Method == "POST" {
		s.login(w, r)
		return
	}
	cookie, e := r.Cookie("ndg_session")
	if e != nil {
		reject(w, 401)
		return
	}
	s.mu.Lock()
	sess, ok := s.sessions[cookie.Value]
	if ok && s.now().Sub(sess.touched) > s.cfg.IdleTimeout {
		delete(s.sessions, cookie.Value)
		ok = false
	}
	if ok && s.now().Sub(sess.window) >= time.Minute {
		sess.window = s.now()
		sess.requests = 0
	}
	if ok && sess.requests >= 120 {
		s.mu.Unlock()
		reject(w, 429)
		return
	}
	if ok {
		sess.requests++
		sess.touched = s.now()
		s.sessions[cookie.Value] = sess
	}
	s.mu.Unlock()
	if !ok {
		reject(w, 401)
		return
	}
	if r.Method == "POST" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(sess.csrf)) != 1 {
		reject(w, 403)
		return
	}
	switch r.URL.Path {
	case "/api/session":
		if r.Method != "GET" {
			reject(w, 405)
			return
		}
		reply(w, 200, map[string]string{"csrf": sess.csrf})
	case "/api/logout":
		if r.Method != "POST" {
			reject(w, 405)
			return
		}
		s.mu.Lock()
		delete(s.sessions, cookie.Value)
		s.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "ndg_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cfg.Secure, SameSite: http.SameSiteStrictMode})
		reply(w, 200, map[string]bool{"logged_out": true})
	case "/readyz":
		if r.Method != "GET" {
			reject(w, 405)
			return
		}
		for _, source := range s.backend.Sources() {
			if _, e := s.backend.Snapshot(source.Alias); e != nil {
				reject(w, 503)
				return
			}
		}
		reply(w, 200, map[string]string{"state": "ready"})
	case "/api/version":
		if r.Method != "GET" {
			reject(w, 405)
			return
		}
		reply(w, 200, version.Get())
	case "/api/sources":
		if r.Method != "GET" {
			reject(w, 405)
			return
		}
		reply(w, 200, s.backend.Sources())
	case "/api/capabilities":
		if r.Method != "GET" {
			reject(w, 405)
			return
		}
		reply(w, 200, map[string]bool{"scan": true, "resume": true, "quarantine": false, "restore": false, "purge": false, "plan_execution": false})
	case "/api/status", "/api/scan", "/api/cancel":
		if r.Method != "POST" {
			reject(w, 405)
			return
		}
		var in struct {
			Alias  string `json:"alias"`
			Resume bool   `json:"resume"`
		}
		if !decode(w, r, &in) {
			return
		}
		if !validAlias(in.Alias) {
			reject(w, 400)
			return
		}
		if r.URL.Path == "/api/status" {
			v, e := s.backend.Snapshot(in.Alias)
			if e != nil {
				reject(w, 409)
				return
			}
			reply(w, 200, v)
			return
		}
		if r.URL.Path == "/api/scan" {
			e = s.backend.Start(in.Alias, in.Resume)
		} else {
			e = s.backend.Cancel(in.Alias)
		}
		if e != nil {
			reject(w, 409)
			return
		}
		reply(w, 202, map[string]bool{"accepted": true})
	default:
		reject(w, 403)
	}
}
func validAlias(a string) bool {
	if len(a) < 1 || len(a) > 32 {
		return false
	}
	for _, c := range a {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if now.Before(s.retryAt) {
		reject(w, 429)
		return
	}
	hash := sha256.Sum256([]byte(in.Password))
	if subtle.ConstantTimeCompare(hash[:], s.password[:]) != 1 {
		s.failures++
		if s.failures >= 5 {
			s.retryAt = now.Add(time.Minute)
		}
		reject(w, 401)
		return
	}
	s.failures = 0
	id, e := nonce()
	if e != nil {
		reject(w, 503)
		return
	}
	csrf, e := nonce()
	if e != nil {
		reject(w, 503)
		return
	}
	if old, e := r.Cookie("ndg_session"); e == nil {
		delete(s.sessions, old.Value)
	}
	for id, x := range s.sessions {
		if now.Sub(x.touched) > s.cfg.IdleTimeout {
			delete(s.sessions, id)
		}
	}
	if len(s.sessions) >= 32 {
		reject(w, 429)
		return
	}
	s.sessions[id] = session{csrf: csrf, touched: now, window: now}
	http.SetCookie(w, &http.Cookie{Name: "ndg_session", Value: id, Path: "/", HttpOnly: true, Secure: s.cfg.Secure, SameSite: http.SameSiteStrictMode})
	reply(w, 200, map[string]string{"csrf": csrf})
}
