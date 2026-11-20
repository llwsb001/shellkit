//go:build nogui

package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/llwsb001/shellkit/internal/core"
)

func TestNoguiSaysSo(t *testing.T) {
	t.Setenv("SHELLKIT_HOME", t.TempDir())
	s := core.New("127.0.0.1:0")
	Mount(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotImplemented {
		t.Fatalf("/: want 501, got %d", res.StatusCode)
	}

	res, err = http.Get(ts.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("api must keep working without a page: %d", res.StatusCode)
	}
}
