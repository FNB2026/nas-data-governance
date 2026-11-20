package govweb

import (
	_ "embed"
	"net/http"
)

//go:embed purge_console.html
var purgeConsoleHTML []byte

//go:embed purge_console.js
var purgeConsoleJS []byte

func (s *Server) handlePurgeConsole(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.TLS == nil || s.quarantinePurge == nil || s.sourcePurge == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; connect-src 'self'; script-src 'self'; frame-ancestors 'none'; form-action 'none'; base-uri 'none'")
	if r.URL.Path == "/gov/purge-console.js" {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(purgeConsoleJS)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(purgeConsoleHTML)
}
