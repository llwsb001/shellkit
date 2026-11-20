//go:build !nogui

package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/llwsb001/shellkit/internal/core"
)

func mount(t *testing.T) *httptest.Server {
	t.Helper()
	t.Setenv("SHELLKIT_HOME", t.TempDir())
	s := core.New("127.0.0.1:0")
	Mount(s)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestRootServesPage(t *testing.T) {
	ts := mount(t)
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		// A redirect here was the original bug: http.FileServer sends
		// /index.html to ./ and the root handler loops.
		t.Fatalf("root: want 200, got %d (redirect loop?)", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content type: %q", ct)
	}
	if res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control: %q", res.Header.Get("Cache-Control"))
	}
	body := readAll(t, res)
	for _, want := range []string{"shellkit", "app.js", "app.css"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not mention %q", want)
		}
	}
}

func TestStaticFilesAreServed(t *testing.T) {
	ts := mount(t)
	for _, p := range []string{"/app.js", "/app.css"} {
		res, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Errorf("%s: %d", p, res.StatusCode)
		}
	}
}

func TestUnknownPathIs404(t *testing.T) {
	ts := mount(t)
	res, err := http.Get(ts.URL + "/nope.txt")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("/nope.txt: %d", res.StatusCode)
	}
}

func TestAPISurvivesMounting(t *testing.T) {
	ts := mount(t)
	if code, body := post(t, ts.URL+"/api/notes", `{"text":"from the page"}`); code != 201 || !strings.Contains(body, "from the page") {
		t.Fatalf("post after mount: %d %s", code, body)
	}
}

func readAll(t *testing.T, res *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func post(t *testing.T, url, body string) (int, string) {
	t.Helper()
	res, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}
