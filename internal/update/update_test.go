package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.1.0", "v0.1.1", -1},
		{"v0.2.0", "v0.1.9", 1},
		{"v1.10.0", "v1.9.0", 1},
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.0-beta.1", "v1.0.0", -1},
		{"1.2.3", "v1.2.4", -1},
		{"v1.0.0-beta.2", "v1.0.0-beta.10", -1},
		{"v1.0.0-beta", "v1.0.0-beta.1", -1},
		{"v1.2.3.4", "v1.2.3", 1},
		{"v2.0.0-rc.1", "v2.0.0-beta.9", 1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	if got := AssetName("linux", "amd64"); got != "shellkit-linux-amd64" {
		t.Fatalf("asset name: %s", got)
	}
	if got := AssetName("darwin", "arm64"); got != "shellkit-darwin-arm64" {
		t.Fatalf("asset name: %s", got)
	}
	if got := AssetName("windows", "amd64"); got != "shellkit-windows-amd64.exe" {
		t.Fatalf("windows asset keeps its extension: %s", got)
	}
}

func TestParseSumsAndVerify(t *testing.T) {
	sums := ParseSums([]byte("" +
		"0b1a9c  shellkit-linux-amd64\n" +
		"111111 *shellkit-darwin-arm64\r\n" +
		"# a comment line\n" +
		"malformed\n"))
	if len(sums) != 2 || sums["shellkit-linux-amd64"] != "0b1a9c" {
		t.Fatalf("sums: %v", sums)
	}
	if err := Verify(sums, "shellkit-linux-amd64", "0b1a9c"); err != nil {
		t.Fatalf("should verify: %v", err)
	}
	if err := Verify(sums, "shellkit-linux-amd64", "deadbeef"); err == nil {
		t.Fatal("wrong digest accepted")
	}
	if err := Verify(sums, "shellkit-windows-amd64", "0b1a9c"); err == nil {
		t.Fatal("missing asset accepted")
	}
	if sums["shellkit-darwin-arm64"] != "111111" {
		t.Fatalf("binary marker or CRLF broke parsing: %v", sums)
	}
}

func TestCheckReadsRelease(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(Release{
			Tag: "v0.3.0",
			Assets: []struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			}{{Name: "shellkit-linux-amd64", URL: "https://example.com/asset"}},
		})
	}))
	defer ts.Close()

	old := APIBase
	APIBase = ts.URL
	defer func() { APIBase = old }()

	rel, err := Check(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v0.3.0" || len(rel.Assets) != 1 {
		t.Fatalf("release: %+v", rel)
	}
}

func TestFetchRefusesOversize(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 600)))
	}))
	defer ts.Close()
	if _, err := fetch(context.Background(), ts.URL, "", 512); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversize download should error, got %v", err)
	}
}

func TestFetchSkipsTokenForForeignHost(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()
	old := APIBase
	APIBase = "https://api.github.com"
	defer func() { APIBase = old }()

	if _, err := fetch(context.Background(), ts.URL, "secret", 512); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Fatalf("token leaked to %s: %q", ts.URL, gotAuth)
	}
}

func TestCheckSurfacesHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	defer ts.Close()
	old := APIBase
	APIBase = ts.URL
	defer func() { APIBase = old }()

	if _, err := Check(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("want a 404 in the error, got %v", err)
	}
}

func TestCompareBuildMetadataAndHugeComponent(t *testing.T) {
	if got := Compare("v1.2.3+build", "v1.2.3"); got != 0 {
		t.Fatalf("build metadata must not change precedence: %d", got)
	}
	if got := Compare("v1.2.99999999999999999999", "v1.2.0"); got != 1 {
		t.Fatalf("overflowing component should still be newer: %d", got)
	}
}
