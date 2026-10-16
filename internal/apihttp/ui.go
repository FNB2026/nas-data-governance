package apihttp

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed webdist
var webFiles embed.FS

// ServeReadOnlyUI serves only the embedded public read-only UI. API callers
// must enforce their own routing and authentication boundary.
func ServeReadOnlyUI(w http.ResponseWriter, r *http.Request) {
	assets, _ := fs.Sub(webFiles, "webdist")
	http.FileServer(http.FS(assets)).ServeHTTP(w, r)
}
