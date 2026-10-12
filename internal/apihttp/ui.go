package apihttp

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed webdist
var webFiles embed.FS

func serveUI(w http.ResponseWriter, r *http.Request) {
	assets, _ := fs.Sub(webFiles, "webdist")
	http.FileServer(http.FS(assets)).ServeHTTP(w, r)
}
