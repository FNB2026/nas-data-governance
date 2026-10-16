package fnosgateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixture(t *testing.T) *Gateway {
	t.Helper()
	g, e := New(Config{AppName: "ndg-test", Origin: "https://nas.example:5667", Upstream: "http://10.98.45.2:8080", PackageUser: "ndgtest", PackageGroup: "ndgtest"})
	if e != nil {
		t.Fatal(e)
	}
	return g
}
func request(g *Gateway, method, route, body string) *http.Request {
	r := httptest.NewRequest(method, g.cfg.Origin+g.prefix+strings.TrimPrefix(route, "/"), strings.NewReader(body))
	if method == "POST" {
		r.Header.Set("Origin", g.cfg.Origin)
		r.Header.Set("Content-Type", "application/json")
	}
	return r
}
func TestBoundaryRejectsBeforeUpstream(t *testing.T) {
	for _, scenario := range []string{"write", "no_origin", "foreign_origin", "wrong_host", "plaintext", "query", "escape", "large_body"} {
		t.Run(scenario, func(t *testing.T) {
			g := fixture(t)
			called := false
			g.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { called = true; return nil, ErrRejected })
			r := request(g, "POST", "api/status", `{"alias":"source_a"}`)
			switch scenario {
			case "write":
				r.URL.Path = g.prefix + "api/purge/execute"
			case "no_origin":
				r.Header.Del("Origin")
			case "foreign_origin":
				r.Header.Set("Origin", "https://evil.example")
			case "wrong_host":
				r.Host = "attacker.example"
			case "plaintext":
				r.TLS = nil
			case "query":
				r.URL.RawQuery = "private=synthetic"
			case "escape":
				r.URL.Path = g.prefix + "../api/status"
			case "large_body":
				r.Body = io.NopCloser(strings.NewReader(strings.Repeat("x", 4097)))
			}
			w := httptest.NewRecorder()
			g.ServeHTTP(w, r)
			if w.Code < 400 || called || strings.Contains(w.Body.String(), "synthetic") {
				t.Fatalf("status=%d called=%v", w.Code, called)
			}
		})
	}
}
func TestCookieAndHeaderIsolation(t *testing.T) {
	g := fixture(t)
	token := strings.Repeat("a", 64)
	g.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != g.cfg.Upstream+"/api/login" || r.Host != "nas.example:5667" {
			t.Fatal("destination changed")
		}
		cookies := r.Cookies()
		if len(cookies) != 1 || cookies[0].Name != "ndg_session" || cookies[0].Value != token {
			t.Fatal("cookie boundary")
		}
		for _, h := range []string{"Authorization", "X-Trim-User", "Forwarded", "X-Forwarded-Host", "X-Arbitrary"} {
			if r.Header.Get(h) != "" {
				t.Fatal("private header forwarded", h)
			}
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}, "Set-Cookie": {"ndg_session=" + token + "; Domain=evil.example; Path=/"}}, Body: io.NopCloser(strings.NewReader(`{"csrf":"synthetic"}`))}, nil
	})
	r := request(g, "POST", "api/login", `{"password":"synthetic"}`)
	r.AddCookie(&http.Cookie{Name: "fnos_session", Value: "synthetic_nas_secret"})
	r.AddCookie(&http.Cookie{Name: g.cookie, Value: token})
	for _, h := range []string{"Authorization", "X-Trim-User", "Forwarded", "X-Forwarded-Host", "X-Arbitrary"} {
		r.Header.Set(h, "synthetic_nas_secret")
	}
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	c := w.Result().Cookies()
	if len(c) != 1 || c[0].Name != g.cookie || c[0].Domain != "" || c[0].Path != g.prefix || !c[0].Secure || !c[0].HttpOnly || c[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("external cookie policy", c)
	}
}
func TestAmbiguousCookieAndBackendErrors(t *testing.T) {
	g := fixture(t)
	called := false
	g.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 409, Body: io.NopCloser(strings.NewReader("/synthetic/private/source"))}, nil
	})
	r := request(g, "POST", "api/status", `{}`)
	r.AddCookie(&http.Cookie{Name: g.cookie, Value: strings.Repeat("a", 64)})
	r.AddCookie(&http.Cookie{Name: g.cookie, Value: strings.Repeat("b", 64)})
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 403 || called {
		t.Fatal("ambiguous cookie accepted")
	}
	w = httptest.NewRecorder()
	g.ServeHTTP(w, request(g, "POST", "api/status", `{}`))
	if w.Code != 409 || strings.Contains(w.Body.String(), "private") {
		t.Fatal("raw upstream error")
	}
}
func TestBackendRedirectAndOversizeAreRefused(t *testing.T) {
	for _, code := range []int{302, 200} {
		g := fixture(t)
		g.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {"application/json"}, "Location": {"https://evil.example"}}, Body: io.NopCloser(strings.NewReader(strings.Repeat(" ", 262145)))}, nil
		})
		w := httptest.NewRecorder()
		g.ServeHTTP(w, request(g, "GET", "api/sources", ""))
		if w.Code != 502 || w.Header().Get("Location") != "" {
			t.Fatal("unbounded response")
		}
	}
}
func TestPublicUIAndOriginConfig(t *testing.T) {
	g := fixture(t)
	if g.client.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("environment proxy enabled")
	}
	w := httptest.NewRecorder()
	g.ServeHTTP(w, request(g, "GET", "/", ""))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "NDG") {
		t.Fatal("missing public UI")
	}
	for _, upstream := range []string{"http://127.0.0.1:8080", "http://example.com:8080", "http://10.98.45.2:8080/api", "http://10.98.45.2:9090", "https://10.98.45.2:8080"} {
		c := g.cfg
		c.Upstream = upstream
		if _, e := New(c); e == nil {
			t.Fatal("unsafe upstream accepted", upstream)
		}
	}
}
