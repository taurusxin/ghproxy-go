package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestIsFullGitHubURL(t *testing.T) {
	cases := map[string]bool{
		// full form, with scheme
		"https://github.com/user/repo":            true,
		"http://github.com/user/repo":             true,
		"https://raw.githubusercontent.com/u/r/b/f": true,
		"https://gist.github.com/u/gid":           true,
		// full form, scheme collapsed by a reverse proxy
		"https:/github.com/user/repo":             true,
		"https:/raw.githubusercontent.com/u/r/b/f": true,
		// full form, no scheme
		"github.com/user/repo":            true,
		"raw.githubusercontent.com/u/r/b/f": true,
		// short form (no host)
		"user/repo":                                 false,
		"user/repo/releases/download/v1.0/file.zip": false,
		"user/repo/info/refs":                       false,
		"user/repo.git/git-upload-pack":             false,
	}
	for in, want := range cases {
		if got := isFullGitHubURL(in); got != want {
			t.Errorf("isFullGitHubURL(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestResolveTarget(t *testing.T) {
	type tc struct {
		path, query string
		wantOK      bool
		wantURL     string // checked only when wantOK
	}
	cases := []tc{
		// index page
		{path: "/", wantOK: false},
		{path: "", wantOK: false},

		// ---- short form: git clone ----
		{path: "/user/repo/info/refs", query: "service=git-upload-pack",
			wantOK: true, wantURL: "https://github.com/user/repo/info/refs?service=git-upload-pack"},
		{path: "/user/repo.git/git-upload-pack", wantOK: true,
			wantURL: "https://github.com/user/repo.git/git-upload-pack"},
		{path: "/user/repo/info/refs", query: "service=git-receive-pack", wantOK: true,
			wantURL: "https://github.com/user/repo/info/refs?service=git-receive-pack"},

		// ---- short form: release / archive download ----
		{path: "/user/repo/releases/download/v1.0/file.zip", wantOK: true,
			wantURL: "https://github.com/user/repo/releases/download/v1.0/file.zip"},
		{path: "/user/repo/archive/refs/heads/main.zip", wantOK: true,
			wantURL: "https://github.com/user/repo/archive/refs/heads/main.zip"},

		// ---- short form: web resources must be rejected ----
		{path: "/user/repo", wantOK: false},                       // repo page
		{path: "/user/repo/blob/main/README.md", wantOK: false},   // blob
		{path: "/user/repo/raw/main/README.md", wantOK: false},    // raw
		{path: "/user/repo/issues", wantOK: false},                // issues page
		{path: "/user/repo/pulls", wantOK: false},                 // pulls page
		{path: "/user/repo/tree/main", wantOK: false},             // tree view
		{path: "/user/repo/stargazers", wantOK: false},            // misc page

		// ---- full form: everything the proxy already supported still works ----
		{path: "/https://github.com/user/repo/releases/download/v1.0/file.zip", wantOK: true,
			wantURL: "https://github.com/user/repo/releases/download/v1.0/file.zip"},
		{path: "/https://github.com/user/repo/blob/main/README.md", wantOK: true,
			wantURL: "https://github.com/user/repo/raw/main/README.md"}, // blob -> raw
		{path: "/github.com/user/repo/info/refs", query: "service=git-upload-pack", wantOK: true,
			wantURL: "https://github.com/user/repo/info/refs?service=git-upload-pack"},
		{path: "/https:/github.com/user/repo/releases/download/v1.0/file.zip", wantOK: true,
			wantURL: "https://github.com/user/repo/releases/download/v1.0/file.zip"}, // collapsed scheme
		{path: "/https://raw.githubusercontent.com/user/repo/main/file", wantOK: true,
			wantURL: "https://raw.githubusercontent.com/user/repo/main/file"},

		// full form, unsupported resource -> rejected (unchanged behavior)
		{path: "/https://github.com/user/repo", wantOK: false},
	}
	for _, c := range cases {
		gotURL, gotOK := resolveTarget(c.path, c.query)
		if gotOK != c.wantOK {
			t.Errorf("resolveTarget(path=%q, q=%q): ok=%v, want %v", c.path, c.query, gotOK, c.wantOK)
			continue
		}
		if c.wantOK && gotURL != c.wantURL {
			t.Errorf("resolveTarget(path=%q, q=%q): url=%q, want %q", c.path, c.query, gotURL, c.wantURL)
		}
	}
}

// newProxyTestContext builds a gin context backed by a response recorder, with a
// GET request suitable for driving doProxy directly in tests.
func newProxyTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c, w
}

// TestDoProxyFollowsRedirectChain verifies that doProxy follows a redirect chain
// internally and streams the final response body to the client.
func TestDoProxyFollowsRedirectChain(t *testing.T) {
	const body = "FILE_CONTENT"
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer final.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer origin.Close()

	c, w := newProxyTestContext()
	doProxy(c, origin.URL, false)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != body {
		t.Errorf("body = %q, want %q", w.Body.String(), body)
	}
}

// TestDoProxyFollowsGitHubPatternRedirect is the regression test for the
// "releases/latest" bug. The redirect target matches the proxy's GitHub URL
// patterns; previously such a Location was rewritten and a 302 was returned to
// the client. It must now be followed internally so the client gets the file.
func TestDoProxyFollowsGitHubPatternRedirect(t *testing.T) {
	const body = "LATEST_FILE"
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer final.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer origin.Close()

	// Make the redirect target look like a GitHub URL so it matches the proxy
	// patterns (matchURL == true). The proxy must still follow it internally.
	origExps := exps
	exps = []*regexp.Regexp{regexp.MustCompile("^" + regexp.QuoteMeta(final.URL))}
	defer func() { exps = origExps }()

	c, w := newProxyTestContext()
	doProxy(c, origin.URL, false)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (should follow redirect internally, not return 302)", w.Code, http.StatusOK)
	}
	if w.Body.String() != body {
		t.Errorf("body = %q, want %q", w.Body.String(), body)
	}
}
