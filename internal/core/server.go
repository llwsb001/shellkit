package core

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultAddr is where the web shell listens.
const DefaultAddr = "127.0.0.1:3427"

// EnvAddr overrides it (Docker, tests, a second instance).
const EnvAddr = "SHELLKIT_ADDR"

// Server is the HTTP face of the same core. The web page mounts on it; the
// CLI never needs it, which is the point of the shells being optional.
type Server struct {
	addr  string
	mux   *http.ServeMux
	start time.Time

	mu    sync.Mutex
	calls int
	last  *Action
}

// Action is the most recent thing a shell did, shown on the page.
type Action struct {
	At     time.Time `json:"at"`
	Kind   string    `json:"kind"`
	Detail string    `json:"detail"`
	Error  string    `json:"error,omitempty"`
}

// State is what /api/state returns.
type State struct {
	Addr     string    `json:"addr"`
	File     string    `json:"file"`
	Count    int       `json:"count"`
	Started  time.Time `json:"started"`
	Requests int       `json:"requests"`
	Last     *Action   `json:"last"`
	Error    string    `json:"error,omitempty"`
}

func New(addr string) *Server {
	s := &Server{addr: addr, mux: http.NewServeMux(), start: time.Now()}
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	s.mux.HandleFunc("GET /api/state", s.state)
	s.mux.HandleFunc("GET /api/notes", s.listNotes)
	s.mux.HandleFunc("POST /api/notes", s.addNote)
	s.mux.HandleFunc("DELETE /api/notes/{id}", s.removeNote)
	return s
}

func (s *Server) Addr() string { return s.addr }

// Handle lets the web page mount routes on the same port. Call it before
// ListenAndServe; the mux is not safe to change once serving.
func (s *Server) Handle(pattern string, h http.Handler) { s.mux.Handle(pattern, h) }

// HandleFunc is Handle for a plain function.
func (s *Server) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	s.mux.HandleFunc(pattern, h)
}

func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s.mux, ReadHeaderTimeout: 5 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return srv.Shutdown(shut)
	}
}

// Snapshot is the same data /api/state returns, for shells that draw it
// in-process (the TUI) instead of over HTTP.
type Snapshot struct {
	Addr     string
	File     string
	Count    int
	Err      string
	Started  time.Time
	Requests int
	Last     *Action
}

// Snapshot copies the counters and the note count.
func (s *Server) Snapshot() Snapshot {
	count, err := Count()
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := Snapshot{Addr: s.addr, File: File(), Count: count, Started: s.start, Requests: s.calls, Last: s.last}
	if err != nil {
		ss.Err = err.Error()
	}
	return ss
}

func (s *Server) state(w http.ResponseWriter, _ *http.Request) {
	count, err := Count()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	st := State{Addr: s.addr, File: File(), Count: count, Started: s.start}
	s.mu.Lock()
	st.Requests = s.calls
	st.Last = s.last
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) listNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "limit must be a non-negative integer"})
			return
		}
		limit = n
	}
	notes, err := List(q, limit)
	if err != nil {
		s.record("list", q, err)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.record("list", q, nil)
	writeJSON(w, http.StatusOK, map[string]any{"notes": notes, "count": len(notes)})
}

func (s *Server) addNote(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]any{"error": "send application/json"})
		return
	}
	var in struct {
		Text string   `json:"text"`
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad JSON: " + err.Error()})
		return
	}
	note, err := Add(in.Text, in.Tags)
	if err != nil {
		s.record("add", in.Text, err)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.record("add", note.Text, nil)
	writeJSON(w, http.StatusCreated, note)
}

func (s *Server) removeNote(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := Remove(id); err != nil {
		s.record("remove", id, err)
		code := http.StatusNotFound
		if !errors.Is(err, ErrNotFound) {
			code = http.StatusInternalServerError
		}
		writeJSON(w, code, map[string]any{"error": err.Error()})
		return
	}
	s.record("remove", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) record(kind, detail string, err error) {
	a := &Action{At: time.Now().UTC(), Kind: kind, Detail: truncate(detail, 200)}
	if err != nil {
		a.Error = err.Error()
	}
	s.mu.Lock()
	s.calls++
	s.last = a
	s.mu.Unlock()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
