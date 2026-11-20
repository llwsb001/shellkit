//go:build nogui

package ui

import (
	"net/http"

	"github.com/llwsb001/shellkit/internal/core"
)

// Mount in a nogui build: there is no page, and the server says so instead of
// answering with a blank 404.
func Mount(s *core.Server) {
	off := func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "this build has no page (-tags nogui); use the CLI or the API", http.StatusNotImplemented)
	}
	s.HandleFunc("GET /", off)
}
