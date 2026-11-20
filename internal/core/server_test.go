package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	t.Setenv("SHELLKIT_HOME", t.TempDir())
	s := New("127.0.0.1:0")
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func do(t *testing.T, method, url, body string) (int, map[string]any) {
	t.Helper()
	var r *http.Request
	if body == "" {
		r, _ = http.NewRequest(method, url, nil)
	} else {
		r, _ = http.NewRequest(method, url, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func TestHealthAndState(t *testing.T) {
	_, ts := newTestServer(t)

	code, body := do(t, "GET", ts.URL+"/healthz", "")
	if code != 200 || body["ok"] != true {
		t.Fatalf("healthz: %d %v", code, body)
	}
	code, body = do(t, "GET", ts.URL+"/api/state", "")
	if code != 200 || body["count"].(float64) != 0 {
		t.Fatalf("state: %d %v", code, body)
	}
	if !strings.HasSuffix(body["file"].(string), "notes.json") {
		t.Fatalf("state file: %v", body["file"])
	}
}

func TestNotesRoundTrip(t *testing.T) {
	_, ts := newTestServer(t)

	code, body := do(t, "POST", ts.URL+"/api/notes", `{"text":"hello shell","tags":["Demo"]}`)
	if code != 201 {
		t.Fatalf("want 201, got %d %v", code, body)
	}
	id, _ := body["id"].(string)
	if id == "" || body["text"] != "hello shell" {
		t.Fatalf("bad create: %v", body)
	}

	code, body = do(t, "GET", ts.URL+"/api/notes?q=demo", "")
	if code != 200 || body["count"].(float64) != 1 {
		t.Fatalf("list by tag: %d %v", code, body)
	}

	code, _ = do(t, "DELETE", ts.URL+"/api/notes/"+id, "")
	if code != 200 {
		t.Fatalf("delete: %d", code)
	}
	code, body = do(t, "DELETE", ts.URL+"/api/notes/"+id, "")
	if code != 404 {
		t.Fatalf("second delete should 404: %d %v", code, body)
	}
}

func TestAddRejects(t *testing.T) {
	_, ts := newTestServer(t)

	if code, _ := do(t, "POST", ts.URL+"/api/notes", `{"text":"  "}`); code != 400 {
		t.Fatalf("empty text: %d", code)
	}
	if code, _ := do(t, "POST", ts.URL+"/api/notes", `{bad`); code != 400 {
		t.Fatalf("bad json: %d", code)
	}
	req, _ := http.NewRequest("POST", ts.URL+"/api/notes", strings.NewReader("text=x"))
	req.Header.Set("Content-Type", "text/plain")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 415 {
		t.Fatalf("wrong content type: %d", res.StatusCode)
	}
}

func TestInvalidLimitIs400(t *testing.T) {
	_, ts := newTestServer(t)
	if code, _ := do(t, "GET", ts.URL+"/api/notes?limit=abc", ""); code != 400 {
		t.Fatalf("want 400, got %d", code)
	}
	if code, _ := do(t, "GET", ts.URL+"/api/notes?limit=-1", ""); code != 400 {
		t.Fatalf("negative limit: %d", code)
	}
}

func TestCorruptFileIs500(t *testing.T) {
	t.Setenv("SHELLKIT_HOME", t.TempDir())
	s := New("127.0.0.1:0")
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(File(), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _ := do(t, "GET", ts.URL+"/api/state", ""); code != 500 {
		t.Fatalf("state should report corruption, got %d", code)
	}
}

func TestSnapshot(t *testing.T) {
	s, _ := newTestServer(t)
	st := s.Snapshot()
	if st.Count != 0 || st.File != File() || st.Addr != "127.0.0.1:0" {
		t.Fatalf("snapshot: %+v", st)
	}
}

func TestStateCountsRequests(t *testing.T) {
	_, ts := newTestServer(t)
	do(t, "POST", ts.URL+"/api/notes", `{"text":"one"}`)
	do(t, "GET", ts.URL+"/api/notes", "")
	code, body := do(t, "GET", ts.URL+"/api/state", "")
	if code != 200 || body["count"].(float64) != 1 || body["requests"].(float64) != 2 {
		t.Fatalf("state after calls: %v", body)
	}
	last, _ := body["last"].(map[string]any)
	if last == nil || last["kind"] != "list" {
		t.Fatalf("last action: %v", body["last"])
	}
}
