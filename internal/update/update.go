// Package update is the self-update path: ask GitHub for the latest release,
// download the asset for this OS/arch, check it against SHA256SUMS, then
// replace the running binary. The network part is one function so the rest is
// testable without it.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo is where releases live. Private: callers need GITHUB_TOKEN.
const Repo = "llwsb001/shellkit"

// APIBase is overridable in tests and for GitHub Enterprise.
var APIBase = "https://api.github.com"

// Release is the slice of the GitHub payload this tool needs.
type Release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"url"` // API asset url, not browser_download_url
	} `json:"assets"`
	Draft   bool   `json:"draft"`
	Pre     bool   `json:"prerelease"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
}

// AssetName is the file a release ships for this platform.
func AssetName(goos, goarch string) string {
	name := "shellkit-" + goos + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// Compare orders two vX.Y.Z tags numerically; a prerelease suffix sorts below
// the same numbers without one. Equal is 0.
func Compare(a, b string) int {
	an, ap := splitTag(a)
	bn, bp := splitTag(b)
	for i := 0; i < len(an) || i < len(bn); i++ {
		x, y := 0, 0
		if i < len(an) {
			x = an[i]
		}
		if i < len(bn) {
			y = bn[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case ap == bp:
		return 0
	case ap == "":
		return 1
	case bp == "":
		return -1
	default:
		return comparePrerelease(ap, bp)
	}
}

// splitTag splits "v1.2.3-beta.1" into as many numeric components as it finds
// and the prerelease tail. Extra components are not dropped: 1.2.3.4 is newer
// than 1.2.3, not older.
func splitTag(t string) ([]int, string) {
	t = strings.TrimPrefix(strings.TrimSpace(t), "v")
	// Build metadata (+...) does not affect precedence in semver.
	if i := strings.Index(t, "+"); i >= 0 {
		t = t[:i]
	}
	pre := ""
	if i := strings.Index(t, "-"); i >= 0 {
		pre = t[i+1:]
		t = t[:i]
	}
	var out []int
	for _, part := range strings.Split(t, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			// A component too large for int is still "very new", not zero.
			out = append(out, math.MaxInt)
			continue
		}
		out = append(out, n)
	}
	return out, pre
}

// comparePrerelease orders identifiers the semver way: numeric identifiers
// numerically (beta.2 < beta.10), alphanumeric ones lexically, and a numeric
// identifier sorts below an alphanumeric one.
func comparePrerelease(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		if i == len(as) {
			return -1
		}
		if i == len(bs) {
			return 1
		}
		x, xerr := strconv.Atoi(as[i])
		y, yerr := strconv.Atoi(bs[i])
		switch {
		case xerr == nil && yerr == nil:
			if x != y {
				if x < y {
					return -1
				}
				return 1
			}
		case xerr == nil:
			return -1
		case yerr == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return 0
}

// Check asks GitHub for the latest published release.
func Check(ctx context.Context, token string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, APIBase+"/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 400))
		return Release{}, fmt.Errorf("github: %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	var rel Release
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&rel); err != nil {
		return Release{}, err
	}
	return rel, nil
}

// ParseSums turns sha256sum output into name → hex.
func ParseSums(b []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		out[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	return out
}

// Verify checks one asset's sha256 against the parsed sums.
func Verify(sums map[string]string, asset, digest string) error {
	want, ok := sums[asset]
	if !ok {
		return fmt.Errorf("%s is not in SHA256SUMS", asset)
	}
	if !strings.EqualFold(want, digest) {
		return fmt.Errorf("%s: sha256 %s, expected %s", asset, digest, want)
	}
	return nil
}

// Run is the CLI entry: `shellkit update` or `shellkit update --check`.
func Run(args []string, current string) error {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	checkOnly := false
	for _, a := range args {
		switch a {
		case "--check", "-check", "-n":
			checkOnly = true
		case "":
		default:
			return fmt.Errorf("usage: shellkit update [--check]")
		}
	}

	rel, err := Check(ctx, token)
	if err != nil {
		return err
	}
	if Compare(rel.Tag, current) <= 0 {
		fmt.Printf("shellkit %s is up to date (latest %s)\n", current, rel.Tag)
		return nil
	}

	fmt.Printf("shellkit %s → %s\n  %s\n", current, rel.Tag, rel.HTMLURL)
	if checkOnly {
		return nil
	}
	return apply(ctx, rel, token)
}

func apply(ctx context.Context, rel Release, token string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("self-replace is not implemented on Windows: download %s manually", AssetName(runtime.GOOS, runtime.GOARCH))
	}
	want := AssetName(runtime.GOOS, runtime.GOARCH)
	var assetURL, sumsURL string
	for _, a := range rel.Assets {
		switch a.Name {
		case want:
			assetURL = a.URL
		case "SHA256SUMS":
			sumsURL = a.URL
		}
	}
	if assetURL == "" || sumsURL == "" {
		return fmt.Errorf("%s or SHA256SUMS is missing from release %s", want, rel.Tag)
	}

	bin, err := fetch(ctx, assetURL, token, 64<<20)
	if err != nil {
		return err
	}
	sumsRaw, err := fetch(ctx, sumsURL, token, 1<<20)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(bin)
	if err := Verify(ParseSums(sumsRaw), want, hex.EncodeToString(sum[:])); err != nil {
		return err
	}

	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, _ = filepath.EvalSymlinks(self)
	// A random, exclusive temp file: a stale ".new" from an interrupted run
	// must not block the next one, and an existing path must not be reused.
	f, err := os.CreateTemp(filepath.Dir(self), filepath.Base(self)+".new-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := f.Chmod(0o755); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if _, err := f.Write(bin); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	old := self + ".old"
	_ = os.Remove(old)
	if err := os.Rename(self, old); err != nil {
		return err
	}
	if err := os.Rename(tmp, self); err != nil {
		// Put the old binary back; if that also fails, say so instead of
		// leaving the user with a binary that is simply gone.
		if rerr := os.Rename(old, self); rerr != nil {
			return fmt.Errorf("%w (and restoring the old binary failed: %v)", err, rerr)
		}
		return err
	}
	_ = os.Remove(old)
	fmt.Printf("updated to %s\n", rel.Tag)
	return nil
}

func fetch(ctx context.Context, url, token string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	// Send the token to our API host only: a hostile or misconfigured asset
	// URL must not receive it.
	if token != "" && sameHost(url, APIBase) {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", filepath.Base(url), res.Status)
	}
	// Read one byte past the cap so an oversized payload is an error, not a
	// silently truncated file that fails the checksum with a confusing message.
	b, err := io.ReadAll(io.LimitReader(res.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("download exceeds %d bytes", max)
	}
	return b, nil
}

func sameHost(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}
	return strings.EqualFold(ua.Hostname(), ub.Hostname())
}
