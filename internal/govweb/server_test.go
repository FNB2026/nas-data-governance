package govweb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// No credential-shaped literal lives in this repository: a source string that
// looks like a token is exactly what the secret scanner is meant to reject,
// and an embedded one would also make the shape checks vacuously pass.
func validToken(t *testing.T) string {
	t.Helper()
	token, err := RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func writeCredential(t *testing.T, token string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gov.credential")
	if err := os.WriteFile(path, []byte(token+"\n"), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func loopbackConfig(t *testing.T) Config {
	t.Helper()
	return Config{Addr: "127.0.0.1:8443", CredentialPath: writeCredential(t, validToken(t), 0o600),
		RequireTLS: true, Backing: ClosedBacking{}}
}

func mustServer(t *testing.T, cfg Config) *Server {
	t.Helper()
	server, err := New(cfg)
	if err != nil {
		t.Fatalf("expected the loopback deployment to be accepted, got %v", err)
	}
	return server
}

// newServer builds a server whose bearer token is known only to this test run.
func newServer(t *testing.T) (*Server, string) {
	t.Helper()
	token := validToken(t)
	cfg := Config{Addr: "127.0.0.1:8443", CredentialPath: writeCredential(t, token, 0o600),
		RequireTLS: true, Backing: ClosedBacking{}}
	return mustServer(t, cfg), token
}

func call(t *testing.T, server *Server, request *httptest.ResponseRecorder, method, target, body, token string) (int, map[string]interface{}) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	server.Handler().ServeHTTP(request, req)
	payload := map[string]interface{}{}
	_ = json.Unmarshal(request.Body.Bytes(), &payload)
	return request.Code, payload
}

func TestDeploymentBoundaryRefusals(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*Config)
		contains string
	}{
		{"no_credential", func(c *Config) { c.CredentialPath = "" }, "govweb_credential_path_required"},
		{"credential_missing", func(c *Config) { c.CredentialPath = filepath.Join(t.TempDir(), "gone") },
			"govweb_credential_unreadable"},
		{"credential_world_readable", func(c *Config) {
			c.CredentialPath = writeCredential(t, validToken(t), 0o644)
		}, "govweb_credential_mode_must_be_0600"},
		{"token_shape", func(c *Config) {
			c.CredentialPath = writeCredential(t, "hunter2", 0o600)
		}, "govweb_credential_token_shape_rejected"},
		{"token_too_short", func(c *Config) {
			c.CredentialPath = writeCredential(t, "ndg-gov-short", 0o600)
		}, "govweb_credential_token_shape_rejected"},
		{"wildcard_listen", func(c *Config) { c.Addr = "0.0.0.0:8443" }, "govweb_wildcard_listen_refused"},
		{"lan_without_allowlist", func(c *Config) { c.Addr = "10.0.0.9:8443" }, "govweb_lan_allowlist_required"},
		{"lan_outside_allowlist", func(c *Config) {
			c.Addr = "10.9.0.9:8443"
			c.LANAllowlist = []string{"10.8.0.0/24"}
		}, "govweb_listen_address_outside_allowlist"},
		{"malformed_allowlist", func(c *Config) {
			c.Addr = "10.8.0.9:8443"
			c.LANAllowlist = []string{"10.8.0.0"}
		}, "govweb_lan_allowlist_entry_invalid"},
		{"plaintext_off_loopback", func(c *Config) {
			c.Addr = "10.0.0.9:8443"
			c.LANAllowlist = []string{"10.0.0.0/24"}
			c.RequireTLS = false
		}, "govweb_plaintext_refused_off_loopback"},
		{"no_backing", func(c *Config) { c.Backing = nil }, "govweb_backing_required"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			cfg := loopbackConfig(t)
			item.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected %s to be refused", item.name)
			}
			if !strings.Contains(err.Error(), item.contains) {
				t.Fatalf("expected %q, got %v", item.contains, err)
			}
		})
	}
}

func TestCredentialMustStayPrivateAtUseTime(t *testing.T) {
	cfg := loopbackConfig(t)
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(cfg.CredentialPath, 0o600)
	if err := os.Chmod(cfg.CredentialPath, 0o644); err != nil {
		t.Fatal(err)
	}
	// Nothing may build from a credential that is no longer private, even if
	// the same config validated moments ago.
	if _, err := New(cfg); err == nil || !strings.Contains(err.Error(), "govweb_credential_mode_must_be_0600") {
		t.Fatalf("a loosened credential must still be refused at New, got %v", err)
	}
}

func TestLoopbackLANWithAllowlistAndTLSIsAccepted(t *testing.T) {
	cfg := loopbackConfig(t)
	cfg.Addr = "10.0.0.9:8443"
	cfg.LANAllowlist = []string{"10.0.0.0/24"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("explicit LAN allowlist with TLS should be accepted, got %v", err)
	}
}

func TestAuthenticationIsRequiredAndNotCookieBased(t *testing.T) {
	server := mustServer(t, loopbackConfig(t))
	for _, endpoint := range []string{"/gov/capabilities", "/gov/pending", "/gov/journal", "/gov/intent"} {
		recorder := httptest.NewRecorder()
		code, payload := call(t, server, recorder, http.MethodGet, endpoint, "", "")
		if code != http.StatusUnauthorized || payload["reason"] != "govweb_unauthenticated" {
			t.Fatalf("%s without a bearer: got %d %v", endpoint, code, payload)
		}
		if recorder.Result().Header.Get("Set-Cookie") != "" {
			t.Fatalf("%s set a cookie; this surface is bearer-only", endpoint)
		}
	}
	server2 := mustServer(t, loopbackConfig(t))
	var last int
	for attempt := 0; attempt < 6; attempt++ {
		recorder := httptest.NewRecorder()
		last, _ = call(t, server2, recorder, http.MethodGet, "/gov/capabilities", "", "wrong-token")
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("after 5 failures the surface must answer 429, got %d", last)
	}
}

func TestReadEndpointsReportDisabledCapabilities(t *testing.T) {
	server, token := newServer(t)
	recorder := httptest.NewRecorder()
	code, payload := call(t, server, recorder, http.MethodGet, "/gov/capabilities", "", token)
	if code != http.StatusOK || payload["result"] != "CAPABILITIES" {
		t.Fatal(code, payload)
	}
	list, ok := payload["capabilities"].([]interface{})
	if !ok || len(list) == 0 {
		t.Fatalf("capabilities must be listed, got %v", payload)
	}
	for _, entry := range list {
		field, ok := entry.(map[string]interface{})
		if !ok {
			t.Fatalf("capability shape wrong: %v", entry)
		}
		if field["enabled"] != false {
			t.Fatalf("%v must not be advertised as enabled by default", field["name"])
		}
		if reason, _ := field["reason"].(string); reason == "" {
			t.Fatalf("%v must explain why it is disabled", field["name"])
		}
	}
}

func TestEveryWriteIntentIsATypedRefusal(t *testing.T) {
	cases := map[string]string{
		"decide":     "govweb_decision_writer_not_enabled",
		"approve":    "govweb_approval_gate_not_enabled",
		"quarantine": "govweb_quarantine_executor_not_enabled",
		"restore":    "govweb_restore_executor_not_enabled",
		"purge":      "govweb_purge_release_disabled",
	}
	for kind, reason := range cases {
		server, token := newServer(t)
		body := `{"kind":"` + kind + `","objects":["obj-1"],"note":"operator asked for this"}`
		code, payload := call(t, server, httptest.NewRecorder(), http.MethodPost, "/gov/intent", body, token)
		if code != http.StatusForbidden || payload["reason"] != reason {
			t.Fatalf("%s: expected 403 %s, got %d %v", kind, reason, code, payload)
		}
	}
	server, token := newServer(t)
	if code, payload := call(t, server, httptest.NewRecorder(), http.MethodPost, "/gov/intent",
		`{"kind":"teleport"}`, token); code != http.StatusBadRequest ||
		payload["reason"] != "govweb_intent_kind_unknown" {
		t.Fatalf("unknown intent must be refused by kind, got %d %v", code, payload)
	}
	if code, payload := call(t, server, httptest.NewRecorder(), http.MethodPost, "/gov/intent",
		`{"kind":`, token); code != http.StatusBadRequest ||
		payload["reason"] != "govweb_intent_unreadable" {
		t.Fatalf("unreadable intent must be refused, got %d %v", code, payload)
	}
	if code, _ := call(t, server, httptest.NewRecorder(), http.MethodDelete, "/gov/capabilities", "",
		token); code != http.StatusMethodNotAllowed {
		t.Fatalf("DELETE must not be served, got %d", code)
	}
}

func TestRefusedIntentCreatesNoWork(t *testing.T) {
	server, token := newServer(t)
	for _, kind := range []string{"decide", "approve", "quarantine", "restore", "purge"} {
		call(t, server, httptest.NewRecorder(), http.MethodPost, "/gov/intent",
			`{"kind":"`+kind+`","objects":["obj-1"]}`, token)
	}
	for _, endpoint := range []string{"/gov/pending", "/gov/journal"} {
		code, payload := call(t, server, httptest.NewRecorder(), http.MethodGet, endpoint, "", token)
		if code != http.StatusOK {
			t.Fatal(endpoint, code, payload)
		}
		for _, key := range []string{"pending", "journal"} {
			if body, ok := payload[key].(map[string]interface{}); ok && len(body) != 0 {
				t.Fatalf("%s produced %v after refusals; a refusal must leave nothing behind", endpoint, body)
			}
		}
	}
}
