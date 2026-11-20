//go:build !nogui

// Package ui is the web shell: three static files, embedded into the binary
// and served on the same port as the API. No framework, no bundler.
package ui

import (
	"io/fs"
	"net/http"

	"github.com/llwsb001/shellkit/internal/core"
)

// Mount adds the page to the gateway. A -tags nogui build replaces this with
// a stub, so the page is absent rather than silently broken.
func Mount(s *core.Server) {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic("ui: embedded assets are broken: " + err.Error())
	}
	files := http.FileServer(http.FS(sub))
	s.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store") // prototype: always the file on disk
		if r.URL.Path == "/" {
			// Serve index.html directly: http.FileServer redirects
			// "/index.html" to "./", which turns a rewrite into a loop.
			b, err := fs.ReadFile(sub, "index.html")
			if err != nil {
				http.Error(w, "index.html missing", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(b)
			return
		}
		files.ServeHTTP(w, r)
	})
}
