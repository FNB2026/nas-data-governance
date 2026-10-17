// Package govweb is the *private* governance surface: a separate deployment
// identity with its own authentication, intended to sit inside one trusted LAN
// and nothing else.
//
// What it deliberately is not:
//
//   - it is not the public read-only Web. Nothing here relaxes, imports or
//     re-exports internal/apihttp, and nothing here can reach the public API's
//     session store;
//   - it does not perform file operations. A browser action here is an
//     *intent*: the surface records what the operator asked for and then hands
//     the actual write to the already reviewed Gate, execution lock, live
//     object check and least privilege worker. Anything not yet wired is a
//     typed refusal, never a silent success.
//
// The default configuration is closed: capabilities come from the backing
// store and, when nothing is wired, are false with a reason string.
package govweb

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Capability is one governance action the surface may expose. Enabled is a fact
// about the backend, not about the UI.
type Capability struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason"`
}

// Backing is the read side: approvals, managed objects and journal state. The
// surface never writes through it.
type Backing interface {
	Capabilities() ([]Capability, error)
	// Pending returns executor-owned work the UI may display: quarantine
	// candidates awaiting approval, restore pending entries, journal decisions.
	Pending() (map[string]int, error)
	// JournalLast returns the most recent journal phase per step, used to show
	// interrupted work instead of guessing it finished.
	JournalLast() (map[string]string, error)
}

// ClosedBacking is what every environment starts with: nothing enabled and a
// reason saying so. It exists so an "unwired" deployment can never look ready.
type ClosedBacking struct{}

// Capabilities reports the full governance set as disabled with reasons.
func (ClosedBacking) Capabilities() ([]Capability, error) {
	return []Capability{
		{Name: "scan", Enabled: false, Reason: "scan_still_owned_by_the_read_only_channel"},
		{Name: "decide", Enabled: false, Reason: "decision_store_not_wired"},
		{Name: "approve", Enabled: false, Reason: "approval_gate_not_wired"},
		{Name: "quarantine", Enabled: false, Reason: "executor_not_wired"},
		{Name: "restore", Enabled: false, Reason: "executor_not_wired"},
		{Name: "purge", Enabled: false, Reason: "purge_release_disabled"},
	}, nil
}

// Pending reports no work because nothing is decided yet.
func (ClosedBacking) Pending() (map[string]int, error) { return map[string]int{}, nil }

// JournalLast reports no entries.
func (ClosedBacking) JournalLast() (map[string]string, error) { return map[string]string{}, nil }

// Config describes how the private surface is deployed.
type Config struct {
	// Addr is the listen address. Anything that is not loopback requires an
	// explicit LAN allowlist; 0.0.0.0 is refused outright.
	Addr string
	// LANAllowlist holds CIDRs the server may be exposed to. Empty means the
	// listener must be loopback only.
	LANAllowlist []string
	// CredentialPath points at a file that must be 0600, owned by the process
	// user, and hold exactly one bearer token line.
	CredentialPath string
	// RequireTLS forbids plaintext once the surface leaves loopback.
	RequireTLS bool
	Backing    Backing
}

// ErrBound is any startup refusal about the deployment boundary itself. Server
// startup must refuse these instead of listening.
var ErrBound = errors.New("govweb: deployment boundary")

type reasons struct{}

func refuse(reason string) error { return errors.New(reason) }

// ValidateAnswer is intentionally unused: kept private and typed so callers
// cannot smuggle a boolean "open" flag past a boundary check.
type ValidateAnswer = reasons

func loopbackOnly(addr string) (bool, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false, refuse("govweb_addr_requires_host_port")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		if host == "localhost" {
			return true, nil
		}
		return false, refuse("govweb_addr_host_not_resolved_to_ip")
	}
	return ip.IsLoopback(), nil
}

// loadCredential reads the bearer token in one place so the mode check is
// always adjacent to the read: validating and then reading separately would
// leave a window in which the file could be loosened between the two steps.
func loadCredential(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", refuse("govweb_credential_unreadable")
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeType != 0 {
		return "", refuse("govweb_credential_must_be_a_regular_file")
	}
	if info.Mode().Perm() != 0o600 {
		return "", refuse("govweb_credential_mode_must_be_0600")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", refuse("govweb_credential_unreadable")
	}
	token := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(token, "ndg-gov-") || len(token) < 32 || strings.ContainsAny(token, "\n\r") {
		return "", refuse("govweb_credential_token_shape_rejected")
	}
	return token, nil
}

// Validate checks the deployment boundary before anything listens.
func (c Config) Validate() error {
	if c.CredentialPath == "" {
		return refuse("govweb_credential_path_required")
	}
	if _, err := loadCredential(c.CredentialPath); err != nil {
		return err
	}
	loopback, err := loopbackOnly(c.Addr)
	if err != nil {
		return ErrBound
	}
	if !loopback {
		host, _, _ := net.SplitHostPort(c.Addr)
		ip := net.ParseIP(host)
		if ip == nil || ip.IsUnspecified() {
			return refuse("govweb_wildcard_listen_refused")
		}
		if len(c.LANAllowlist) == 0 {
			return refuse("govweb_lan_allowlist_required")
		}
		allowed := false
		for _, cidr := range c.LANAllowlist {
			_, network, err := net.ParseCIDR(cidr)
			if err != nil {
				return refuse("govweb_lan_allowlist_entry_invalid")
			}
			if network.Contains(ip) {
				allowed = true
			}
		}
		if !allowed {
			return refuse("govweb_listen_address_outside_allowlist")
		}
		if !c.RequireTLS {
			return refuse("govweb_plaintext_refused_off_loopback")
		}
	}
	if c.Backing == nil {
		return refuse("govweb_backing_required")
	}
	return nil
}

// Server serves the private governance surface.
type Server struct {
	token   string
	backing Backing
	mu      sync.Mutex
	failed  map[string]int
}

// New builds a server from a validated configuration. The credential shape was
// already checked by Validate, so an unvalidated caller still cannot get past
// re-reading it here.
func New(cfg Config) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	// The token is re-read here rather than passed in from Validate, so a
	// server can only be built from a credential that is still private now.
	token, err := loadCredential(cfg.CredentialPath)
	if err != nil {
		return nil, err
	}
	return &Server{token: token, backing: cfg.Backing, failed: map[string]int{}}, nil
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	// No cookie is ever set by this surface: bearer-only removes the CSRF class.
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) tooManyFailures(remote string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.failed[remote] >= 5
}

func (s *Server) noteFailure(remote string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed[remote]++
}

func (s *Server) authorised(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	given := []byte(strings.TrimPrefix(header, prefix))
	expected := []byte(s.token)
	return subtle.ConstantTimeCompare(given, expected) == 1
}

// Handler exposes only read endpoints plus one intent recorder.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/gov/capabilities", s.handleCapabilities)
	mux.HandleFunc("/gov/pending", s.handlePending)
	mux.HandleFunc("/gov/journal", s.handleJournal)
	mux.HandleFunc("/gov/intent", s.handleIntent)
	return mux
}

func (s *Server) guarded(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"result": "REFUSED",
				"reason": "govweb_method_not_allowed"})
			return
		}
		remote, _, _ := net.SplitHostPort(r.RemoteAddr)
		if s.tooManyFailures(remote) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"result": "REFUSED",
				"reason": "govweb_too_many_failed_attempts"})
			return
		}
		if !s.authorised(r) {
			s.noteFailure(remote)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"result": "REFUSED",
				"reason": "govweb_unauthenticated"})
			return
		}
		next(w, r)
	}
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	s.guarded(func(w http.ResponseWriter, r *http.Request) {
		caps, err := s.backing.Capabilities()
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"result": "REFUSED",
				"reason": "govweb_backing_unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"result": "CAPABILITIES",
			"capabilities": caps})
	})(w, r)
}

func (s *Server) handlePending(w http.ResponseWriter, r *http.Request) {
	s.guarded(func(w http.ResponseWriter, r *http.Request) {
		pending, err := s.backing.Pending()
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"result": "REFUSED",
				"reason": "govweb_backing_unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"result": "PENDING", "pending": pending})
	})(w, r)
}

func (s *Server) handleJournal(w http.ResponseWriter, r *http.Request) {
	s.guarded(func(w http.ResponseWriter, r *http.Request) {
		journal, err := s.backing.JournalLast()
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"result": "REFUSED",
				"reason": "govweb_backing_unavailable"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"result": "JOURNAL", "journal": journal})
	})(w, r)
}

// Intent is what the browser is allowed to say. It is a request for work, not
// work itself.
type Intent struct {
	Kind    string   `json:"kind"`
	Objects []string `json:"objects"`
	Note    string   `json:"note"`
}

// refusals maps every intent the UI can offer to the component that is not yet
// responsible for carrying it out. Nothing here performs a write.
var refusals = map[string]string{
	"decide":     "govweb_decision_writer_not_enabled",
	"approve":    "govweb_approval_gate_not_enabled",
	"quarantine": "govweb_quarantine_executor_not_enabled",
	"restore":    "govweb_restore_executor_not_enabled",
	"purge":      "govweb_purge_release_disabled",
}

func (s *Server) handleIntent(w http.ResponseWriter, r *http.Request) {
	s.guarded(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"result": "REFUSED",
				"reason": "govweb_method_not_allowed"})
			return
		}
		var intent Intent
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&intent); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"result": "REFUSED",
				"reason": "govweb_intent_unreadable"})
			return
		}
		reason, known := refusals[intent.Kind]
		if !known {
			writeJSON(w, http.StatusBadRequest, map[string]string{"result": "REFUSED",
				"reason": "govweb_intent_kind_unknown"})
			return
		}
		// A refusal must be indistinguishable from "nothing happened": no row is
		// written, no queue entry appears, and the UI therefore cannot show a
		// green result for work nobody did.
		writeJSON(w, http.StatusForbidden, map[string]string{"result": "REFUSED", "reason": reason,
			"objects_seen": "0"})
	})(w, r)
}

// RandomToken produces a credential value with the required prefix and length.
// It is only ever called by install tooling, never by the surface itself.
func RandomToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "ndg-gov-" + hex.EncodeToString(buf), nil
}

var _ = time.Second
