package apihttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fake struct{ calls int }

func (f *fake) Sources() []Source { return []Source{{"source_a"}} }
func (f *fake) Snapshot(string) (Snapshot, error) {
	return Snapshot{State: "IDLE", Groups: []Group{}}, nil
}
func (f *fake) Start(string, bool) error {
	f.calls++
	return errors.New("synthetic_private_anchor/path must not appear")
}
func (f *fake) Cancel(string) error { f.calls++; return ErrDenied }

const password = "synthetic-credential-at-least-32-bytes"

func setup(t *testing.T) (*Server, *fake) {
	t.Helper()
	b := &fake{}
	s, e := New(Config{Origin: "http://localhost:8080", Password: password}, b)
	if e != nil {
		t.Fatal(e)
	}
	return s, b
}
func req(s *Server, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost:8080"+path, strings.NewReader(body))
	if method == "POST" {
		r.Header.Set("Origin", "http://localhost:8080")
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func login(t *testing.T, s *Server) (*http.Cookie, string) {
	w := req(s, "POST", "/api/login", `{"password":"`+password+`"}`, nil, "")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	var v map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &v)
	return w.Result().Cookies()[0], v["csrf"]
}
func TestAuthenticationCSRFAndExecutionBoundary(t *testing.T) {
	s, b := setup(t)
	if w := req(s, "POST", "/api/scan", `{"alias":"source_a"}`, nil, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	c, csrf := login(t, s)
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Secure {
		t.Fatal("cookie policy")
	}
	if w := req(s, "POST", "/api/scan", `{"alias":"source_a"}`, c, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, p := range []string{"/api/quarantine/create", "/api/quarantine/approve", "/api/quarantine/dry-run", "/api/quarantine/execute", "/api/restore/execute", "/api/purge/execute", "/api/execute", "/api/recover"} {
		if w := req(s, "POST", p, `{}`, c, csrf); w.Code != 403 {
			t.Fatal(p, w.Code)
		}
	}
	if b.calls != 0 {
		t.Fatal("unsafe backend called")
	}
	for _, body := range []string{`{"alias":"../private"}`, `{"alias":"source_a","root":"/synthetic-private-anchor"}`, `{"alias":"source_a"} {}`, strings.Repeat("x", 5000)} {
		w := req(s, "POST", "/api/scan", body, c, csrf)
		if w.Code != 400 || strings.Contains(w.Body.String(), "private-anchor") {
			t.Fatal(w.Code, w.Body)
		}
	}
	if b.calls != 0 {
		t.Fatal("request boundary called backend")
	}
}
func TestRebindingAndOriginRejected(t *testing.T) {
	s, _ := setup(t)
	c, csrf := login(t, s)
	for _, tc := range []struct{ host, origin, site string }{{"evil.example", "http://localhost:8080", ""}, {"localhost:8080", "http://evil.example", ""}, {"localhost:8080", "", ""}, {"localhost:8080", "http://localhost:8080", "cross-site"}} {
		r := httptest.NewRequest("POST", "http://localhost:8080/api/scan", strings.NewReader(`{"alias":"source_a"}`))
		r.Host = tc.host
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Sec-Fetch-Site", tc.site)
		r.AddCookie(c)
		r.Header.Set("X-CSRF-Token", csrf)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(tc, w.Code)
		}
	}
	if req(s, "GET", "/healthz?secret=private-anchor", "", nil, "").Code != 403 {
		t.Fatal("query accepted")
	}
}
func TestExpiryLogoutAndThrottle(t *testing.T) {
	s, _ := setup(t)
	now := time.Now()
	s.now = func() time.Time { return now }
	c, _ := login(t, s)
	now = now.Add(16 * time.Minute)
	if req(s, "GET", "/api/sources", "", c, "").Code != 401 {
		t.Fatal("expired session accepted")
	}
	c, csrf := login(t, s)
	if req(s, "POST", "/api/logout", `{}`, c, csrf).Code != 200 || req(s, "GET", "/api/sources", "", c, "").Code != 401 {
		t.Fatal("logout")
	}
	for i := 0; i < 5; i++ {
		if req(s, "POST", "/api/login", `{"password":"wrong"}`, nil, "").Code != 401 {
			t.Fatal("bad credential")
		}
	}
	if req(s, "POST", "/api/login", `{"password":"`+password+`"}`, nil, "").Code != 429 {
		t.Fatal("no throttle")
	}
}
func TestNoPrivateErrorsOrCaching(t *testing.T) {
	s, _ := setup(t)
	c, csrf := login(t, s)
	w := req(s, "POST", "/api/scan", `{"alias":"source_a"}`, c, csrf)
	if w.Code != 409 || w.Body.String() != "{\"error\":\"request_rejected\"}\n" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, origin := range []string{"http://u:p@localhost", "http://localhost/path", "ftp://localhost", "http://localhost?x=1"} {
		if _, e := New(Config{Origin: origin, Password: password}, &fake{}); e == nil {
			t.Fatal(origin)
		}
	}
}

func TestSessionRateBoundAndSecureCookie(t *testing.T) {
	s, _ := setup(t)
	c, _ := login(t, s)
	for i := 0; i < 120; i++ {
		if w := req(s, "GET", "/api/sources", "", c, ""); w.Code != 200 {
			t.Fatal(i, w.Code)
		}
	}
	if w := req(s, "GET", "/api/sources", "", c, ""); w.Code != 429 {
		t.Fatal("unbounded requests")
	}
	secure, e := New(Config{Origin: "https://localhost:8080", Password: password, Secure: true}, &fake{})
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest("POST", "https://localhost:8080/api/login", strings.NewReader(`{"password":"`+password+`"}`))
	r.Header.Set("Origin", "https://localhost:8080")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	secure.ServeHTTP(w, r)
	if w.Code != 200 || !w.Result().Cookies()[0].Secure {
		t.Fatal("secure cookie")
	}
	if _, e := New(Config{Origin: "http://localhost:8080", Password: password}, nil); e == nil {
		t.Fatal("nil backend")
	}
}
