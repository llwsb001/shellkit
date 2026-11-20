// Package configs rewrites the model key in three real agent configs, in
// their own formats. JSON files are decoded and re-encoded (only the bytes of
// the document survive, not its layout); TOML files are edited line by line
// because there is no standard library parser here.
package configs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Target is one agent's config file and the key its model lives under.
type Target struct {
	Agent  string
	Rel    string   // path under the config home
	Path   []string // key path inside the file
	Format string   // json | toml
}

// Targets are the three formats this tool knows. Adding a fourth is data,
// not a new branch: path + key path + format.
var Targets = []Target{
	{Agent: "claude", Rel: ".claude/settings.json", Path: []string{"model"}, Format: "json"},
	{Agent: "codex", Rel: ".codex/config.toml", Path: []string{"model"}, Format: "toml"},
	{Agent: "gemini", Rel: ".gemini/settings.json", Path: []string{"model", "name"}, Format: "json"},
}

// ErrNoKey means the file exists but does not name a model. Get reports it so
// the caller can tell "empty file" from "wrong file".
var ErrNoKey = errors.New("no model key")

// Home is the directory the relative paths hang from. Tests point it at a
// temp dir; users get their real home.
func Home() string {
	if d := os.Getenv("SHELLKIT_CONFIG_HOME"); d != "" {
		return d
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

// Lookup finds a target by agent name.
func Lookup(agent string) (Target, error) {
	for _, t := range Targets {
		if t.Agent == agent {
			return t, nil
		}
	}
	return Target{}, fmt.Errorf("unknown agent %q (known: %s)", agent, strings.Join(Names(), ", "))
}

// Names lists the known agents, for help text and tests.
func Names() []string {
	out := make([]string, 0, len(Targets))
	for _, t := range Targets {
		out = append(out, t.Agent)
	}
	return out
}

// File is the absolute config path for a target.
func (t Target) File() string { return filepath.Join(Home(), filepath.FromSlash(t.Rel)) }

// Get reads the model the file currently names; a missing file is "" and
// ErrNoKey, because most people have never run that agent.
func (t Target) Get() (string, error) {
	b, err := os.ReadFile(t.File())
	if os.IsNotExist(err) {
		return "", ErrNoKey
	}
	if err != nil {
		return "", err
	}
	if t.Format == "toml" {
		return getTOML(string(b))
	}
	return getJSON(b, t.Path)
}

// Set writes the model in place. A missing file is created with just the key.
func (t Target) Set(model string) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("model is empty")
	}
	path := t.File()
	b, err := os.ReadFile(path)
	missing := os.IsNotExist(err)
	if err != nil && !missing {
		return err
	}

	var out []byte
	if t.Format == "toml" {
		out, err = setTOML(string(b), model)
	} else {
		out, err = setJSON(b, t.Path, model)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return writeAtomic(path, out)
}

// ---- JSON ----

func getJSON(b []byte, path []string) (string, error) {
	root, err := decodeJSON(b)
	if err != nil {
		return "", err
	}
	var node any = root
	for _, key := range path {
		m, ok := node.(map[string]any)
		if !ok {
			return "", fmt.Errorf("%w: %q is not inside an object", ErrNoKey, key)
		}
		node, ok = m[key]
		if !ok {
			return "", fmt.Errorf("%w: key %q missing", ErrNoKey, key)
		}
	}
	s, ok := node.(string)
	if !ok {
		return "", fmt.Errorf("%w: %q is not a string", ErrNoKey, strings.Join(path, "."))
	}
	return s, nil
}

func setJSON(b []byte, path []string, model string) ([]byte, error) {
	root, err := decodeJSON(b)
	if err != nil {
		return nil, err
	}
	node := root
	for _, key := range path[:len(path)-1] {
		next, ok := node[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			node[key] = next
		}
		node = next
	}
	node[path[len(path)-1]] = model

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // <, > and & are data, not markup
	enc.SetIndent("", "  ")
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decodeJSON keeps numbers as json.Number: decoding into float64 would turn
// 1234567890123456789 into …800 and silently corrupt a key we never touched.
// A file holding "null" decodes to an empty object so Set can fill it in.
func decodeJSON(b []byte) (map[string]any, error) {
	root := map[string]any{}
	if len(bytes.TrimSpace(b)) == 0 {
		return root, nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("not a JSON object: %w", err)
	}
	if raw == nil {
		return root, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("not a JSON object")
	}
	return m, nil
}

// ---- TOML ----
//
// Only the top-level area is touched: the first real [table] header ends it,
// so a profile's own model is left alone. Values may be basic ("…") or literal
// ('…') strings; multi-line strings and anything else are refused rather than
// guessed at.

func getTOML(text string) (string, error) {
	start, end, err := findTopLevelModelSpan(text)
	if err != nil {
		return "", err
	}
	return tomlUnquote(text[start:end])
}

func setTOML(text, model string) ([]byte, error) {
	quoted := tomlQuote(model)
	start, end, err := findTopLevelModelSpan(text)
	if err != nil && !errors.Is(err, ErrNoKey) {
		return nil, err
	}
	if err == nil {
		return []byte(text[:start] + quoted + text[end:]), nil
	}
	cut := topLevelEnd(text)
	line := "model = " + quoted + "\n"
	if cut == 0 {
		return []byte(line + text), nil
	}
	head := text[:cut]
	if !strings.HasSuffix(head, "\n") {
		head += "\n"
	}
	// Keep one blank line between the new key and a following [table].
	tail := text[cut:]
	if tail != "" && !strings.HasPrefix(tail, "\n") {
		tail = "\n" + tail
	}
	return []byte(head + line + tail), nil
}

// findTopLevelModelSpan returns the span of the top-level model value.
// ErrNoKey means there is none; any other error means there is one, but in a
// shape this tool refuses to edit (a multi-line string).
func findTopLevelModelSpan(text string) (int, int, error) {
	// A multi-line string can hide a `model =` line or a `[table]` header,
	// so a line scanner cannot tell structure from content. Refuse such
	// files instead of guessing; a hand edit is cheap, a corrupted config is not.
	if strings.Contains(text, `"""`) || strings.Contains(text, "'''") {
		return 0, 0, fmt.Errorf("file uses a multi-line string, which this tool will not edit")
	}
	end := topLevelEnd(text)
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		if offset >= end {
			return 0, 0, ErrNoKey
		}
		body := strings.TrimSuffix(line, "\n")
		trimmed := strings.TrimSpace(body)
		leading := len(body) - len(strings.TrimLeft(body, " \t"))
		key, raw, rhs, ok := splitTOMLAssignment(trimmed)
		if !ok && eqKey(trimmed) == "model" {
			return 0, 0, fmt.Errorf("model is not a single-line string; this tool refuses to guess where it ends")
		}
		if ok && key == "model" {
			vstart := offset + leading + rhs
			return vstart, vstart + len(raw), nil
		}
		offset += len(line)
	}
	return 0, 0, ErrNoKey
}

// topLevelEnd is the byte offset of the first real [table] header, or
// len(text). A line starting with "[" only counts at bracket depth zero, so
// the continuation of a multi-line array is not mistaken for a table.
func topLevelEnd(text string) int {
	offset, depth := 0, 0
	inStr, esc := false, false
	for _, line := range strings.SplitAfter(text, "\n") {
		body := strings.TrimSpace(line)
		if depth == 0 && strings.HasPrefix(body, "[") {
			return offset
		}
		for i := 0; i < len(line); i++ {
			c := line[i]
			if inStr {
				if esc {
					esc = false
				} else if c == '\\' {
					esc = true
				} else if c == '"' {
					inStr = false
				}
				continue
			}
			switch c {
			case '"':
				inStr = true
			case '[':
				depth++
			case ']':
				if depth > 0 {
					depth--
				}
			}
		}
		offset += len(line)
	}
	return len(text)
}

// eqKey is the (unquoted) key on the left of "=", used when the value itself
// is in a shape we refuse to edit.
func eqKey(line string) string {
	i := strings.Index(line, "=")
	if i < 0 {
		return ""
	}
	key := strings.TrimSpace(line[:i])
	if len(key) >= 2 && (key[0] == '"' || key[0] == '\'') && key[len(key)-1] == key[0] {
		return key[1 : len(key)-1]
	}
	return key
}

// splitTOMLAssignment splits `key = value`, including `"model" = "value"  # note`,
// and returns the raw value token plus its byte offset inside the line. A
// multi-line string is refused: guessing where it ends is how configs get
// corrupted.
func splitTOMLAssignment(line string) (string, string, int, bool) {
	eq := strings.Index(line, "=")
	if eq < 0 {
		return "", "", 0, false
	}
	key := strings.TrimSpace(line[:eq])
	if len(key) >= 2 && (key[0] == '"' || key[0] == '\'') && key[len(key)-1] == key[0] {
		key = key[1 : len(key)-1] // a quoted key names the same key
	}
	if key == "" || strings.ContainsAny(key, " \t\"'[]") {
		return "", "", 0, false
	}
	rest := line[eq+1:]
	rhs := len(rest) - len(strings.TrimLeft(rest, " \t"))
	value := rest[rhs:]
	if value == "" || (value[0] != '"' && value[0] != '\'') {
		return "", "", 0, false
	}
	if strings.HasPrefix(value, `"""`) || strings.HasPrefix(value, "'''") {
		return "", "", 0, false
	}
	q := value[0]
	end := -1
	for i := 1; i < len(value); i++ {
		if q == '"' && value[i] == '\\' {
			i++
			continue
		}
		if value[i] == q {
			end = i + 1
			break
		}
	}
	if end < 0 {
		return "", "", 0, false
	}
	return key, value[:end], eq + 1 + rhs, true
}

func tomlQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 0x20 {
				b.WriteString(fmt.Sprintf(`\u%04X`, c))
				continue
			}
			b.WriteByte(c) // bytes, so invalid UTF-8 is not silently replaced
		}
	}
	b.WriteByte('"')
	return b.String()
}

func tomlUnquote(raw string) (string, error) {
	if len(raw) < 2 {
		return "", fmt.Errorf("not a TOML string: %q", raw)
	}
	if raw[0] == '\'' {
		return raw[1 : len(raw)-1], nil
	}
	s, err := strconv.Unquote(raw) // a basic string is close enough to a JSON string
	if err != nil {
		return "", fmt.Errorf("bad TOML string: %w", err)
	}
	return s, nil
}

func writeAtomic(path string, text []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, ".cfg-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(text); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
