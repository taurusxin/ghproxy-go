package main

import "testing"

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
