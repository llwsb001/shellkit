// Package core is the whole tool: capture, search, delete. Every shell calls
// these functions or the HTTP API on top of them; nothing here knows a shell
// exists.
package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// storeMu serialises read-modify-write inside this process: the HTTP server
// handles requests concurrently, and two Adds racing on the same file would
// otherwise each read the old list and write back its own version.
// (Cross-process writers still rely on rename atomicity; last writer wins.)
var storeMu sync.Mutex

// ErrNotFound means there is no note with that id.
var ErrNotFound = errors.New("no note with that id")

// Note is one capture.
type Note struct {
	ID   string    `json:"id"`
	Text string    `json:"text"`
	Tags []string  `json:"tags,omitempty"`
	At   time.Time `json:"at"`
}

// Dir is where the tool keeps its one file.
func Dir() string {
	if d := os.Getenv("SHELLKIT_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".shellkit")
}

// File is the notes file.
func File() string { return filepath.Join(Dir(), "notes.json") }

// Load reads every note. A missing file is an empty list, not an error.
func Load() ([]Note, error) {
	b, err := os.ReadFile(File())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(b)) == "" {
		return nil, nil
	}
	var notes []Note
	if err := json.Unmarshal(b, &notes); err != nil {
		return nil, fmt.Errorf("%s: %w", File(), err)
	}
	return notes, nil
}

// Add captures one line. Text is required; tags are optional, lowercased and
// de-duplicated so search is not case-sensitive by accident.
func Add(text string, tags []string) (Note, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Note{}, fmt.Errorf("note text is empty")
	}
	if len([]rune(text)) > 5000 {
		return Note{}, fmt.Errorf("note is %d runes, limit is 5000", len([]rune(text)))
	}
	storeMu.Lock()
	defer storeMu.Unlock()
	unlock, err := lockStore()
	if err != nil {
		return Note{}, err
	}
	defer unlock()
	notes, err := Load()
	if err != nil {
		return Note{}, err
	}
	note := Note{ID: newID(), Text: text, Tags: cleanTags(tags), At: time.Now().UTC()}
	notes = append(notes, note)
	if err := save(notes); err != nil {
		return Note{}, err
	}
	return note, nil
}

// List returns notes newest first, filtered by a case-insensitive substring
// over text and tags. limit <= 0 means no limit.
func List(query string, limit int) ([]Note, error) {
	notes, err := Load()
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Note, 0, len(notes))
	for _, n := range notes {
		if q != "" && !match(n, q) {
			continue
		}
		out = append(out, n)
	}
	// Newest first by timestamp, not by file order: a hand-edited or merged
	// file still lists sensibly. Ties fall back to id, which sorts
	// lexicographically (generated ids share a fixed width).
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].At.Equal(out[j].At) {
			return out[i].ID > out[j].ID
		}
		return out[i].At.After(out[j].At)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func match(n Note, q string) bool {
	if strings.Contains(strings.ToLower(n.Text), q) {
		return true
	}
	for _, t := range n.Tags {
		if strings.Contains(strings.ToLower(t), q) {
			return true
		}
	}
	return false
}

// Remove deletes one note by id. An unknown id is an error, not a silent no-op.
func Remove(id string) error {
	storeMu.Lock()
	defer storeMu.Unlock()
	unlock, err := lockStore()
	if err != nil {
		return err
	}
	defer unlock()
	notes, err := Load()
	if err != nil {
		return err
	}
	kept := make([]Note, 0, len(notes))
	found := false
	for _, n := range notes {
		if n.ID == id {
			found = true
			continue
		}
		kept = append(kept, n)
	}
	if !found {
		return fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	return save(kept)
}

// Count is the number in the file, for /api/state.
func Count() (int, error) {
	notes, err := Load()
	if err != nil {
		return 0, err
	}
	return len(notes), nil
}

// save writes through a temp file and a rename: a crash never leaves half a
// file behind. Three short retries cover Windows holding the target open.
func save(notes []Note) error {
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(notes, "", "  ")
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(File()); err == nil {
		mode = fi.Mode().Perm() // do not turn a readable file into a 0600 one
	}
	tmp, err := os.CreateTemp(Dir(), ".notes-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(body, '\n')); err != nil {
		tmp.Close()
		return err
	}
	// Sync before rename: a rename is atomic against a crash, but not against
	// power loss, and a repaired file beats a truncated one.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	var renameErr error
	for attempt := 0; attempt < 3; attempt++ {
		if renameErr = os.Rename(tmp.Name(), File()); renameErr == nil {
			_ = os.Chmod(File(), mode)
			syncDir(Dir())
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return renameErr
}

// syncDir makes the rename itself durable. Best effort: not every filesystem
// lets a directory be fsynced.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

func cleanTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] || len([]rune(t)) > 32 {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) == 8 {
			break // a cap keeps a hostile API caller from growing the file forever
		}
	}
	sort.Strings(out)
	return out
}

var idSeq atomic.Uint64

func newID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// No entropy at all is exotic; a process-local counter still keeps
		// ids unique within this run.
		n := idSeq.Add(1)
		return fmt.Sprintf("%x-%04x", time.Now().UTC().UnixNano()/1e6, n)
	}
	return fmt.Sprintf("%x-%s", time.Now().UTC().UnixNano()/1e6, hex.EncodeToString(b[:]))
}
