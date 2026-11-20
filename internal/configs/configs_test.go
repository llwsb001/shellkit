package configs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sandbox(t *testing.T) {
	t.Helper()
	t.Setenv("SHELLKIT_CONFIG_HOME", t.TempDir())
}

func write(t *testing.T, rel, body string) {
	t.Helper()
	p := filepath.Join(Home(), filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(Home(), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestClaudeSettingsKeepEverythingElse(t *testing.T) {
	sandbox(t)
	write(t, ".claude/settings.json", `{
  "model": "opus",
  "permissions": {"allow": ["Bash(ls)"]},
  "env": {"ANTHROPIC_BASE_URL": "http://127.0.0.1:3425"}
}
`)
	tg, _ := Lookup("claude")
	if err := tg.Set("sonnet"); err != nil {
		t.Fatal(err)
	}
	got := read(t, ".claude/settings.json")
	if strings.Contains(got, `"opus"`) {
		t.Fatalf("old model survived:\n%s", got)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, got)
	}
	if doc["model"] != "sonnet" {
		t.Fatalf("model = %v", doc["model"])
	}
	if _, ok := doc["permissions"]; !ok {
		t.Fatalf("permissions dropped:\n%s", got)
	}
	if v, _ := tg.Get(); v != "sonnet" {
		t.Fatalf("Get disagrees with file: %q", v)
	}
}

// A nested "model" key must not be mistaken for the top-level one.
func TestNestedModelIsNotTheTarget(t *testing.T) {
	sandbox(t)
	write(t, ".claude/settings.json", `{"env":{"model":"nested"},"model":"opus"}`)
	tg, _ := Lookup("claude")
	if err := tg.Set("sonnet"); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(read(t, ".claude/settings.json")), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["model"] != "sonnet" {
		t.Fatalf("top-level model = %v", doc["model"])
	}
	if doc["env"].(map[string]any)["model"] != "nested" {
		t.Fatalf("nested model was rewritten: %v", doc["env"])
	}
}

// A dollar sign is data, not a regexp replacement.
func TestDollarSignSurvives(t *testing.T) {
	sandbox(t)
	write(t, ".claude/settings.json", `{"model":"opus"}`)
	tg, _ := Lookup("claude")
	if err := tg.Set("a$1b"); err != nil {
		t.Fatal(err)
	}
	if v := read(t, ".claude/settings.json"); strings.Contains(v, `"a"`) && !strings.Contains(v, "a$1b") {
		t.Fatalf("model was truncated at $:\n%s", v)
	}
}

// Escaped quotes in the old value must not produce invalid JSON.
func TestEscapedValueIsReplaced(t *testing.T) {
	sandbox(t)
	write(t, ".claude/settings.json", `{"model":"a\"b","x":1}`)
	tg, _ := Lookup("claude")
	if err := tg.Set("sonnet"); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(read(t, ".claude/settings.json")), &doc); err != nil {
		t.Fatalf("invalid JSON after rewrite: %v", err)
	}
	if doc["model"] != "sonnet" {
		t.Fatalf("model = %v", doc["model"])
	}
}

func TestCodexTOMLKeepsCommentsAndSiblings(t *testing.T) {
	sandbox(t)
	write(t, ".codex/config.toml", `# my codex config
model = "gpt-5"
model_reasoning_effort = "high"

[profiles.fast]
model = "gpt-4o-mini"
`)
	tg, _ := Lookup("codex")
	if err := tg.Set("kimi-k2"); err != nil {
		t.Fatal(err)
	}
	got := read(t, ".codex/config.toml")
	if strings.Contains(got, `"gpt-5"`) {
		t.Fatalf("old top-level model survived:\n%s", got)
	}
	if !strings.Contains(got, `model = "kimi-k2"`) ||
		!strings.Contains(got, "# my codex config") ||
		!strings.Contains(got, `model_reasoning_effort = "high"`) {
		t.Fatalf("siblings or comment damaged:\n%s", got)
	}
	if !strings.Contains(got, "[profiles.fast]\nmodel = \"gpt-4o-mini\"") {
		t.Fatalf("profile model should be untouched:\n%s", got)
	}
	if v, err := tg.Get(); err != nil || v != "kimi-k2" {
		t.Fatalf("Get = %q, %v", v, err)
	}
}

// A comment that looks like a table header is still a comment; the top-level
// area does not end there.
func TestCommentWithHeaderDoesNotEndTopLevel(t *testing.T) {
	sandbox(t)
	write(t, ".codex/config.toml", `# see [profiles.fast] below
model = "gpt-5"
`)
	tg, _ := Lookup("codex")
	if err := tg.Set("o3"); err != nil {
		t.Fatal(err)
	}
	got := read(t, ".codex/config.toml")
	if strings.Count(got, "model =") != 1 || !strings.Contains(got, `model = "o3"`) {
		t.Fatalf("expected one model line, got:\n%s", got)
	}
}

// Literal (single-quoted) TOML values are valid; replacing one must not add a
// second key.
func TestTOMLSingleQuotedValue(t *testing.T) {
	sandbox(t)
	write(t, ".codex/config.toml", "model = 'gpt-5'\n")
	tg, _ := Lookup("codex")
	if v, err := tg.Get(); err != nil || v != "gpt-5" {
		t.Fatalf("Get literal = %q, %v", v, err)
	}
	if err := tg.Set("o3"); err != nil {
		t.Fatal(err)
	}
	got := read(t, ".codex/config.toml")
	if strings.Count(got, "model") != 1 || !strings.Contains(got, `model = "o3"`) {
		t.Fatalf("duplicate key or wrong value:\n%s", got)
	}
}

// Get must read the same area Set writes: a profile model with no top-level
// model is "no key", not a value.
func TestTOMLGetIgnoresProfiles(t *testing.T) {
	sandbox(t)
	write(t, ".codex/config.toml", "[profiles.fast]\nmodel = \"gpt-4o-mini\"\n")
	tg, _ := Lookup("codex")
	if _, err := tg.Get(); !errors.Is(err, ErrNoKey) {
		t.Fatalf("want ErrNoKey, got %v", err)
	}
	if err := tg.Set("o3"); err != nil {
		t.Fatal(err)
	}
	got := read(t, ".codex/config.toml")
	if !strings.HasPrefix(got, "model = \"o3\"\n[profiles.fast]") &&
		!strings.Contains(got, "model = \"o3\"\n\n[profiles.fast]") {
		t.Fatalf("top-level model not above the first table:\n%s", got)
	}
	if !strings.Contains(got, `model = "gpt-4o-mini"`) {
		t.Fatalf("profile model damaged:\n%s", got)
	}
}

func TestGeminiNestedModelNameOnly(t *testing.T) {
	sandbox(t)
	write(t, ".gemini/settings.json", `{
  "model": {
    "name": "gemini-2.5-pro",
    "maxSessionTurns": 50
  },
  "mcpServers": {
    "name": "not-the-model"
  }
}
`)
	tg, _ := Lookup("gemini")
	if err := tg.Set("gemini-3-pro"); err != nil {
		t.Fatal(err)
	}
	got := read(t, ".gemini/settings.json")
	var doc map[string]any
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	model := doc["model"].(map[string]any)
	if model["name"] != "gemini-3-pro" {
		t.Fatalf("model.name = %v", model["name"])
	}
	if model["maxSessionTurns"] != float64(50) {
		t.Fatalf("sibling inside model damaged: %v", model)
	}
	if doc["mcpServers"].(map[string]any)["name"] != "not-the-model" {
		t.Fatalf("same-named key outside model was rewritten")
	}
	if strings.Contains(got, "gemini-2.5-pro") {
		t.Fatalf("old model survived:\n%s", got)
	}
}

func TestMissingFileIsCreated(t *testing.T) {
	sandbox(t)
	tg, _ := Lookup("gemini")
	if err := tg.Set("gemini-3-pro"); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(read(t, ".gemini/settings.json")), &doc); err != nil {
		t.Fatalf("created file is not JSON: %v", err)
	}
	model, ok := doc["model"].(map[string]any)
	if !ok || model["name"] != "gemini-3-pro" {
		t.Fatalf("created file has no model.name: %v", doc)
	}

	codex, _ := Lookup("codex")
	if err := codex.Set("o3"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, ".codex/config.toml"); got != "model = \"o3\"\n" {
		t.Fatalf("created toml: %q", got)
	}
}

func TestTOMLValueNeedingEscapes(t *testing.T) {
	sandbox(t)
	write(t, ".codex/config.toml", `model = "old"`)
	tg, _ := Lookup("codex")
	if err := tg.Set(`we"ird\name`); err != nil {
		t.Fatal(err)
	}
	if v, err := tg.Get(); err != nil || v != `we"ird\name` {
		t.Fatalf("round trip: %q %v", v, err)
	}
}

func TestUnknownAgentAndEmptyModel(t *testing.T) {
	sandbox(t)
	if _, err := Lookup("nope"); err == nil {
		t.Fatal("unknown agent accepted")
	}
	tg, _ := Lookup("claude")
	if err := tg.Set("  "); err == nil {
		t.Fatal("empty model accepted")
	}
	if v, err := tg.Get(); v != "" || !errors.Is(err, ErrNoKey) {
		t.Fatalf("missing file: %q %v", v, err)
	}
}

// A trailing comment after the value is common; the replacement must land on
// the value, not inside the comment.
func TestTOMLTrailingComment(t *testing.T) {
	sandbox(t)
	write(t, ".codex/config.toml", "model = \"gpt-5-codex\" # pinned by the team\n")
	tg, _ := Lookup("codex")
	if err := tg.Set("kimi-k2"); err != nil {
		t.Fatal(err)
	}
	got := read(t, ".codex/config.toml")
	if got != "model = \"kimi-k2\" # pinned by the team\n" {
		t.Fatalf("comment or value damaged: %q", got)
	}
	if v, err := tg.Get(); err != nil || v != "kimi-k2" {
		t.Fatalf("Get = %q, %v", v, err)
	}
}

func TestTOMLQuotedKeyAndMultiLine(t *testing.T) {
	sandbox(t)
	write(t, ".codex/config.toml", "\"model\" = \"old\"\n")
	tg, _ := Lookup("codex")
	if err := tg.Set("o3"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, ".codex/config.toml"); strings.Count(got, "model\" =")+strings.Count(got, "model =") != 1 {
		t.Fatalf("quoted key produced a second key:\n%s", got)
	}

	write(t, ".codex/config.toml", "model = \"\"\"line one\nline two\"\"\"\n")
	if err := tg.Set("o3"); err == nil {
		t.Fatal("multi-line value should be refused, not guessed at")
	}
	if got := read(t, ".codex/config.toml"); !strings.Contains(got, "line two") {
		t.Fatalf("refused write still modified the file:\n%s", got)
	}
}

// Inserting a key must not land inside a multi-line array.
func TestTOMLArrayContinuationIsNotATable(t *testing.T) {
	sandbox(t)
	write(t, ".codex/config.toml", "arr = [\n[1, 2],\n]\n")
	tg, _ := Lookup("codex")
	if err := tg.Set("o3"); err != nil {
		t.Fatal(err)
	}
	got := read(t, ".codex/config.toml")
	if strings.Contains(got, "[1, 2],\nmodel") {
		t.Fatalf("inserted inside the array:\n%s", got)
	}
	if !strings.Contains(got, `model = "o3"`) {
		t.Fatalf("model not written:\n%s", got)
	}
}

// A file holding JSON null must be filled in, not panic.
func TestJSONNullRoot(t *testing.T) {
	sandbox(t)
	write(t, ".claude/settings.json", "null\n")
	tg, _ := Lookup("claude")
	if err := tg.Set("sonnet"); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(read(t, ".claude/settings.json")), &doc); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if doc["model"] != "sonnet" {
		t.Fatalf("model = %v", doc["model"])
	}
}

// Numbers and HTML-ish strings outside the model key must survive byte for
// byte in value terms: no float rounding, no \u003c escaping.
func TestJSONLeavesOtherKeysAlone(t *testing.T) {
	sandbox(t)
	write(t, ".claude/settings.json", `{"model":"opus","big":1234567890123456789,"html":"<a & b>"}`)
	tg, _ := Lookup("claude")
	if err := tg.Set("sonnet"); err != nil {
		t.Fatal(err)
	}
	got := read(t, ".claude/settings.json")
	if !strings.Contains(got, "1234567890123456789") {
		t.Fatalf("big integer was rounded:\n%s", got)
	}
	if !strings.Contains(got, "<a & b>") || strings.Contains(got, `\u003c`) {
		t.Fatalf("HTML-ish string was escaped:\n%s", got)
	}
}

// A multi-line string anywhere before the model can hide a fake `model =`
// line; the whole file must be refused rather than edited inside the string.
func TestTOMLMultilineStringElsewhere(t *testing.T) {
	sandbox(t)
	write(t, ".codex/config.toml", "prompt = \"\"\"\nmodel = \"fake\"\n\"\"\"\nmodel = \"real\"\n")
	tg, _ := Lookup("codex")
	if err := tg.Set("o3"); err == nil {
		t.Fatal("a file with a multi-line string should be refused")
	}
	if got := read(t, ".codex/config.toml"); !strings.Contains(got, `model = "fake"`) || !strings.Contains(got, `model = "real"`) {
		t.Fatalf("refused write still changed the file:\n%s", got)
	}
}
